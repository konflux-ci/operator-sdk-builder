package main

import (
	"context"
	"fmt"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/internal/handlers"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/config"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot [bundle-image]",
	Short: "Create a Konflux snapshot from an OLM bundle image",
	Long: `Analyze an OLM bundle image, resolve image references using ICSP/IDMS policies,
extract source information from SLSA provenance, and generate a Konflux snapshot YAML.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse CLI flags
		bundleImage := args[0]
		mirrorPolicyFile, _ := cmd.Flags().GetString("mirror-policy")
		outputFile, _ := cmd.Flags().GetString("output")
		namespace, _ := cmd.Flags().GetString("namespace")
		appName, _ := cmd.Flags().GetString("application")
		bundleRepo, _ := cmd.Flags().GetString("bundle-repo")
		bundleCommit, _ := cmd.Flags().GetString("bundle-commit")

		// Create request from CLI parameters
		request := handlers.SnapshotRequest{
			BundleImage:  bundleImage,
			OutputFile:   outputFile,
			Namespace:    namespace,
			AppName:      appName,
			BundleRepo:   bundleRepo,
			BundleCommit: bundleCommit,
			MirrorPolicy: config.MirrorPolicyConfig{
				MirrorPolicyFile: mirrorPolicyFile,
			},
		}

		// Create handler and process the request
		handler := handlers.NewSnapshotHandler()
		result, err := handler.GenerateSnapshot(context.Background(), request)
		if err != nil {
			return err
		}

		// Display summary
		fmt.Println("\nGenerated Konflux Snapshot Summary:")
		fmt.Printf("  Application: %s\n", result.Application)
		fmt.Printf("  Namespace: %s\n", result.Namespace)
		fmt.Printf("  Components: %d\n", result.ComponentsCount)
		fmt.Printf("  File: %s\n", result.SnapshotFile)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(snapshotCmd)

	snapshotCmd.Flags().StringP("mirror-policy", "m", "", "Path to mirror policy YAML file (ICSP or IDMS)")
	snapshotCmd.Flags().StringP("output", "o", "", "Output file for generated snapshot (default: stdout)")
	snapshotCmd.Flags().StringP("namespace", "n", "", "Target namespace for the snapshot (optional - omit to use current namespace when applying)")
	snapshotCmd.Flags().StringP("application", "a", "", "Application name (required when bundle provenance unavailable)")
	snapshotCmd.Flags().StringP("bundle-repo", "", "", "Bundle source repository URL (required for build-time usage when bundle provenance unavailable)")
	snapshotCmd.Flags().StringP("bundle-commit", "", "", "Bundle source commit SHA (required for build-time usage when bundle provenance unavailable)")
}
