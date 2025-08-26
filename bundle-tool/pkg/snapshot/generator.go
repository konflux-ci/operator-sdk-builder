package snapshot

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/provenance"
	"gopkg.in/yaml.v3"
)

// KonfluxSnapshot represents a Konflux Snapshot resource matching the official API
type KonfluxSnapshot struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   SnapshotMetadata `yaml:"metadata"`
	Spec       SnapshotSpec     `yaml:"spec"`
}

type SnapshotMetadata struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// SnapshotSpec matches the official Konflux API
type SnapshotSpec struct {
	Application        string              `yaml:"application"`
	DisplayName        string              `yaml:"displayName,omitempty"`
	DisplayDescription string              `yaml:"displayDescription,omitempty"`
	Components         []SnapshotComponent `yaml:"components,omitempty"`
}

// SnapshotComponent matches the official Konflux API
type SnapshotComponent struct {
	Name           string           `yaml:"name"`
	ContainerImage string           `yaml:"containerImage"`
	Source         *ComponentSource `yaml:"source,omitempty"`
}

// ComponentSource matches the official Konflux API
type ComponentSource struct {
	Git *GitSource `yaml:"git,omitempty"`
}

// GitSource matches the official Konflux API
type GitSource struct {
	URL      string `yaml:"url"`
	Revision string `yaml:"revision,omitempty"`
}

// SnapshotGenerator handles generation of Konflux Snapshot YAML
type SnapshotGenerator struct {
	appName        string
	namespace      string
	componentNames map[string]string // Map from image reference to component name
	nameCollisions map[string]int    // Map from base name to collision count
}

// NewSnapshotGenerator creates a new SnapshotGenerator
func NewSnapshotGenerator(appName, namespace string) *SnapshotGenerator {
	return &SnapshotGenerator{
		appName:        appName,
		namespace:      namespace,
		componentNames: make(map[string]string),
		nameCollisions: make(map[string]int),
	}
}

// NewSnapshotGeneratorWithProvenanceParser creates a new SnapshotGenerator with application name and namespace from provenance
func NewSnapshotGeneratorWithProvenanceParser(ctx context.Context, bundleImage, defaultNamespace string, provenanceParser *provenance.ProvenanceParser) (*SnapshotGenerator, error) {
	return NewSnapshotGeneratorWithSourceFallback(ctx, bundleImage, defaultNamespace, "", "", provenanceParser)
}

// NewSnapshotGeneratorWithSourceFallback creates a new SnapshotGenerator with application name and namespace from provenance,
// with fallbacks for application name and namespace when provenance is not available
func NewSnapshotGeneratorWithSourceFallback(ctx context.Context, bundleImage, defaultNamespace, fallbackAppName, fallbackNamespace string, provenanceParser *provenance.ProvenanceParser) (*SnapshotGenerator, error) {
	var appName, namespace string

	// Try to get application name and namespace from provenance first
	if provenanceParser != nil {
		if provenanceAppName, err := provenanceParser.ExtractApplicationName(ctx, bundleImage); err == nil && provenanceAppName != "" {
			appName = provenanceAppName
		}
		if provenanceNamespace, err := provenanceParser.ExtractNamespace(ctx, bundleImage); err == nil && provenanceNamespace != "" {
			namespace = provenanceNamespace
		}
	}

	// Fall back to provided values if provenance extraction failed
	if appName == "" {
		if fallbackAppName == "" {
			return nil, fmt.Errorf("application name not found in provenance and no fallback provided")
		}
		appName = fallbackAppName
		fmt.Printf("Using fallback application name: %s (provenance not available)\n", appName)
	}

	if namespace == "" && fallbackNamespace != "" {
		namespace = fallbackNamespace
		fmt.Printf("Using fallback namespace: %s (provenance not available)\n", namespace)
	}

	// Override with explicit namespace parameter if provided
	if defaultNamespace != "" {
		namespace = defaultNamespace
	}
	// Note: namespace can be empty - omitted from generated YAML, applied to current namespace

	return &SnapshotGenerator{
		appName:        appName,
		namespace:      namespace,
		componentNames: make(map[string]string),
		nameCollisions: make(map[string]int),
	}, nil
}

