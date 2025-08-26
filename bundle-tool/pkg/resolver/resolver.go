package resolver

import (
	"fmt"
	"os"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"gopkg.in/yaml.v3"
)

// ImageContentSourcePolicy represents the ICSP structure
type ImageContentSourcePolicy struct {
	APIVersion string                       `yaml:"apiVersion"`
	Kind       string                       `yaml:"kind"`
	Metadata   map[string]interface{}       `yaml:"metadata"`
	Spec       ImageContentSourcePolicySpec `yaml:"spec"`
}

type ImageContentSourcePolicySpec struct {
	RepositoryDigestMirrors []RepositoryDigestMirror `yaml:"repositoryDigestMirrors"`
}

type RepositoryDigestMirror struct {
	Source  string   `yaml:"source"`
	Mirrors []string `yaml:"mirrors"`
}

// ImageDigestMirrorSet represents the IDMS structure (newer OpenShift versions)
type ImageDigestMirrorSet struct {
	APIVersion string                   `yaml:"apiVersion"`
	Kind       string                   `yaml:"kind"`
	Metadata   map[string]interface{}   `yaml:"metadata"`
	Spec       ImageDigestMirrorSetSpec `yaml:"spec"`
}

type ImageDigestMirrorSetSpec struct {
	ImageDigestMirrors []ImageDigestMirror `yaml:"imageDigestMirrors"`
}

type ImageDigestMirror struct {
	Source  string   `yaml:"source"`
	Mirrors []string `yaml:"mirrors"`
}

// ImageResolver handles mapping of image references using ICSP/IDMS policies
type ImageResolver struct {
	icspPolicies []ImageContentSourcePolicy
	idmsPolicies []ImageDigestMirrorSet
}

// NewImageResolver creates a new ImageResolver
func NewImageResolver() *ImageResolver {
	return &ImageResolver{
		icspPolicies: []ImageContentSourcePolicy{},
		idmsPolicies: []ImageDigestMirrorSet{},
	}
}

// LoadICSP loads ImageContentSourcePolicy from a YAML file
func (ir *ImageResolver) LoadICSP(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read ICSP file %s: %w", filePath, err)
	}

	var icsp ImageContentSourcePolicy
	if err := yaml.Unmarshal(data, &icsp); err != nil {
		return fmt.Errorf("failed to unmarshal ICSP from %s: %w", filePath, err)
	}

	if icsp.Kind != "ImageContentSourcePolicy" {
		return fmt.Errorf("file %s is not an ImageContentSourcePolicy (kind: %s)", filePath, icsp.Kind)
	}

	ir.icspPolicies = append(ir.icspPolicies, icsp)
	return nil
}

// LoadIDMS loads ImageDigestMirrorSet from a YAML file
func (ir *ImageResolver) LoadIDMS(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read IDMS file %s: %w", filePath, err)
	}

	var idms ImageDigestMirrorSet
	if err := yaml.Unmarshal(data, &idms); err != nil {
		return fmt.Errorf("failed to unmarshal IDMS from %s: %w", filePath, err)
	}

	if idms.Kind != "ImageDigestMirrorSet" {
		return fmt.Errorf("file %s is not an ImageDigestMirrorSet (kind: %s)", filePath, idms.Kind)
	}

	ir.idmsPolicies = append(ir.idmsPolicies, idms)
	return nil
}

// LoadMirrorPolicy loads either ICSP or IDMS from a YAML file by auto-detecting the format
func (ir *ImageResolver) LoadMirrorPolicy(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read mirror policy file %s: %w", filePath, err)
	}

	// Parse just enough to determine the kind
	var kindCheck struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &kindCheck); err != nil {
		return fmt.Errorf("failed to parse YAML from %s: %w", filePath, err)
	}

	switch kindCheck.Kind {
	case "ImageContentSourcePolicy":
		return ir.LoadICSP(filePath)
	case "ImageDigestMirrorSet":
		return ir.LoadIDMS(filePath)
	default:
		return fmt.Errorf("unsupported mirror policy kind in %s: %s (expected ImageContentSourcePolicy or ImageDigestMirrorSet)", filePath, kindCheck.Kind)
	}
}

