package handlers

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/config"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/resolver"
	"github.com/operator-framework/operator-manifest-tools/pkg/imagename"
	"github.com/operator-framework/operator-manifest-tools/pkg/pullspec"
)

// GenerateRelatedImagesRequest represents the parameters for generating related images
type GenerateRelatedImagesRequest struct {
	Target       string
	MirrorPolicy config.MirrorPolicyConfig
	DryRun       bool
}

// ImageChange represents a change from an original image to an updated image
type ImageChange struct {
	Original string
	Updated  string
}

// GenerateRelatedImagesResult represents the result of the generate related images operation
type GenerateRelatedImagesResult struct {
	CSVFile     string
	Changes     []ImageChange
	ImagesCount int
	DryRun      bool
	Summary     string
}

// GenerateRelatedImagesHandler handles the business logic for generating related images in CSV files
type GenerateRelatedImagesHandler struct {
	imageResolver *resolver.ImageResolver
}

// NewGenerateRelatedImagesHandler creates a new GenerateRelatedImagesHandler
func NewGenerateRelatedImagesHandler() *GenerateRelatedImagesHandler {
	return &GenerateRelatedImagesHandler{
		imageResolver: resolver.NewImageResolver(),
	}
}

// GenerateRelatedImages processes a request to generate and update relatedImages in a CSV file
func (h *GenerateRelatedImagesHandler) GenerateRelatedImages(req GenerateRelatedImagesRequest) (*GenerateRelatedImagesResult, error) {
	// Validate the request
	if err := h.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Step 1: Find and load CSV files
	csvDir, err := h.findCSVDirectory(req.Target)
	if err != nil {
		return nil, fmt.Errorf("failed to find CSV directory: %w", err)
	}

	fmt.Printf("Processing CSV directory: %s\n", csvDir)

	csv, err := h.loadCSVFile(csvDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load CSV file: %w", err)
	}

	// Step 2: Extract image references from CSV
	imageRefs, err := h.extractImageReferences(csv)
	if err != nil {
		return nil, fmt.Errorf("failed to extract image references: %w", err)
	}

	if len(imageRefs) == 0 {
		fmt.Println("No image references found in CSV")
		return &GenerateRelatedImagesResult{
			CSVFile:     csvDir,
			Changes:     []ImageChange{},
			ImagesCount: 0,
			DryRun:      req.DryRun,
			Summary:     "No images found in CSV",
		}, nil
	}

	fmt.Printf("Found %d image references in CSV\n", len(imageRefs))

	// Step 3: Setup mirror policy and resolve images (optional)
	resolvedRefs, changes, err := h.resolveImageReferences(imageRefs, req.MirrorPolicy)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve image references: %w", err)
	}

	// Step 4: Update CSV with related images
	err = h.updateCSVWithRelatedImages(csv, resolvedRefs, changes, req.DryRun)
	if err != nil {
		return nil, fmt.Errorf("failed to update CSV: %w", err)
	}

	// Create and return result
	result := &GenerateRelatedImagesResult{
		CSVFile:     csvDir,
		Changes:     changes,
		ImagesCount: len(imageRefs),
		DryRun:      req.DryRun,
		Summary:     h.createSummary(len(imageRefs), len(changes), req.DryRun),
	}

	return result, nil
}

// validateRequest validates the generate related images request parameters
func (h *GenerateRelatedImagesHandler) validateRequest(req GenerateRelatedImagesRequest) error {
	if req.Target == "" {
		return fmt.Errorf("target directory or file is required")
	}

	// Validate mirror policy configuration
	mirrorPolicyLoader := config.NewMirrorPolicyLoader(req.MirrorPolicy)
	if err := mirrorPolicyLoader.Validate(); err != nil {
		return fmt.Errorf("invalid mirror policy configuration: %w", err)
	}

	return nil
}

// findCSVDirectory determines the directory containing CSV files
func (h *GenerateRelatedImagesHandler) findCSVDirectory(target string) (string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("failed to stat target: %w", err)
	}

	if info.IsDir() {
		// It's a directory - look for manifests subdirectory
		manifestsDir := filepath.Join(target, "manifests")
		if _, err := os.Stat(manifestsDir); os.IsNotExist(err) {
			return "", fmt.Errorf("manifests directory not found in %s", target)
		}
		return manifestsDir, nil
	} else {
		// It's a file - use its directory
		return filepath.Dir(target), nil
	}
}

// loadCSVFile loads CSV files from the specified directory
func (h *GenerateRelatedImagesHandler) loadCSVFile(csvDir string) (*pullspec.OperatorCSV, error) {
	// Load CSVs using operator-manifest-tools
	csvs, err := pullspec.FromDirectory(csvDir, pullspec.DefaultHeuristic)
	if err != nil {
		return nil, fmt.Errorf("failed to load CSVs from directory: %w", err)
	}

	if len(csvs) == 0 {
		return nil, fmt.Errorf("no CSV files found in directory: %s", csvDir)
	}

	if len(csvs) > 1 {
		return nil, fmt.Errorf("multiple CSV files found, only one expected")
	}

	return csvs[0], nil
}