// GenerateSnapshot creates a Konflux Snapshot from image references and provenance info
func (sg *SnapshotGenerator) GenerateSnapshot(
	ctx context.Context,
	imageRefs []bundle.ImageReference,
	provenanceResults []provenance.ProvenanceInfo,
	bundleImage string,
	provenanceParser *provenance.ProvenanceParser,
) (*KonfluxSnapshot, error) {
	return sg.GenerateSnapshotWithBundleSource(ctx, imageRefs, provenanceResults, bundleImage, "", "", provenanceParser)
}

// GenerateSnapshotWithBundleSource creates a Konflux Snapshot with optional bundle source fallback information
func (sg *SnapshotGenerator) GenerateSnapshotWithBundleSource(
	ctx context.Context,
	imageRefs []bundle.ImageReference,
	provenanceResults []provenance.ProvenanceInfo,
	bundleImage string,
	bundleSourceRepo string,
	bundleSourceCommit string,
	provenanceParser *provenance.ProvenanceParser,
) (*KonfluxSnapshot, error) {

	timestamp := time.Now().Format("20060102-150405")
	snapshotName := fmt.Sprintf("%s-bundle-snapshot-%s", sg.appName, timestamp)

	metadata := SnapshotMetadata{
		Name: snapshotName,
		Labels: map[string]string{
			"appstudio.openshift.io/application": sg.appName,
			"bundle-tool.konflux.io/source":      "bundle-analysis",
		},
		Annotations: map[string]string{
			"bundle-tool.konflux.io/source-bundle": bundleImage,
			"bundle-tool.konflux.io/generated-at":  time.Now().Format(time.RFC3339),
		},
	}

	// Only set namespace field if provided (when omitted, YAML has no namespace field)
	if sg.namespace != "" {
		metadata.Namespace = sg.namespace
	}

	snapshot := &KonfluxSnapshot{
		APIVersion: "appstudio.redhat.com/v1alpha1",
		Kind:       "Snapshot",
		Metadata:   metadata,
		Spec: SnapshotSpec{
			Application:        sg.appName,
			DisplayName:        fmt.Sprintf("%s Bundle Snapshot", sg.appName),
			DisplayDescription: fmt.Sprintf("Snapshot generated from OLM bundle %s", bundleImage),
			Components:         []SnapshotComponent{},
		},
	}

	// Add the bundle image itself as a component
	bundleComponentName := sg.getBundleComponentName(ctx, bundleImage, provenanceParser)
	bundleComponent := SnapshotComponent{
		Name:           bundleComponentName,
		ContainerImage: bundleImage,
	}

	// Try to add source information for bundle component from its own provenance
	var bundleSourceAdded bool
	if provenanceParser != nil {
		if bundleProvInfo, err := sg.getBundleProvenance(ctx, bundleImage, provenanceParser); err == nil {
			if bundleProvInfo.SourceRepo != "" {
				bundleComponent.Source = &ComponentSource{
					Git: &GitSource{
						URL:      sg.cleanGitURL(bundleProvInfo.SourceRepo),
						Revision: bundleProvInfo.SourceCommit,
					},
				}
				bundleSourceAdded = true
			} else {
				fmt.Printf("Warning: Bundle image %s has no source repository in provenance\n", bundleImage)
			}
		} else {
			fmt.Printf("Warning: Bundle image %s has no valid provenance: %v\n", bundleImage, err)
		}
	}

	// Fall back to provided bundle source information if provenance is not available
	if !bundleSourceAdded && bundleSourceRepo != "" {
		bundleComponent.Source = &ComponentSource{
			Git: &GitSource{
				URL:      bundleSourceRepo,
				Revision: bundleSourceCommit,
			},
		}
		fmt.Printf("Using fallback source for bundle: %s (provenance not available)\n", bundleSourceRepo)
	}

	snapshot.Spec.Components = append(snapshot.Spec.Components, bundleComponent)

	// Convert image references to snapshot components
	// Only include images that have valid provenance with source information
	for i, ref := range imageRefs {
		// Skip images without provenance or without source information
		if i >= len(provenanceResults) {
			fmt.Printf("Skipping image %s: no provenance results\n", ref.Image)
			continue
		}

		prov := provenanceResults[i]
		if !prov.Verified || prov.SourceRepo == "" {
			fmt.Printf("Skipping image %s: no valid provenance source (verified=%t, sourceRepo=%q)\n",
				ref.Image, prov.Verified, prov.SourceRepo)
			continue
		}

		component := SnapshotComponent{
			Name:           sg.generateComponentName(ref),
			ContainerImage: ref.Image,
			Source: &ComponentSource{
				Git: &GitSource{
					URL:      sg.cleanGitURL(prov.SourceRepo),
					Revision: prov.SourceCommit,
				},
			},
		}

		snapshot.Spec.Components = append(snapshot.Spec.Components, component)
	}

	// Deduplicate components based on container image
	sg.DeduplicateComponents(snapshot)

	return snapshot, nil
}