// ResolveImageReferences maps image references using loaded ICSP/IDMS policies
func (ir *ImageResolver) ResolveImageReferences(imageRefs []bundle.ImageReference) ([]bundle.ImageReference, error) {
	var resolvedRefs []bundle.ImageReference

	for _, ref := range imageRefs {
		resolvedRef := ref // copy original

		// Try to resolve using ICSP policies first
		if resolved, found := ir.resolveWithICSP(ref.Image); found {
			resolvedRef.Image = resolved
		} else if resolved, found := ir.resolveWithIDMS(ref.Image); found {
			// Try IDMS if ICSP didn't match
			resolvedRef.Image = resolved
		}

		resolvedRefs = append(resolvedRefs, resolvedRef)
	}

	return resolvedRefs, nil
}

// resolveWithICSP attempts to resolve an image reference using ICSP policies
func (ir *ImageResolver) resolveWithICSP(imageRef string) (string, bool) {
	var bestMatch string
	var bestScore int
	var found bool

	for _, policy := range ir.icspPolicies {
		for _, mirror := range policy.Spec.RepositoryDigestMirrors {
			if resolved, matched := ir.matchAndReplace(imageRef, mirror.Source, mirror.Mirrors); matched {
				// Calculate specificity score (longer source = more specific = higher score)
				score := len(mirror.Source)
				if score > bestScore {
					bestScore = score
					bestMatch = resolved
					found = true
				}
			}
		}
	}

	if found {
		return bestMatch, true
	}
	return imageRef, false
}

// resolveWithIDMS attempts to resolve an image reference using IDMS policies
func (ir *ImageResolver) resolveWithIDMS(imageRef string) (string, bool) {
	var bestMatch string
	var bestScore int
	var found bool

	for _, policy := range ir.idmsPolicies {
		for _, mirror := range policy.Spec.ImageDigestMirrors {
			if resolved, matched := ir.matchAndReplace(imageRef, mirror.Source, mirror.Mirrors); matched {
				// Calculate specificity score (longer source = more specific = higher score)
				score := len(mirror.Source)
				if score > bestScore {
					bestScore = score
					bestMatch = resolved
					found = true
				}
			}
		}
	}

	if found {
		return bestMatch, true
	}
	return imageRef, false
}

// matchAndReplace performs the actual image reference matching and replacement using robust parsing
func (ir *ImageResolver) matchAndReplace(imageRef, source string, mirrors []string) (string, bool) {
	if len(mirrors) == 0 {
		return imageRef, false
	}

	// Parse the image reference using containers/image/v5
	parsed, err := ir.parseImageReference(imageRef)
	if err != nil {
		// If parsing fails, fall back to the original string-based approach for backward compatibility
		return ir.matchAndReplaceFallback(imageRef, source, mirrors)
	}

	// Handle the case where the source is just a registry name (like "docker.io")
	// This is a common pattern in mirror policies
	if !strings.Contains(source, "/") {
		// Source is just a registry - check if it matches the image registry exactly
		// Use strict domain boundary matching to avoid partial matches
		if ir.isExactRegistryMatch(parsed.Registry, source) {
			newParsed := *parsed

			// Handle the mirror - it could be just a registry or have a repository path
			if strings.Contains(mirrors[0], "/") {
				// Mirror has a repository path (e.g., "quay.io/redhat-user-workloads")
				parts := strings.SplitN(mirrors[0], "/", 2)
				newParsed.Registry = parts[0]
				if len(parts) > 1 {
					// Combine mirror repository with original repository
					newParsed.Repository = parts[1] + "/" + parsed.Repository
				}
			} else {
				// Mirror is just a registry (e.g., "mirror.dockerhub.com")
				newParsed.Registry = mirrors[0]
				// Keep the original repository
			}

			return ir.reconstructImageReference(&newParsed, newParsed.Registry), true
		}
	}

	// Try to parse the source as an image reference
	sourceParsed, sourceErr := ir.parseImageReference(source)

	// Handle parsed source scenarios
	if sourceErr == nil {
		// Check for exact registry match (source is just a registry)
		if parsed.Registry == sourceParsed.Registry && sourceParsed.Repository == "" {
			newParsed := *parsed
			newParsed.Registry = mirrors[0]
			return ir.reconstructImageReference(&newParsed, newParsed.Registry), true
		}

		// Check for exact registry and repository match
		if parsed.Registry == sourceParsed.Registry && parsed.Repository == sourceParsed.Repository {
			newParsed := *parsed
			newParsed.Registry = mirrors[0]
			return ir.reconstructImageReference(&newParsed, newParsed.Registry), true
		}

		// Check for repository prefix match (registry/repo-prefix)
		fullSourceRepo := sourceParsed.Registry + "/" + sourceParsed.Repository
		fullImageRepo := parsed.Registry + "/" + parsed.Repository

		if ir.isValidPrefixMatch(fullImageRepo, fullSourceRepo) {
			remainder := strings.TrimPrefix(fullImageRepo, fullSourceRepo)
			remainder = strings.TrimPrefix(remainder, "/")

			newParsed := *parsed
			if remainder == "" {
				newParsed.Registry = mirrors[0]
			} else {
				newParsed.Registry = mirrors[0]
				newParsed.Repository = remainder
			}
			return ir.reconstructImageReference(&newParsed, newParsed.Registry), true
		}
	}

	// If source parsing failed or no matches found, try simple string matching
	return ir.matchSourceAsPrefix(parsed, source, mirrors)
}

