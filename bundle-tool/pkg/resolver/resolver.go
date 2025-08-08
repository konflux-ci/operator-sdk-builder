package resolver

import (
	"fmt"
	"os"
	"strings"

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
	for _, policy := range ir.icspPolicies {
		for _, mirror := range policy.Spec.RepositoryDigestMirrors {
			if resolved, matched := ir.matchAndReplace(imageRef, mirror.Source, mirror.Mirrors); matched {
				return resolved, true
			}
		}
	}
	return imageRef, false
}

// resolveWithIDMS attempts to resolve an image reference using IDMS policies
func (ir *ImageResolver) resolveWithIDMS(imageRef string) (string, bool) {
	for _, policy := range ir.idmsPolicies {
		for _, mirror := range policy.Spec.ImageDigestMirrors {
			if resolved, matched := ir.matchAndReplace(imageRef, mirror.Source, mirror.Mirrors); matched {
				return resolved, true
			}
		}
	}
	return imageRef, false
}

// matchAndReplace performs the actual image reference matching and replacement
func (ir *ImageResolver) matchAndReplace(imageRef, source string, mirrors []string) (string, bool) {
	// Extract registry and repository from the image reference
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

// GetMappingSummary returns a summary of loaded mapping policies
func (ir *ImageResolver) GetMappingSummary() map[string]interface{} {
	return map[string]interface{}{
		"icsp_policies_count": len(ir.icspPolicies),
		"idms_policies_count": len(ir.idmsPolicies),
		"total_icsp_mirrors":  ir.countICSPMirrors(),
		"total_idms_mirrors":  ir.countIDMSMirrors(),
	}
}

func (ir *ImageResolver) countICSPMirrors() int {
	count := 0
	for _, policy := range ir.icspPolicies {
		count += len(policy.Spec.RepositoryDigestMirrors)
	}
	return count
}

func (ir *ImageResolver) countIDMSMirrors() int {
	count := 0
	for _, policy := range ir.idmsPolicies {
		count += len(policy.Spec.ImageDigestMirrors)
	}
	return count
}