// getBundleComponentName gets the component name for the bundle image, trying provenance first
func (sg *SnapshotGenerator) getBundleComponentName(ctx context.Context, bundleImage string, provenanceParser *provenance.ProvenanceParser) string {
	// Try to get component name from provenance if parser is available
	if provenanceParser != nil {
		if componentName, err := provenanceParser.ExtractComponentName(ctx, bundleImage); err == nil && componentName != "" {
			return componentName
		}
	}

	// Fallback to generating component name from image reference
	bundleRef := bundle.ImageReference{
		Image: bundleImage,
		Name:  "", // Will be generated from image name
	}
	return sg.generateComponentName(bundleRef)
}

// getBundleProvenance gets provenance information for the bundle image
func (sg *SnapshotGenerator) getBundleProvenance(ctx context.Context, bundleImage string, provenanceParser *provenance.ProvenanceParser) (*provenance.ProvenanceInfo, error) {
	// Create a single-item slice to use the existing ParseProvenance method
	bundleRef := []bundle.ImageReference{{Image: bundleImage}}
	provenanceResults, err := provenanceParser.ParseProvenance(ctx, bundleRef)
	if err != nil {
		return nil, err
	}

	if len(provenanceResults) == 0 {
		return nil, fmt.Errorf("no provenance results for bundle image")
	}

	return &provenanceResults[0], nil
}

// generateComponentName creates a valid Kubernetes resource name from an image reference with collision detection
func (sg *SnapshotGenerator) generateComponentName(ref bundle.ImageReference) string {
	// Check if we already have a name for this image reference (deterministic)
	if existingName, exists := sg.componentNames[ref.Image]; exists {
		return existingName
	}

	// Generate base name from image reference
	baseName := sg.extractBaseComponentName(ref)

	// Generate unique name with collision detection
	uniqueName := sg.generateUniqueComponentName(baseName, ref.Image)

	// Store the mapping for future reference
	sg.componentNames[ref.Image] = uniqueName

	return uniqueName
}

// extractBaseComponentName extracts and normalizes a component name from an image reference
func (sg *SnapshotGenerator) extractBaseComponentName(ref bundle.ImageReference) string {
	name := ref.Name
	if name == "" {
		// Extract component name from image reference
		parts := strings.Split(ref.Image, "/")
		imageName := parts[len(parts)-1]

		// Remove tag/digest to get the base image name
		if idx := strings.Index(imageName, ":"); idx != -1 {
			imageName = imageName[:idx]
		}
		if idx := strings.Index(imageName, "@"); idx != -1 {
			imageName = imageName[:idx]
		}

		name = imageName
	}

	return sg.normalizeKubernetesName(name)
}

// normalizeKubernetesName ensures the name follows Kubernetes naming conventions
func (sg *SnapshotGenerator) normalizeKubernetesName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace common invalid characters
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "@", "-")

	// Use regex to remove any remaining invalid characters (keep only alphanumeric and hyphens)
	validChars := regexp.MustCompile(`[^a-z0-9-]`)
	name = validChars.ReplaceAllString(name, "")

	// Remove leading/trailing hyphens and consecutive hyphens
	name = strings.Trim(name, "-")
	multipleHyphens := regexp.MustCompile(`-+`)
	name = multipleHyphens.ReplaceAllString(name, "-")

	// Ensure name is not empty
	if name == "" {
		name = "component"
	}

	// Ensure it starts with alphanumeric character
	if len(name) > 0 && (name[0] < 'a' || name[0] > 'z') && (name[0] < '0' || name[0] > '9') {
		name = "c" + name
	}

	// Ensure it ends with alphanumeric character
	if len(name) > 0 && (name[len(name)-1] < 'a' || name[len(name)-1] > 'z') && (name[len(name)-1] < '0' || name[len(name)-1] > '9') {
		name = name + "1"
	}

	// Kubernetes names must be 63 characters or fewer
	if len(name) > 63 {
		name = name[:63]
		// Ensure it still ends with alphanumeric after truncation
		if len(name) > 0 && (name[len(name)-1] < 'a' || name[len(name)-1] > 'z') && (name[len(name)-1] < '0' || name[len(name)-1] > '9') {
			name = name[:len(name)-1] + "1"
		}
	}

	return name
}