// matchSourceAsPrefix handles cases where source parsing fails, treating source as a simple prefix
func (ir *ImageResolver) matchSourceAsPrefix(parsed *ParsedImageRef, source string, mirrors []string) (string, bool) {
	fullImage := parsed.Registry + "/" + parsed.Repository

	if ir.isValidPrefixMatch(fullImage, source) {
		remainder := strings.TrimPrefix(fullImage, source)
		remainder = strings.TrimPrefix(remainder, "/")

		newParsed := *parsed
		if remainder == "" {
			// Exact match - replace registry
			newParsed.Registry = mirrors[0]
		} else {
			// Prefix match - use mirror as registry and remainder as repository
			newParsed.Registry = mirrors[0]
			newParsed.Repository = remainder
		}
		return ir.reconstructImageReference(&newParsed, newParsed.Registry), true
	}

	// Check if source matches just the registry
	if parsed.Registry == source {
		newParsed := *parsed
		newParsed.Registry = mirrors[0]
		return ir.reconstructImageReference(&newParsed, newParsed.Registry), true
	}

	return parsed.Original, false
}

// matchAndReplaceFallback provides backward compatibility for cases where parsing fails
func (ir *ImageResolver) matchAndReplaceFallback(imageRef, source string, mirrors []string) (string, bool) {
	// Extract registry and repository from the image reference using simple string splitting
	parts := strings.SplitN(imageRef, "/", 2)
	if len(parts) < 2 {
		return imageRef, false
	}

	registry := parts[0]
	repoAndTag := parts[1]

	// Check for exact registry match
	if registry == source {
		if len(mirrors) > 0 {
			// Use the first mirror
			return mirrors[0] + "/" + repoAndTag, true
		}
	}

	// Check for repository prefix match
	repoPath := repoAndTag
	// Remove tag/digest to get the repository path
	if strings.Contains(repoPath, "@") {
		repoPath = strings.Split(repoPath, "@")[0]
	} else if strings.Contains(repoPath, ":") {
		repoPath = strings.Split(repoPath, ":")[0]
	}
	fullRepo := registry + "/" + repoPath

	if strings.HasPrefix(fullRepo, source) {
		if len(mirrors) > 0 {
			// Replace the source prefix with the first mirror
			remainder := strings.TrimPrefix(fullRepo, source)
			remainder = strings.TrimPrefix(remainder, "/")

			// Preserve tag/digest from original reference
			var tagOrDigest string
			if strings.Contains(repoAndTag, "@") {
				// Handle digest format
				digestParts := strings.SplitN(repoAndTag, "@", 2)
				if len(digestParts) == 2 {
					tagOrDigest = "@" + digestParts[1]
				}
			} else if strings.Contains(repoAndTag, ":") {
				// Handle tag format
				tagParts := strings.SplitN(repoAndTag, ":", 2)
				if len(tagParts) == 2 {
					tagOrDigest = ":" + tagParts[1]
				}
			}

			if remainder != "" {
				return mirrors[0] + "/" + remainder + tagOrDigest, true
			} else {
				return mirrors[0] + tagOrDigest, true
			}
		}
	}

	return imageRef, false
}

// ParsedImageRef contains parsed components of an image reference
type ParsedImageRef struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
	Original   string
}

