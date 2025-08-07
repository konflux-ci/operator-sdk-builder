package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/resolver"
	"github.com/operator-framework/operator-manifest-tools/pkg/imagename"
	"github.com/operator-framework/operator-manifest-tools/pkg/pullspec"
	"github.com/spf13/cobra"
)

var generateRelatedImagesCmd = &cobra.Command{
	Use:   "generate-related-images [csv-file-or-directory]",
	Short: "Generate and update relatedImages in ClusterServiceVersion",
	Long: `Generate and update the relatedImages section in a ClusterServiceVersion YAML file 
by extracting image references from deployment containers and optionally resolving them using 
ICSP/IDMS policies.

This ensures all deployment images are properly listed in spec.relatedImages.

If a directory is provided, it will search for CSV files in the manifests subdirectory.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		icspFile, _ := cmd.Flags().GetString("icsp")
		idmsFile, _ := cmd.Flags().GetString("idms")
		mirrorPolicyFile, _ := cmd.Flags().GetString("mirror-policy")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Use operator-manifest-tools to extract images from CSV
		var csvDir string
		info, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("failed to stat target: %w", err)
		}

		if info.IsDir() {
			// It's a directory - look for manifests subdirectory
			manifestsDir := filepath.Join(target, "manifests")
			if _, err := os.Stat(manifestsDir); os.IsNotExist(err) {
				return fmt.Errorf("manifests directory not found in %s", target)
			}
			csvDir = manifestsDir
		} else {
			// It's a file - use its directory
			csvDir = filepath.Dir(target)
		}

		fmt.Printf("Processing CSV directory: %s\n", csvDir)

		// Load CSVs using operator-manifest-tools
		csvs, err := pullspec.FromDirectory(csvDir, pullspec.DefaultHeuristic)
		if err != nil {
			return fmt.Errorf("failed to load CSVs from directory: %w", err)
		}

		if len(csvs) == 0 {
			return fmt.Errorf("no CSV files found in directory: %s", csvDir)
		}

		if len(csvs) > 1 {
			return fmt.Errorf("multiple CSV files found, only one expected")
		}

		csv := csvs[0]

		// Extract image references using operator-manifest-tools
		pullSpecs, err := csv.GetPullSpecs()
		if err != nil {
			return fmt.Errorf("failed to extract image references from CSV: %w", err)
		}

		if len(pullSpecs) == 0 {
			fmt.Println("No image references found in CSV")
			return nil
		}

		fmt.Printf("Found %d image references in CSV\n", len(pullSpecs))

		// Convert pullspecs to our ImageReference format
		var imageRefs []bundle.ImageReference
		for _, ps := range pullSpecs {
			imageRefs = append(imageRefs, bundle.ImageReference{
				Image: ps.String(),
				Name:  ps.Repo,
			})
		}

		// Setup image resolver (optional)
		var imageResolver *resolver.ImageResolver
		hasMirrorPolicy := false

		if mirrorPolicyFile != "" || icspFile != "" || idmsFile != "" {
			imageResolver = resolver.NewImageResolver()
			hasMirrorPolicy = true

			// Load mirror policy (unified approach)
			if mirrorPolicyFile != "" {
				if err := imageResolver.LoadMirrorPolicy(mirrorPolicyFile); err != nil {
					return fmt.Errorf("failed to load mirror policy: %w", err)
				}
			} else {
				// Backward compatibility: Load ICSP if provided
				if icspFile != "" {
					if err := imageResolver.LoadICSP(icspFile); err != nil {
						return fmt.Errorf("failed to load ICSP: %w", err)
					}
				}

				// Load IDMS if provided
				if idmsFile != "" {
					if err := imageResolver.LoadIDMS(idmsFile); err != nil {
						return fmt.Errorf("failed to load IDMS: %w", err)
					}
				}
			}
		}

		// Resolve image references (optional)
		var resolvedRefs []bundle.ImageReference
		if hasMirrorPolicy {
			var err error
			resolvedRefs, err = imageResolver.ResolveImageReferences(imageRefs)
			if err != nil {
				return fmt.Errorf("failed to resolve image references: %w", err)
			}
		} else {
			// No mirror policy - use original images
			resolvedRefs = imageRefs
		}

		// Create replacement map for resolved images
		replacements := make(map[imagename.ImageName]imagename.ImageName)
		var changes []ImageChange
		
		if hasMirrorPolicy {
			fmt.Printf("Image mapping summary: %+v\n", imageResolver.GetMappingSummary())
			
			// Create replacements for resolved images
			for i, orig := range imageRefs {
				if i < len(resolvedRefs) && orig.Image != resolvedRefs[i].Image {
					changes = append(changes, ImageChange{
						Original: orig.Image,
						Updated:  resolvedRefs[i].Image,
					})
				}
			}
		} else {
			fmt.Println("No mirror policy provided - using original image references")
		}

		// Update relatedImages in CSV using operator-manifest-tools
		err = csv.SetRelatedImages()
		if err != nil {
			return fmt.Errorf("failed to set related images in CSV: %w", err)
		}

		// Apply image replacements if there are any
		if len(changes) > 0 {
			fmt.Printf("Made %d changes:\n", len(changes))
			for _, change := range changes {
				fmt.Printf("  %s -> %s\n", change.Original, change.Updated)
			}
			
			// Create replacement map
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
	},
}

type ImageChange struct {
	Original string
	Updated  string
}


func init() {
	rootCmd.AddCommand(generateRelatedImagesCmd)

	generateRelatedImagesCmd.Flags().StringP("mirror-policy", "m", "", "Path to mirror policy YAML file (ICSP or IDMS)")
	generateRelatedImagesCmd.Flags().StringP("icsp", "i", "", "Path to ImageContentSourcePolicy YAML file (deprecated, use --mirror-policy)")
	generateRelatedImagesCmd.Flags().StringP("idms", "d", "", "Path to ImageDigestMirrorSet YAML file (deprecated, use --mirror-policy)")
	generateRelatedImagesCmd.Flags().Bool("dry-run", false, "Show changes without modifying files")
}