// generateUniqueComponentName generates a unique component name, handling collisions deterministically
func (sg *SnapshotGenerator) generateUniqueComponentName(baseName string, imageRef string) string {
	// Check if this base name has been used before
	if count, exists := sg.nameCollisions[baseName]; exists {
		// There's a collision, generate a unique suffix
		sg.nameCollisions[baseName] = count + 1
		return sg.createNameWithSuffix(baseName, imageRef, count+1)
	}

	// Check if any existing component name matches this base name
	for _, existingName := range sg.componentNames {
		if existingName == baseName {
			// First collision detected
			sg.nameCollisions[baseName] = 1
			return sg.createNameWithSuffix(baseName, imageRef, 1)
		}
	}

	// No collision, use the base name
	sg.nameCollisions[baseName] = 0
	return baseName
}

// createNameWithSuffix creates a name with a deterministic suffix to resolve collisions
func (sg *SnapshotGenerator) createNameWithSuffix(baseName string, imageRef string, collisionCount int) string {
	// Generate a deterministic suffix based on the image reference
	// This ensures the same image always gets the same component name
	hash := sha256.Sum256([]byte(imageRef))
	suffix := fmt.Sprintf("%x", hash[:3]) // Use first 6 characters of hash

	// Combine base name with suffix, ensuring total length stays within Kubernetes limits
	maxBaseLength := 63 - len(suffix) - 1 // -1 for the hyphen
	if len(baseName) > maxBaseLength {
		baseName = baseName[:maxBaseLength]
		// Ensure it ends with alphanumeric after truncation
		if len(baseName) > 0 && (baseName[len(baseName)-1] < 'a' || baseName[len(baseName)-1] > 'z') && (baseName[len(baseName)-1] < '0' || baseName[len(baseName)-1] > '9') {
			baseName = baseName[:len(baseName)-1] + "1"
		}
	}

	return fmt.Sprintf("%s-%s", baseName, suffix)
}

// cleanGitURL converts provenance git URLs to standard format supporting multiple Git hosting platforms
func (sg *SnapshotGenerator) cleanGitURL(provenanceURL string) string {
	if provenanceURL == "" {
		return provenanceURL
	}

	// Handle git+ prefix (commonly used in provenance)
	provenanceURL = strings.TrimPrefix(provenanceURL, "git+")

	// Clean up the URL using the Git URL parser
	return sg.parseAndNormalizeGitURL(provenanceURL)
}

// parseAndNormalizeGitURL parses and normalizes Git URLs for multiple hosting platforms
func (sg *SnapshotGenerator) parseAndNormalizeGitURL(gitURL string) string {
	// Handle SSH URLs first (git@host:path format)
	if sshURL := sg.parseSSHGitURL(gitURL); sshURL != "" {
		return sshURL
	}

	// Handle HTTPS URLs
	if httpsURL := sg.parseHTTPSGitURL(gitURL); httpsURL != "" {
		return httpsURL
	}

	// Handle URLs without protocol (assume HTTPS)
	if protocollessURL := sg.parseProtocollessGitURL(gitURL); protocollessURL != "" {
		return protocollessURL
	}

	// Return original URL if no parsing succeeded
	return gitURL
}

