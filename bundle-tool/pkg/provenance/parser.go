package provenance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/sigstore/cosign/v2/pkg/cosign"
	"github.com/sigstore/cosign/v2/pkg/oci/remote"
)

// ProvenanceInfo contains source information extracted from provenance attestations
type ProvenanceInfo struct {
	ImageRef        string            `json:"image_ref"`
	SourceRepo      string            `json:"source_repo,omitempty"`
	SourceCommit    string            `json:"source_commit,omitempty"`
	BuildPlatform   string            `json:"build_platform,omitempty"`
	ComponentName   string            `json:"component_name,omitempty"`
	ApplicationName string            `json:"application_name,omitempty"`
	Namespace       string            `json:"namespace,omitempty"`
	Verified        bool              `json:"verified"`
	Error           string            `json:"error,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// SLSAAttestation represents a unified SLSA provenance structure supporting both v0.1 and v1.0
type SLSAAttestation struct {
	PredicateType string `json:"predicateType"`
	Predicate     struct {
		Builder struct {
			ID string `json:"id"`
		} `json:"builder"`

		// SLSA v1.0 format
		BuildDefinition struct {
			ExternalParameters   map[string]interface{} `json:"externalParameters"`
			ResolvedDependencies []struct {
				URI    string            `json:"uri"`
				Digest map[string]string `json:"digest"`
			} `json:"resolvedDependencies"`
		} `json:"buildDefinition"`
		Invocation struct {
			Environment struct {
				Labels map[string]string `json:"labels"`
			} `json:"environment"`
		} `json:"invocation"`

		// SLSA v0.1 format
		Materials []struct {
			URI    string            `json:"uri"`
			Digest map[string]string `json:"digest"`
		} `json:"materials"`
		Recipe struct {
			Type              string                 `json:"type"`
			DefinedInMaterial int                    `json:"definedInMaterial,omitempty"`
			EntryPoint        string                 `json:"entryPoint,omitempty"`
			Arguments         map[string]interface{} `json:"arguments,omitempty"`
			Environment       map[string]interface{} `json:"environment,omitempty"`
		} `json:"recipe"`

		// Common fields
		RunDetails struct {
			Builder struct {
				ID string `json:"id"`
			} `json:"builder"`
			Metadata struct {
				InvocationID string `json:"invocationId"`
			} `json:"metadata"`
		} `json:"runDetails"`
	} `json:"predicate"`
}

// ProvenanceParser handles parsing of image provenance using cosign SDK
type ProvenanceParser struct {
	verbose           bool
	maxAttestations   int           // Security: Limit number of attestations processed
	maxPayloadSize    int64         // Security: Maximum payload size
	processingTimeout time.Duration // Security: Timeout for long-running operations
}

// NewProvenanceParser creates a new ProvenanceParser with secure defaults
func NewProvenanceParser() *ProvenanceParser {
	return &ProvenanceParser{
		verbose:           false,
		maxAttestations:   50,               // Security: Limit to 50 attestations per image
		maxPayloadSize:    10 * 1024 * 1024, // Security: 10MB limit per payload
		processingTimeout: 30 * time.Second, // Security: 30 second timeout
	}
}

// SetMaxAttestations sets the maximum number of attestations to process per image
func (pp *ProvenanceParser) SetMaxAttestations(max int) {
	if max > 0 && max <= 1000 { // Reasonable upper bound
		pp.maxAttestations = max
	}
}

// SetMaxPayloadSize sets the maximum payload size in bytes
func (pp *ProvenanceParser) SetMaxPayloadSize(size int64) {
	if size > 0 && size <= 100*1024*1024 { // Max 100MB
		pp.maxPayloadSize = size
	}
}

// SetProcessingTimeout sets the timeout for processing operations
func (pp *ProvenanceParser) SetProcessingTimeout(timeout time.Duration) {
	if timeout > 0 && timeout <= 5*time.Minute { // Max 5 minutes
		pp.processingTimeout = timeout
	}
}

// SetVerbose enables verbose output
func (pp *ProvenanceParser) SetVerbose(verbose bool) {
	pp.verbose = verbose
}

// ParseProvenance parses provenance for a list of image references with input validation
func (pp *ProvenanceParser) ParseProvenance(ctx context.Context, imageRefs []bundle.ImageReference) ([]ProvenanceInfo, error) {
	// Security: Limit number of image references processed
	if len(imageRefs) > 1000 {
		return nil, fmt.Errorf("too many image references: %d exceeds maximum of 1000", len(imageRefs))
	}

	var results []ProvenanceInfo

	for i, ref := range imageRefs {
		// Security: Validate image reference
		if err := pp.validateImageReference(ref); err != nil {
			if pp.verbose {
				fmt.Printf("Warning: skipping invalid image reference %d: %v\n", i, err)
			}
			continue
		}

		info := ProvenanceInfo{
			ImageRef: ref.Image,
			Metadata: make(map[string]string),
		}

		// Try to parse and extract provenance
		if pp.verbose {
			fmt.Printf("Debug: Starting provenance parsing for image: %s\n", ref.Image)
		}
		if err := pp.parseImageProvenance(ctx, ref.Image, &info); err != nil {
			info.Verified = false
			info.Error = err.Error()
			if pp.verbose {
				fmt.Printf("Warning: provenance parsing failed for %s: %v\n", ref.Image, err)
			}
		} else {
			info.Verified = true
			if pp.verbose {
				fmt.Printf("Debug: Provenance parsing succeeded for %s, SourceRepo=%s\n", ref.Image, info.SourceRepo)
			}
		}

		results = append(results, info)
	}

	return results, nil
}

// validateImageReference performs security validation on image references
func (pp *ProvenanceParser) validateImageReference(ref bundle.ImageReference) error {
	// Security: Validate image reference length
	if len(ref.Image) > 1024 {
		return fmt.Errorf("image reference too long: %d characters exceeds maximum of 1024", len(ref.Image))
	}

	// Security: Validate name length
	if len(ref.Name) > 256 {
		return fmt.Errorf("image name too long: %d characters exceeds maximum of 256", len(ref.Name))
	}

	// Security: Check for control characters in image reference
	for _, char := range ref.Image {
		if char < 32 || char == 127 {
			return fmt.Errorf("invalid character in image reference")
		}
	}

	// Security: Basic format validation
	if ref.Image == "" {
		return fmt.Errorf("empty image reference")
	}

	return nil
}

// parseImageProvenance parses a single image's provenance using cosign SDK with timeout protection
func (pp *ProvenanceParser) parseImageProvenance(ctx context.Context, imageRef string, info *ProvenanceInfo) error {
	// Security: Create context with timeout to prevent long-running operations
	timeoutCtx, cancel := context.WithTimeout(ctx, pp.processingTimeout)
	defer cancel()

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return fmt.Errorf("failed to parse image reference: %w", err)
	}

	// Get the image's attestations with timeout
	attestations, err := pp.getAttestations(timeoutCtx, ref)
	if err != nil {
		return fmt.Errorf("failed to get attestations: %w", err)
	}

	if len(attestations) == 0 {
		return fmt.Errorf("no provenance attestations found")
	}

	// Security: Limit number of attestations processed
	attestationsToProcess := attestations
	if len(attestations) > pp.maxAttestations {
		if pp.verbose {
			fmt.Printf("Warning: limiting attestations from %d to %d for image %s\n",
				len(attestations), pp.maxAttestations, imageRef)
		}
		attestationsToProcess = attestations[:pp.maxAttestations]
	}

	// Parse attestations with proper memory management
	for i, attestation := range attestationsToProcess {
		// Security: Check for timeout on each iteration
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("provenance parsing timed out after processing %d attestations", i)
		default:
			// Continue processing
		}

		if err := pp.parseAttestationPayload(attestation, info); err != nil {
			if pp.verbose {
				fmt.Printf("Warning: failed to parse attestation %d for %s: %v\n", i, imageRef, err)
			}
			continue
		}
		return nil // Successfully parsed at least one attestation
	}

	return fmt.Errorf("failed to parse any attestations")
}

// getAttestations retrieves attestations for an image using cosign SDK
func (pp *ProvenanceParser) getAttestations(ctx context.Context, ref name.Reference) ([]cosign.AttestationPayload, error) {
	remoteOpts := []remote.Option{}

	// Get all attestations from the image (no predicate type filter)
	attestations, err := cosign.FetchAttestationsForReference(ctx, ref, "", remoteOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch attestations: %w", err)
	}

	return attestations, nil
}

// parseAttestationPayload parses a single attestation payload with memory protection
func (pp *ProvenanceParser) parseAttestationPayload(attestation cosign.AttestationPayload, info *ProvenanceInfo) error {
	// Security: Check raw payload size before processing
	if int64(len(attestation.PayLoad)) > pp.maxPayloadSize {
		return fmt.Errorf("attestation payload too large: %d bytes exceeds maximum of %d bytes",
			len(attestation.PayLoad), pp.maxPayloadSize)
	}

	if pp.verbose {
		fmt.Printf("Debug: Raw attestation payload: %s\n", attestation.PayLoad[:min(100, len(attestation.PayLoad))])
	}

	var decodedPayload []byte
	var err error

	// Security: Handle both base64-encoded and raw JSON payloads gracefully
	// First check if the payload looks like JSON (starts with '{' or '[')
	trimmedPayload := strings.TrimSpace(attestation.PayLoad)
	if strings.HasPrefix(trimmedPayload, "{") || strings.HasPrefix(trimmedPayload, "[") {
		// Payload appears to be raw JSON, use it directly
		decodedPayload = []byte(trimmedPayload)
		if pp.verbose {
			fmt.Printf("Debug: Using raw JSON payload\n")
		}
	} else {
		// Assume it's base64-encoded, attempt to decode
		decodedPayload, err = base64.StdEncoding.DecodeString(attestation.PayLoad)
		if err != nil {
			// If base64 decoding fails, try raw payload as fallback
			decodedPayload = []byte(attestation.PayLoad)
			if pp.verbose {
				fmt.Printf("Debug: Base64 decoding failed, using raw payload: %v\n", err)
			}
		} else if pp.verbose {
			fmt.Printf("Debug: Successfully base64 decoded payload\n")
		}
	}

	// Security: Validate decoded payload size again after decoding to prevent expansion attacks
	if int64(len(decodedPayload)) > pp.maxPayloadSize {
		return fmt.Errorf("decoded payload too large: %d bytes exceeds maximum of %d bytes",
			len(decodedPayload), pp.maxPayloadSize)
	}

	if pp.verbose {
		fmt.Printf("Debug: Decoded payload preview: %s\n", string(decodedPayload[:min(200, len(decodedPayload))]))
	}

	// Parse the data with proper memory management
	err = pp.parseProvenanceData(decodedPayload, info)

	// Security: Explicit cleanup of large byte slices to help GC
	decodedPayload = nil

	return err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseProvenanceData extracts information from SLSA provenance attestation (supports v0.1 and v1.0)
func (pp *ProvenanceParser) parseProvenanceData(attestationJSON []byte, info *ProvenanceInfo) error {
	// Handle both single-line JSON and newline-separated JSON
	data := strings.TrimSpace(string(attestationJSON))

	// If it's a single line JSON, try to parse it directly
	if strings.HasPrefix(data, "{") && !strings.Contains(data, "\n") {
		return pp.parseSingleAttestation([]byte(data), info)
	}

	// Otherwise, handle newline-separated format (cosign output)
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}

		if err := pp.parseSingleAttestation([]byte(line), info); err == nil {
			return nil // Successfully parsed attestation
		}
	}

	return nil
}

// parseSingleAttestation parses a single JSON attestation
func (pp *ProvenanceParser) parseSingleAttestation(attestationJSON []byte, info *ProvenanceInfo) error {
	var attestation SLSAAttestation
	if err := json.Unmarshal(attestationJSON, &attestation); err != nil {
		return fmt.Errorf("failed to unmarshal attestation: %w", err)
	}

	// Only process SLSA provenance attestations
	if !pp.isSLSAProvenance(&attestation) {
		return fmt.Errorf("not a SLSA provenance attestation")
	}

	// Detect SLSA version and extract data accordingly
	if pp.isSLSAv1(&attestation) {
		if pp.verbose {
			fmt.Printf("Debug: Using SLSA v1.0 extraction for predicate type: %s\n", attestation.PredicateType)
		}
		pp.extractFromSLSAv1(&attestation, info)
	} else {
		if pp.verbose {
			fmt.Printf("Debug: Using SLSA v0.1/v0.2 extraction for predicate type: %s\n", attestation.PredicateType)
		}
		pp.extractFromSLSAv01(&attestation, info)
	}

	// Extract build platform (common to both versions)
	if attestation.Predicate.Builder.ID != "" {
		info.BuildPlatform = attestation.Predicate.Builder.ID
	}

	return nil
}

// isSLSAProvenance checks if the attestation is a SLSA provenance attestation
func (pp *ProvenanceParser) isSLSAProvenance(attestation *SLSAAttestation) bool {
	predicateType := attestation.PredicateType
	result := predicateType == "https://slsa.dev/provenance/v0.1" ||
		predicateType == "https://slsa.dev/provenance/v0.2" ||
		predicateType == "https://slsa.dev/provenance/v1" ||
		predicateType == "slsaprovenance" // fallback for non-standard predicate types

	if pp.verbose {
		fmt.Printf("Debug: isSLSAProvenance check - predicateType='%s', result=%t\n", predicateType, result)
	}
	return result
}

// isSLSAv1 detects if the attestation is SLSA v1.0 format
func (pp *ProvenanceParser) isSLSAv1(attestation *SLSAAttestation) bool {
	// SLSA v1.0 has buildDefinition, v0.1 has materials
	// Primary indicator is buildDefinition existence
	return len(attestation.Predicate.BuildDefinition.ResolvedDependencies) > 0 ||
		len(attestation.Predicate.BuildDefinition.ExternalParameters) > 0
}

// extractFromSLSAv1 extracts data from SLSA v1.0 format
func (pp *ProvenanceParser) extractFromSLSAv1(attestation *SLSAAttestation, info *ProvenanceInfo) {
	// Extract source repository information from resolvedDependencies
	if buildDef := attestation.Predicate.BuildDefinition; len(buildDef.ResolvedDependencies) > 0 {
		for _, dep := range buildDef.ResolvedDependencies {
			if strings.Contains(dep.URI, "git+") {
				info.SourceRepo = dep.URI
				if sha, ok := dep.Digest["sha1"]; ok {
					info.SourceCommit = sha
				}
				break
			}
		}
	}

	// Extract component, application names, and namespace from invocation environment labels
	if labels := attestation.Predicate.Invocation.Environment.Labels; len(labels) > 0 {
		if componentName, ok := labels["appstudio.openshift.io/component"]; ok {
			info.ComponentName = componentName
		}
		if applicationName, ok := labels["appstudio.openshift.io/application"]; ok {
			info.ApplicationName = applicationName
		}
		if namespace, ok := labels["appstudio.openshift.io/namespace"]; ok {
			info.Namespace = namespace
		}
	}

	// Extract external parameters as metadata
	if extParams := attestation.Predicate.BuildDefinition.ExternalParameters; len(extParams) > 0 {
		for key, value := range extParams {
			if strVal, ok := value.(string); ok {
				info.Metadata[key] = strVal
			}
		}
	}
}

// extractFromSLSAv01 extracts data from SLSA v0.1 format
func (pp *ProvenanceParser) extractFromSLSAv01(attestation *SLSAAttestation, info *ProvenanceInfo) {
	// Extract source repository information from materials
	if len(attestation.Predicate.Materials) > 0 {
		if pp.verbose {
			fmt.Printf("Debug: Found %d materials in SLSA v0.1/v0.2 attestation\n", len(attestation.Predicate.Materials))
		}
		for i, material := range attestation.Predicate.Materials {
			if pp.verbose {
				fmt.Printf("Debug: Material %d: URI=%s\n", i, material.URI)
			}
			if strings.Contains(material.URI, "git+") {
				info.SourceRepo = material.URI
				if sha, ok := material.Digest["sha1"]; ok {
					info.SourceCommit = sha
				}
				if pp.verbose {
					fmt.Printf("Debug: Extracted git source: repo=%s, commit=%s\n", info.SourceRepo, info.SourceCommit)
				}
				break
			}
		}
	} else if pp.verbose {
		fmt.Printf("Debug: No materials found in SLSA v0.1/v0.2 attestation\n")
	}

	// Extract component, application names, and namespace - Tekton Chains uses invocation.environment.labels even in SLSA v0.1
	if labels := attestation.Predicate.Invocation.Environment.Labels; len(labels) > 0 {
		if componentName, ok := labels["appstudio.openshift.io/component"]; ok {
			info.ComponentName = componentName
		}
		if applicationName, ok := labels["appstudio.openshift.io/application"]; ok {
			info.ApplicationName = applicationName
		}
		if namespace, ok := labels["appstudio.openshift.io/namespace"]; ok {
			info.Namespace = namespace
		}
	}

	// Fallback: check recipe.environment for component, application names, and namespace
	if info.ComponentName == "" || info.ApplicationName == "" || info.Namespace == "" {
		if recipe := attestation.Predicate.Recipe; len(recipe.Environment) > 0 {
			if info.ComponentName == "" {
				if componentName, ok := recipe.Environment["appstudio.openshift.io/component"]; ok {
					if strVal, ok := componentName.(string); ok {
						info.ComponentName = strVal
					}
				}
			}
			if info.ApplicationName == "" {
				if applicationName, ok := recipe.Environment["appstudio.openshift.io/application"]; ok {
					if strVal, ok := applicationName.(string); ok {
						info.ApplicationName = strVal
					}
				}
			}
			if info.Namespace == "" {
				if namespace, ok := recipe.Environment["appstudio.openshift.io/namespace"]; ok {
					if strVal, ok := namespace.(string); ok {
						info.Namespace = strVal
					}
				}
			}
		}
	}

	// Fallback: check recipe arguments for component, application names, and namespace
	if info.ComponentName == "" || info.ApplicationName == "" || info.Namespace == "" {
		if recipe := attestation.Predicate.Recipe; len(recipe.Arguments) > 0 {
			if info.ComponentName == "" {
				if componentName, ok := recipe.Arguments["appstudio.openshift.io/component"]; ok {
					if strVal, ok := componentName.(string); ok {
						info.ComponentName = strVal
					}
				}
			}
			if info.ApplicationName == "" {
				if applicationName, ok := recipe.Arguments["appstudio.openshift.io/application"]; ok {
					if strVal, ok := applicationName.(string); ok {
						info.ApplicationName = strVal
					}
				}
			}
			if info.Namespace == "" {
				if namespace, ok := recipe.Arguments["appstudio.openshift.io/namespace"]; ok {
					if strVal, ok := namespace.(string); ok {
						info.Namespace = strVal
					}
				}
			}
		}
	}

	// Extract recipe environment and arguments as metadata
	if recipe := attestation.Predicate.Recipe; len(recipe.Environment) > 0 {
		for key, value := range recipe.Environment {
			if strVal, ok := value.(string); ok {
				info.Metadata[key] = strVal
			}
		}
	}
	if recipe := attestation.Predicate.Recipe; len(recipe.Arguments) > 0 {
		for key, value := range recipe.Arguments {
			if strVal, ok := value.(string); ok {
				info.Metadata[key] = strVal
			}
		}
	}
}

// ExtractComponentName extracts the component name from a single image's provenance
func (pp *ProvenanceParser) ExtractComponentName(ctx context.Context, imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	// Get attestations
	attestations, err := pp.getAttestations(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("failed to get attestations: %w", err)
	}

	if len(attestations) == 0 {
		return "", fmt.Errorf("no attestations found")
	}

	// Parse attestations to extract component name
	info := ProvenanceInfo{Metadata: make(map[string]string)}
	for _, attestation := range attestations {
		if err := pp.parseAttestationPayload(attestation, &info); err != nil {
			continue // Try next attestation
		}

		if info.ComponentName != "" {
			return info.ComponentName, nil
		}
	}

	return "", fmt.Errorf("component name not found in provenance")
}

// ExtractApplicationName extracts the application name from a single image's provenance
func (pp *ProvenanceParser) ExtractApplicationName(ctx context.Context, imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	// Get attestations
	attestations, err := pp.getAttestations(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("failed to get attestations: %w", err)
	}

	if len(attestations) == 0 {
		return "", fmt.Errorf("no attestations found")
	}

	// Parse attestations to extract application name
	info := ProvenanceInfo{Metadata: make(map[string]string)}
	for _, attestation := range attestations {
		if err := pp.parseAttestationPayload(attestation, &info); err != nil {
			continue // Try next attestation
		}

		if info.ApplicationName != "" {
			return info.ApplicationName, nil
		}
	}

	return "", fmt.Errorf("application name not found in provenance")
}

// ExtractNamespace extracts the namespace from a single image's provenance
func (pp *ProvenanceParser) ExtractNamespace(ctx context.Context, imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	// Get attestations
	attestations, err := pp.getAttestations(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("failed to get attestations: %w", err)
	}

	if len(attestations) == 0 {
		return "", fmt.Errorf("no attestations found")
	}

	// Parse attestations to extract namespace
	info := ProvenanceInfo{Metadata: make(map[string]string)}
	for _, attestation := range attestations {
		if err := pp.parseAttestationPayload(attestation, &info); err != nil {
			continue // Try next attestation
		}

		if info.Namespace != "" {
			return info.Namespace, nil
		}
	}

	return "", fmt.Errorf("namespace not found in provenance")
}

// GetParsingSummary returns a summary of parsing results
func (pp *ProvenanceParser) GetParsingSummary(results []ProvenanceInfo) map[string]interface{} {
	total := len(results)
	verified := 0
	withSource := 0

	for _, result := range results {
		if result.Verified {
			verified++
		}
		if result.SourceRepo != "" {
			withSource++
		}
	}

	return map[string]interface{}{
		"total_images":       total,
		"verified_images":    verified,
		"images_with_source": withSource,
		"verification_rate":  float64(verified) / float64(total) * 100,
	}
}
