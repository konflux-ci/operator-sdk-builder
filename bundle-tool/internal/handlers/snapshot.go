package handlers

import (
	"context"
	"fmt"
	"os"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/config"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/provenance"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/resolver"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/snapshot"
)

// SnapshotRequest represents the parameters for generating a snapshot
type SnapshotRequest struct {
	BundleImage  string
	OutputFile   string
	Namespace    string
	AppName      string
	BundleRepo   string
	BundleCommit string
	MirrorPolicy config.MirrorPolicyConfig
}

// SnapshotResult represents the result of snapshot generation
type SnapshotResult struct {
	SnapshotFile    string
	Application     string
	Namespace       string
	ComponentsCount int
	Summary         string
}

// SnapshotHandler handles the business logic for generating Konflux snapshots
type SnapshotHandler struct {
	analyzer         *bundle.BundleAnalyzer
	imageResolver    *resolver.ImageResolver
	provenanceParser *provenance.ProvenanceParser
}

// NewSnapshotHandler creates a new SnapshotHandler
func NewSnapshotHandler() *SnapshotHandler {
	return &SnapshotHandler{
		analyzer:         bundle.NewBundleAnalyzer(),
		imageResolver:    resolver.NewImageResolver(),
		provenanceParser: provenance.NewProvenanceParser(),
	}
}

// GenerateSnapshot processes a snapshot request and generates a Konflux snapshot
func (h *SnapshotHandler) GenerateSnapshot(ctx context.Context, req SnapshotRequest) (*SnapshotResult, error) {
	// Validate the request
	if err := h.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Step 1: Extract image references from bundle
	imageRefs, err := h.extractImageReferences(ctx, req.BundleImage)
	if err != nil {
		return nil, fmt.Errorf("failed to extract image references: %w", err)
	}

	fmt.Printf("Found %d image references in bundle %s:\n", len(imageRefs), req.BundleImage)

	// Step 2: Load mirror policies and resolve image references
	resolvedRefs, err := h.resolveImageReferences(imageRefs, req.MirrorPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve image references: %w", err)
	}

	// Step 3: Parse provenance for all images
	provenanceResults, err := h.parseProvenance(ctx, resolvedRefs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse provenance: %w", err)
	}

	// Display detailed results
	h.displayResults(imageRefs, resolvedRefs, provenanceResults)

	// Step 4: Generate Konflux Snapshot
	konfluxSnapshot, err := h.generateSnapshot(ctx, req, resolvedRefs, provenanceResults)
	if err != nil {
		return nil, fmt.Errorf("failed to generate snapshot: %w", err)
	}

	// Step 5: Output the snapshot to file
	outputFile, err := h.writeSnapshotFile(konfluxSnapshot, req.OutputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to write snapshot to file: %w", err)
	}

	// Create and return the result
	result := &SnapshotResult{
		SnapshotFile:    outputFile,
		Application:     konfluxSnapshot.Spec.Application,
		Namespace:       konfluxSnapshot.Metadata.Namespace,
		ComponentsCount: len(konfluxSnapshot.Spec.Components),
		Summary:         h.createSummary(konfluxSnapshot),
	}

	return result, nil
}

// validateRequest validates the snapshot request parameters
func (h *SnapshotHandler) validateRequest(req SnapshotRequest) error {
	if req.BundleImage == "" {
		return fmt.Errorf("bundle image is required")
	}

	// Validate mirror policy configuration
	mirrorPolicyLoader := config.NewMirrorPolicyLoader(req.MirrorPolicy)
	if err := mirrorPolicyLoader.Validate(); err != nil {
		return fmt.Errorf("invalid mirror policy configuration: %w", err)
	}

	return nil
}

// extractImageReferences extracts image references from the bundle
func (h *SnapshotHandler) extractImageReferences(ctx context.Context, bundleImage string) ([]bundle.ImageReference, error) {
	return h.analyzer.ExtractImageReferences(ctx, bundleImage)
}

// resolveImageReferences loads mirror policies and resolves image references
func (h *SnapshotHandler) resolveImageReferences(imageRefs []bundle.ImageReference, mirrorPolicyConfig config.MirrorPolicyConfig) ([]bundle.ImageReference, error) {
	// Load mirror policy using the unified configuration approach
	mirrorPolicyLoader := config.NewMirrorPolicyLoader(mirrorPolicyConfig)
	if err := mirrorPolicyLoader.LoadIntoResolver(h.imageResolver); err != nil {
		return nil, err
	}

	// Resolve the image references
	return h.imageResolver.ResolveImageReferences(imageRefs)
}