// parseSSHGitURL handles SSH Git URLs (git@host:user/repo format)
func (sg *SnapshotGenerator) parseSSHGitURL(gitURL string) string {
	// Pattern: git@hostname:path or ssh://git@hostname/path or ssh://hostname/path
	if strings.HasPrefix(gitURL, "ssh://git@") {
		// ssh://git@hostname/path format
		url := strings.TrimPrefix(gitURL, "ssh://git@")
		if idx := strings.Index(url, "/"); idx != -1 {
			hostname := url[:idx]
			path := url[idx+1:]
			return sg.normalizeGitURL(hostname, path)
		}
	} else if strings.HasPrefix(gitURL, "ssh://") {
		// ssh://hostname/path format (without git@)
		url := strings.TrimPrefix(gitURL, "ssh://")
		if idx := strings.Index(url, "/"); idx != -1 {
			hostname := url[:idx]
			path := url[idx+1:]
			return sg.normalizeGitURL(hostname, path)
		}
	} else if strings.HasPrefix(gitURL, "git@") {
		// git@hostname:path format
		url := strings.TrimPrefix(gitURL, "git@")
		if idx := strings.Index(url, ":"); idx != -1 {
			hostname := url[:idx]
			path := url[idx+1:]
			return sg.normalizeGitURL(hostname, path)
		}
	}

	return ""
}

// parseHTTPSGitURL handles HTTPS Git URLs
func (sg *SnapshotGenerator) parseHTTPSGitURL(gitURL string) string {
	// Pattern: https://hostname/path or http://hostname/path
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(gitURL, prefix) {
			url := strings.TrimPrefix(gitURL, prefix)
			if idx := strings.Index(url, "/"); idx != -1 {
				hostname := url[:idx]
				path := url[idx+1:]
				return sg.normalizeGitURL(hostname, path)
			}
		}
	}

	return ""
}

// parseProtocollessGitURL handles URLs without protocol (assumes HTTPS)
func (sg *SnapshotGenerator) parseProtocollessGitURL(gitURL string) string {
	// Pattern: hostname/path (no protocol)
	// Skip URLs with @ symbols as they are likely malformed SSH URLs
	if !strings.Contains(gitURL, "://") && !strings.Contains(gitURL, "@") && strings.Contains(gitURL, "/") {
		if idx := strings.Index(gitURL, "/"); idx != -1 {
			hostname := gitURL[:idx]
			path := gitURL[idx+1:]

			// Only process if hostname looks like a known Git hosting platform
			if sg.isKnownGitHostingPlatform(hostname) {
				return sg.normalizeGitURL(hostname, path)
			}
		}
	}

	return ""
}

// normalizeGitURL normalizes Git URLs to HTTPS format for known hosting platforms
func (sg *SnapshotGenerator) normalizeGitURL(hostname, path string) string {
	// Remove .git suffix from path if present
	path = strings.TrimSuffix(path, ".git")

	// Remove leading/trailing slashes from path
	path = strings.Trim(path, "/")

	// Handle different hosting platforms
	switch {
	case sg.isGitHub(hostname):
		return fmt.Sprintf("https://%s/%s", hostname, path)
	case sg.isGitLab(hostname):
		return fmt.Sprintf("https://%s/%s", hostname, path)
	case sg.isBitBucket(hostname):
		return fmt.Sprintf("https://%s/%s", hostname, path)
	case sg.isAzureDevOps(hostname):
		return sg.normalizeAzureDevOpsURL(hostname, path)
	case sg.isGitea(hostname):
		return fmt.Sprintf("https://%s/%s", hostname, path)
	case sg.isCodeCommit(hostname):
		return fmt.Sprintf("https://%s/%s", hostname, path)
	default:
		// For unknown platforms, assume standard HTTPS format
		return fmt.Sprintf("https://%s/%s", hostname, path)
	}
}

// isKnownGitHostingPlatform checks if hostname is a known Git hosting platform
func (sg *SnapshotGenerator) isKnownGitHostingPlatform(hostname string) bool {
	return sg.isGitHub(hostname) || sg.isGitLab(hostname) || sg.isBitBucket(hostname) ||
		sg.isAzureDevOps(hostname) || sg.isGitea(hostname) || sg.isCodeCommit(hostname)
}

// isGitHub checks if hostname is GitHub
func (sg *SnapshotGenerator) isGitHub(hostname string) bool {
	return hostname == "github.com" || strings.HasSuffix(hostname, ".github.com") || strings.Contains(hostname, "github")
}

// isGitLab checks if hostname is GitLab (including self-hosted instances)
func (sg *SnapshotGenerator) isGitLab(hostname string) bool {
	return hostname == "gitlab.com" || strings.Contains(hostname, "gitlab")
}