// extractImageReferences extracts image references from the CSV file
func (h *GenerateRelatedImagesHandler) extractImageReferences(csv *pullspec.OperatorCSV) ([]bundle.ImageReference, error) {
	// Extract image references using operator-manifest-tools
	pullSpecs, err := csv.GetPullSpecs()
	if err != nil {
		return nil, fmt.Errorf("failed to extract image references from CSV: %w", err)
	}

	// Convert pullspecs to our ImageReference format
	var imageRefs []bundle.ImageReference
	for _, ps := range pullSpecs {
		imageRefs = append(imageRefs, bundle.ImageReference{
			Image: ps.String(),
			Name:  ps.Repo,
		})
	}

	return imageRefs, nil
}

// resolveImageReferences loads mirror policies and resolves image references
func (h *GenerateRelatedImagesHandler) resolveImageReferences(imageRefs []bundle.ImageReference, mirrorPolicyConfig config.MirrorPolicyConfig) ([]bundle.ImageReference, []ImageChange, error) {
	// Setup image resolver if mirror policy is configured
	mirrorPolicyLoader := config.NewMirrorPolicyLoader(mirrorPolicyConfig)
	hasMirrorPolicy := mirrorPolicyLoader.HasMirrorPolicy()

	var resolvedRefs []bundle.ImageReference
	var changes []ImageChange

	if hasMirrorPolicy {
		// Load mirror policy using the unified configuration approach
		if err := mirrorPolicyLoader.LoadIntoResolver(h.imageResolver); err != nil {
			return nil, nil, err
		}

		// Resolve image references
		var err error
		resolvedRefs, err = h.imageResolver.ResolveImageReferences(imageRefs)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to resolve image references: %w", err)
		}

		fmt.Printf("Image mapping summary: %+v\n", h.imageResolver.GetMappingSummary())

		// Create changes list for resolved images
		for i, orig := range imageRefs {
			if i < len(resolvedRefs) && orig.Image != resolvedRefs[i].Image {
				changes = append(changes, ImageChange{
					Original: orig.Image,
					Updated:  resolvedRefs[i].Image,
				})
			}
		}
	} else {
		// No mirror policy - use original images
		resolvedRefs = imageRefs
		fmt.Println("No mirror policy provided - using original image references")
	}

	return resolvedRefs, changes, nil
}

// updateCSVWithRelatedImages updates the CSV file with related images and optionally applies image replacements
func (h *GenerateRelatedImagesHandler) updateCSVWithRelatedImages(csv *pullspec.OperatorCSV, resolvedRefs []bundle.ImageReference, changes []ImageChange, dryRun bool) error {
	// Update relatedImages in CSV using operator-manifest-tools
	err := csv.SetRelatedImages()
	if err != nil {
		return fmt.Errorf("failed to set related images in CSV: %w", err)
	}

	// Apply image replacements if there are any
	if len(changes) > 0 {
		fmt.Printf("Made %d changes:\n", len(changes))
		for _, change := range changes {
			fmt.Printf("  %s -> %s\n", change.Original, change.Updated)
		}

		// Get the original pullspecs for replacement mapping
		pullSpecs, err := csv.GetPullSpecs()
		if err != nil {
			return fmt.Errorf("failed to get pull specs for replacement: %w", err)
		}

		// Create replacement map
		replacements := make(map[imagename.ImageName]imagename.ImageName)
		for _, change := range changes {
			// Find the corresponding pullspec and create replacement
			for _, ps := range pullSpecs {
				if ps.String() == change.Original {
					// Parse new image name and add to replacement map
					newImage := imagename.Parse(change.Updated)
					replacements[*ps] = *newImage
					break
				}
			}
		}

		// Apply replacements
		err = csv.ReplacePullSpecs(replacements)
		if err != nil {
			return fmt.Errorf("failed to replace pull specs in CSV: %w", err)
		}
	} else {
		fmt.Println("No mirror policy changes - using original images in relatedImages")
	}

	if dryRun {
		fmt.Println("Dry run mode - no changes written to file")
		return nil
	}

	// Write updated CSV back to file using operator-manifest-tools
	err = csv.Dump(nil)
	if err != nil {
		return fmt.Errorf("failed to write updated CSV: %w", err)
	}

	fmt.Printf("Successfully updated CSV file\n")
	return nil
}

// createSummary creates a human-readable summary of the operation
func (h *GenerateRelatedImagesHandler) createSummary(imagesCount, changesCount int, dryRun bool) string {
	status := "updated"
	if dryRun {
		status = "would be updated"
	}

	if changesCount > 0 {
		return fmt.Sprintf("CSV %s with %d images, %d mirror policy changes applied", status, imagesCount, changesCount)
	}
	return fmt.Sprintf("CSV %s with %d images, no mirror policy changes", status, imagesCount)
}