// parseProvenance parses provenance for all resolved images
func (h *SnapshotHandler) parseProvenance(ctx context.Context, resolvedRefs []bundle.ImageReference) ([]provenance.ProvenanceInfo, error) {
	fmt.Println("Parsing image provenance...")
	h.provenanceParser.SetVerbose(true)

	provenanceResults, err := h.provenanceParser.ParseProvenance(ctx, resolvedRefs)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Provenance parsing summary: %+v\n", h.provenanceParser.GetParsingSummary(provenanceResults))
	return provenanceResults, nil
}

// displayResults displays the detailed results of image resolution and provenance parsing
func (h *SnapshotHandler) displayResults(imageRefs, resolvedRefs []bundle.ImageReference, provenanceResults []provenance.ProvenanceInfo) {
	// Display mapping summary
	fmt.Printf("Mapping summary: %+v\n", h.imageResolver.GetMappingSummary())

	// Display resolved image references
	fmt.Printf("Resolved image references:\n")
	for i, ref := range resolvedRefs {
		fmt.Printf("  %d. Original: %s\n", i+1, imageRefs[i].Image)
		fmt.Printf("     Resolved: %s\n", ref.Image)
		if ref.Name != "" {
			fmt.Printf("     Name: %s\n", ref.Name)
		}
		if ref.Digest != "" {
			fmt.Printf("     Digest: %s\n", ref.Digest)
		}

		// Display provenance info if available
		if i < len(provenanceResults) {
			prov := provenanceResults[i]
			fmt.Printf("     Provenance parsed: %t\n", prov.Verified)
			if prov.SourceRepo != "" {
				fmt.Printf("     Source repo: %s\n", prov.SourceRepo)
			}
			if prov.SourceCommit != "" {
				fmt.Printf("     Source commit: %s\n", prov.SourceCommit)
			}
			if prov.ComponentName != "" {
				fmt.Printf("     Component name: %s\n", prov.ComponentName)
			}
			if prov.ApplicationName != "" {
				fmt.Printf("     Application name: %s\n", prov.ApplicationName)
			}
			if prov.Namespace != "" {
				fmt.Printf("     Namespace: %s\n", prov.Namespace)
			}
			if prov.Error != "" {
				fmt.Printf("     Provenance error: %s\n", prov.Error)
			}
		}
		fmt.Println()
	}
}

// generateSnapshot creates the Konflux snapshot from resolved references and provenance results
func (h *SnapshotHandler) generateSnapshot(ctx context.Context, req SnapshotRequest, resolvedRefs []bundle.ImageReference, provenanceResults []provenance.ProvenanceInfo) (*snapshot.KonfluxSnapshot, error) {
	// Create snapshot generator with application name from provenance or fallback values
	generator, err := snapshot.NewSnapshotGeneratorWithSourceFallback(ctx, req.BundleImage, req.Namespace, req.AppName, "", h.provenanceParser)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot generator: %w", err)
	}

	konfluxSnapshot, err := generator.GenerateSnapshotWithBundleSource(ctx, resolvedRefs, provenanceResults, req.BundleImage, req.BundleRepo, req.BundleCommit, h.provenanceParser)
	if err != nil {
		return nil, fmt.Errorf("failed to generate snapshot: %w", err)
	}

	// Deduplicate components and validate
	generator.DeduplicateComponents(konfluxSnapshot)
	if err := generator.ValidateSnapshot(konfluxSnapshot); err != nil {
		return nil, fmt.Errorf("generated snapshot is invalid: %w", err)
	}

	return konfluxSnapshot, nil
}

// writeSnapshotFile writes the snapshot to a YAML file
func (h *SnapshotHandler) writeSnapshotFile(konfluxSnapshot *snapshot.KonfluxSnapshot, outputFile string) (string, error) {
	// Convert to YAML
	generator := snapshot.NewSnapshotGenerator("", "")
	yamlOutput, err := generator.ToYAML(konfluxSnapshot)
	if err != nil {
		return "", fmt.Errorf("failed to convert snapshot to YAML: %w", err)
	}

	// Generate default filename if not specified
	if outputFile == "" {
		outputFile = fmt.Sprintf("%s-snapshot.yaml", konfluxSnapshot.Spec.Application)
	}

	// Output to file
	if err := os.WriteFile(outputFile, yamlOutput, 0644); err != nil {
		return "", fmt.Errorf("failed to write snapshot to file %s: %w", outputFile, err)
	}

	fmt.Printf("Snapshot written to: %s\n", outputFile)
	return outputFile, nil
}

// createSummary creates a human-readable summary of the generated snapshot
func (h *SnapshotHandler) createSummary(konfluxSnapshot *snapshot.KonfluxSnapshot) string {
	return fmt.Sprintf("Application: %s, Namespace: %s, Components: %d",
		konfluxSnapshot.Spec.Application,
		konfluxSnapshot.Metadata.Namespace,
		len(konfluxSnapshot.Spec.Components))
}