// isBitBucket checks if hostname is Bitbucket
func (sg *SnapshotGenerator) isBitBucket(hostname string) bool {
	return hostname == "bitbucket.org" || strings.HasSuffix(hostname, ".bitbucket.org")
}

// isAzureDevOps checks if hostname is Azure DevOps
func (sg *SnapshotGenerator) isAzureDevOps(hostname string) bool {
	return strings.Contains(hostname, "dev.azure.com") ||
		strings.Contains(hostname, "visualstudio.com") ||
		strings.Contains(hostname, "azure.com") ||
		strings.Contains(hostname, "ssh.dev.azure.com")
}

// isGitea checks if hostname is Gitea (including self-hosted instances)
func (sg *SnapshotGenerator) isGitea(hostname string) bool {
	return strings.Contains(hostname, "gitea") ||
		hostname == "codeberg.org" // Popular Gitea instance
}

// isCodeCommit checks if hostname is AWS CodeCommit
func (sg *SnapshotGenerator) isCodeCommit(hostname string) bool {
	return strings.Contains(hostname, "codecommit") && strings.Contains(hostname, "amazonaws.com")
}

// normalizeAzureDevOpsURL handles special Azure DevOps URL formats
func (sg *SnapshotGenerator) normalizeAzureDevOpsURL(hostname, path string) string {
	// Azure DevOps has special URL formats:
	// https://dev.azure.com/organization/project/_git/repository
	// https://organization.visualstudio.com/project/_git/repository
	// ssh://ssh.dev.azure.com/v3/organization/project/repository

	if strings.Contains(hostname, "ssh.dev.azure.com") {
		// SSH Azure DevOps format - convert to HTTPS
		return fmt.Sprintf("https://ssh.dev.azure.com/%s", path)
	} else if strings.Contains(hostname, "dev.azure.com") {
		// Modern Azure DevOps format
		pathParts := strings.Split(path, "/")
		if len(pathParts) >= 3 {
			org := pathParts[0]
			project := pathParts[1]
			if len(pathParts) >= 4 && pathParts[2] == "_git" {
				repo := pathParts[3]
				return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s", org, project, repo)
			}
		}
		return fmt.Sprintf("https://dev.azure.com/%s", path)
	} else if strings.Contains(hostname, "visualstudio.com") {
		// Legacy Azure DevOps format
		pathParts := strings.Split(path, "/")
		if len(pathParts) >= 2 && pathParts[0] == "_git" {
			repo := pathParts[1]
			return fmt.Sprintf("https://%s/_git/%s", hostname, repo)
		}
		return fmt.Sprintf("https://%s/%s", hostname, path)
	}

	// Fallback for other Azure formats
	return fmt.Sprintf("https://%s/%s", hostname, path)
}

// ToYAML converts the snapshot to YAML format
func (sg *SnapshotGenerator) ToYAML(snapshot *KonfluxSnapshot) ([]byte, error) {
	return yaml.Marshal(snapshot)
}

// DeduplicateComponents removes duplicate components based on container image
func (sg *SnapshotGenerator) DeduplicateComponents(snapshot *KonfluxSnapshot) {
	seen := make(map[string]bool)
	var uniqueComponents []SnapshotComponent

	for _, component := range snapshot.Spec.Components {
		key := component.ContainerImage
		if !seen[key] {
			seen[key] = true
			uniqueComponents = append(uniqueComponents, component)
		}
	}

	snapshot.Spec.Components = uniqueComponents
}

// ValidateSnapshot performs basic validation on the generated snapshot
func (sg *SnapshotGenerator) ValidateSnapshot(snapshot *KonfluxSnapshot) error {
	if snapshot.Metadata.Name == "" {
		return fmt.Errorf("snapshot name cannot be empty")
	}

	if snapshot.Spec.Application == "" {
		return fmt.Errorf("application name cannot be empty")
	}

	if len(snapshot.Spec.Components) == 0 {
		return fmt.Errorf("snapshot must contain at least one component")
	}

	// Validate component names
	for _, component := range snapshot.Spec.Components {
		if component.Name == "" {
			return fmt.Errorf("component name cannot be empty")
		}
		if component.ContainerImage == "" {
			return fmt.Errorf("component container image cannot be empty")
		}
	}

	return nil
}
