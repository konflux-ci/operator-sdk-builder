package main

import (
	"fmt"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/internal/handlers"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/config"
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
		// Parse CLI flags
		target := args[0]
		mirrorPolicyFile, _ := cmd.Flags().GetString("mirror-policy")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Create request from CLI parameters
		request := handlers.GenerateRelatedImagesRequest{
			Target: target,
			MirrorPolicy: config.MirrorPolicyConfig{
				MirrorPolicyFile: mirrorPolicyFile,
			},
			DryRun: dryRun,
		}

		// Create handler and process the request
		handler := handlers.NewGenerateRelatedImagesHandler()
		result, err := handler.GenerateRelatedImages(request)
		if err != nil {
			return err
		}

		// Display summary
		if len(result.Changes) > 0 {
			fmt.Printf("Summary: %s\n", result.Summary)
		} else {
			fmt.Printf("Summary: %s\n", result.Summary)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(generateRelatedImagesCmd)

	generateRelatedImagesCmd.Flags().StringP("mirror-policy", "m", "", "Path to mirror policy YAML file (ICSP or IDMS)")
	generateRelatedImagesCmd.Flags().Bool("dry-run", false, "Show changes without modifying files")
}

// InitializeCommandsForTesting initializes the commands for testing purposes
func InitializeCommandsForTesting() {
	// Ensure the command is properly initialized for testing
	// This is needed because init() functions are not called during testing
	if generateRelatedImagesCmd.Flags().Lookup("dry-run") == nil {
		// Re-initialize flags if they haven't been set up
		generateRelatedImagesCmd.Flags().StringP("mirror-policy", "m", "", "Path to mirror policy YAML file (ICSP or IDMS)")
		generateRelatedImagesCmd.Flags().Bool("dry-run", false, "Show changes without modifying files")
	}
}