// parseImageReference parses an image reference using containers/image/v5 library
func (ir *ImageResolver) parseImageReference(imageRef string) (*ParsedImageRef, error) {
	// Parse the image reference using containers/image/v5
	ref, err := reference.ParseAnyReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference %s: %w", imageRef, err)
	}

	parsed := &ParsedImageRef{
		Original: imageRef,
	}

	// Handle different reference types
	switch r := ref.(type) {
	case reference.Named:
		// Get registry (domain)
		if domain := reference.Domain(r); domain != "" {
			parsed.Registry = domain
		}

		// Get repository path
		parsed.Repository = reference.Path(r)

		// Handle tagged references
		if tagged, ok := r.(reference.Tagged); ok {
			parsed.Tag = tagged.Tag()
		}

		// Handle digested references
		if digested, ok := r.(reference.Digested); ok {
			parsed.Digest = digested.Digest().String()
		}

	default:
		// Handle other reference types (like digest-only references)
		return nil, fmt.Errorf("unsupported image reference format: %s", imageRef)
	}

	// If no registry is specified, assume docker.io for standard Docker Hub images
	if parsed.Registry == "" {
		parsed.Registry = "docker.io"
	}

	// Normalize Docker Hub registry names
	if parsed.Registry == "docker.io" && !strings.Contains(parsed.Repository, "/") {
		// For official images, prepend "library/"
		parsed.Repository = "library/" + parsed.Repository
	}

	return parsed, nil
}

// reconstructImageReference rebuilds an image reference from parsed components
func (ir *ImageResolver) reconstructImageReference(parsed *ParsedImageRef, newRegistry string) string {
	var result strings.Builder

	// Add registry
	result.WriteString(newRegistry)

	// Add repository path
	if parsed.Repository != "" {
		result.WriteString("/")
		result.WriteString(parsed.Repository)
	}

	// Add tag or digest
	if parsed.Digest != "" {
		result.WriteString("@")
		result.WriteString(parsed.Digest)
	} else if parsed.Tag != "" {
		result.WriteString(":")
		result.WriteString(parsed.Tag)
	}

	return result.String()
}

// GetMappingSummary returns a summary of loaded mapping policies
func (ir *ImageResolver) GetMappingSummary() map[string]interface{} {
	return map[string]interface{}{
		"icsp_policies_count": len(ir.icspPolicies),
		"idms_policies_count": len(ir.idmsPolicies),
		"total_icsp_mirrors":  ir.countICSPMirrors(),
		"total_idms_mirrors":  ir.countIDMSMirrors(),
	}
}

// MirrorStats represents statistics about loaded mirror policies
type MirrorStats struct {
	TotalPolicies int
	TotalMirrors  int
}

// GetMirrorStats returns statistics about loaded mirror policies
func (ir *ImageResolver) GetMirrorStats() MirrorStats {
	return MirrorStats{
		TotalPolicies: len(ir.icspPolicies) + len(ir.idmsPolicies),
		TotalMirrors:  ir.countICSPMirrors() + ir.countIDMSMirrors(),
	}
}

func (ir *ImageResolver) countICSPMirrors() int {
	count := 0
	for _, policy := range ir.icspPolicies {
		for _, mirror := range policy.Spec.RepositoryDigestMirrors {
			count += len(mirror.Mirrors)
		}
	}
	return count
}

func (ir *ImageResolver) countIDMSMirrors() int {
	count := 0
	for _, policy := range ir.idmsPolicies {
		for _, mirror := range policy.Spec.ImageDigestMirrors {
			count += len(mirror.Mirrors)
		}
	}
	return count
}

// isValidPrefixMatch checks if a prefix match is valid according to OpenShift's design
func (ir *ImageResolver) isValidPrefixMatch(fullImageRepo, fullSourceRepo string) bool {
	// Check if the image repository starts with the source repository
	if !strings.HasPrefix(fullImageRepo, fullSourceRepo) {
		return false
	}

	// If it's an exact match, it's valid
	if fullImageRepo == fullSourceRepo {
		return true
	}

	// For prefix matches, we need to check domain boundaries
	// registry.redhat.io should NOT match registry.redhat.io.evil.com
	if !strings.Contains(fullSourceRepo, "/") {
		// Source is registry-only, check domain boundaries
		if len(fullImageRepo) > len(fullSourceRepo) {
			nextChar := fullImageRepo[len(fullSourceRepo) : len(fullSourceRepo)+1]
			// Only allow '/' (path separator) for registry-only sources
			// This prevents registry.redhat.io from matching registry.redhat.io.evil.com
			return nextChar == "/"
		}
		return false
	}

	// For repository paths, allow prefix matching (this is the OpenShift behavior)
	// This allows "quay.io/operator" to match "quay.io/operator-sdk"
	// and "registry.redhat.io/ubi8/nodejs" to match "registry.redhat.io/ubi8/nodejs-16"
	return true
}

// isExactRegistryMatch checks if two registry names match exactly
func (ir *ImageResolver) isExactRegistryMatch(registry1, registry2 string) bool {
	// Exact string match (including ports)
	return registry1 == registry2
}
