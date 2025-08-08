package snapshot

import (
	"context"
	"fmt"
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
	appName   string
	namespace string
}

// NewSnapshotGenerator creates a new SnapshotGenerator
func NewSnapshotGenerator(appName, namespace string) *SnapshotGenerator {
	return &SnapshotGenerator{
		appName:   appName,
		namespace: namespace,
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
		appName:   appName,
		namespace: namespace,
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

// generateComponentName creates a valid Kubernetes resource name from an image reference
func (sg *SnapshotGenerator) generateComponentName(ref bundle.ImageReference) string {
	name := ref.Name
	if name == "" {
		// Extract component name from image reference
		parts := strings.Split(ref.Image, "/")
		imageName := parts[len(parts)-1]

		// Remove tag/digest
		if idx := strings.Index(imageName, ":"); idx != -1 {
			imageName = imageName[:idx]
		}
		if idx := strings.Index(imageName, "@"); idx != -1 {
			imageName = imageName[:idx]
		}

		name = imageName
	}

	// Ensure valid Kubernetes name
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, ".", "-")

	// Remove invalid characters and ensure it starts/ends with alphanumeric
	var cleanName strings.Builder
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r == '-' && i > 0 && i < len(name)-1) {
			cleanName.WriteRune(r)
		}
	}

	result := cleanName.String()
	if result == "" {
		result = "component"
	}

	// Ensure it doesn't start or end with hyphen
	result = strings.Trim(result, "-")
	if result == "" {
		result = "component"
	}

	return result
}

// cleanGitURL converts provenance git URLs to standard format
func (sg *SnapshotGenerator) cleanGitURL(provenanceURL string) string {
	// Handle git+ prefix
	if strings.HasPrefix(provenanceURL, "git+") {
		provenanceURL = strings.TrimPrefix(provenanceURL, "git+")
	}

	// Handle various URL formats
	if strings.HasPrefix(provenanceURL, "https://github.com/") {
		return provenanceURL
	}

	if strings.HasPrefix(provenanceURL, "github.com/") {
		return "https://" + provenanceURL
	}

	return provenanceURL
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
