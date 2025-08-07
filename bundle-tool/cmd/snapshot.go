package main

import (
	"context"
	"fmt"
	"os"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/provenance"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/resolver"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/snapshot"
	"github.com/spf13/cobra"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot [bundle-image]",
	Short: "Create a Konflux snapshot from an OLM bundle image",
	Long: `Analyze an OLM bundle image, resolve image references using ICSP/IDMS policies,
extract source information from SLSA provenance, and generate a Konflux snapshot YAML.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bundleImage := args[0]
		icspFile, _ := cmd.Flags().GetString("icsp")
		idmsFile, _ := cmd.Flags().GetString("idms")
		mirrorPolicyFile, _ := cmd.Flags().GetString("mirror-policy")
		outputFile, _ := cmd.Flags().GetString("output")
		namespace, _ := cmd.Flags().GetString("namespace")
		appName, _ := cmd.Flags().GetString("application")
		bundleRepo, _ := cmd.Flags().GetString("bundle-repo")
		bundleCommit, _ := cmd.Flags().GetString("bundle-commit")

		// Step 1: Extract image references from bundle
		analyzer := bundle.NewBundleAnalyzer()
		imageRefs, err := analyzer.ExtractImageReferences(context.Background(), bundleImage)
		if err != nil {
			return fmt.Errorf("failed to extract image references: %w", err)
		}

		fmt.Printf("Found %d image references in bundle %s:\n", len(imageRefs), bundleImage)

		// Step 2: Resolve image references using ICSP/IDMS
		imageResolver := resolver.NewImageResolver()

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

		// Resolve the image references
		resolvedRefs, err := imageResolver.ResolveImageReferences(imageRefs)
		if err != nil {
			return fmt.Errorf("failed to resolve image references: %w", err)
		}

		// Step 3: Parse provenance for all images
		fmt.Println("Parsing image provenance...")
		parser := provenance.NewProvenanceParser()
		parser.SetVerbose(true)

		provenanceResults, err := parser.ParseProvenance(context.Background(), resolvedRefs)
		if err != nil {
			return fmt.Errorf("failed to parse provenance: %w", err)
		}

		fmt.Printf("Provenance parsing summary: %+v\n", parser.GetParsingSummary(provenanceResults))

		// Display results
		fmt.Printf("Mapping summary: %+v\n", imageResolver.GetMappingSummary())
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

		// Step 4: Generate Konflux Snapshot
		// Note: namespace is optional - when omitted, generated YAML has no namespace field

		// Create snapshot generator with application name from provenance or fallback values
		provenanceParser := provenance.NewProvenanceParser()
		provenanceParser.SetVerbose(true)
		generator, err := snapshot.NewSnapshotGeneratorWithSourceFallback(context.Background(), bundleImage, namespace, appName, "", provenanceParser)
		if err != nil {
			return fmt.Errorf("failed to create snapshot generator: %w", err)
		}
		
		konfluxSnapshot, err := generator.GenerateSnapshotWithBundleSource(context.Background(), resolvedRefs, provenanceResults, bundleImage, bundleRepo, bundleCommit, provenanceParser)
		if err != nil {
			return fmt.Errorf("failed to generate snapshot: %w", err)
		}

		// Deduplicate components and validate
		generator.DeduplicateComponents(konfluxSnapshot)
		if err := generator.ValidateSnapshot(konfluxSnapshot); err != nil {
			return fmt.Errorf("generated snapshot is invalid: %w", err)
		}

		// Convert to YAML
		yamlOutput, err := generator.ToYAML(konfluxSnapshot)
		if err != nil {
			return fmt.Errorf("failed to convert snapshot to YAML: %w", err)
		}

		// Generate default filename if not specified
		if outputFile == "" {
			outputFile = fmt.Sprintf("%s-snapshot.yaml", konfluxSnapshot.Spec.Application)
		}

		// Output to file
		if err := os.WriteFile(outputFile, yamlOutput, 0644); err != nil {
			return fmt.Errorf("failed to write snapshot to file %s: %w", outputFile, err)
		}
		fmt.Printf("Snapshot written to: %s\n", outputFile)

		// Also show summary on stdout
		fmt.Println("\nGenerated Konflux Snapshot Summary:")
		fmt.Printf("  Application: %s\n", konfluxSnapshot.Spec.Application)
		fmt.Printf("  Namespace: %s\n", konfluxSnapshot.Metadata.Namespace)
		fmt.Printf("  Components: %d\n", len(konfluxSnapshot.Spec.Components))
		fmt.Printf("  File: %s\n", outputFile)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(snapshotCmd)

	snapshotCmd.Flags().StringP("mirror-policy", "m", "", "Path to mirror policy YAML file (ICSP or IDMS)")
	snapshotCmd.Flags().StringP("icsp", "i", "", "Path to ImageContentSourcePolicy YAML file (deprecated, use --mirror-policy)")
	snapshotCmd.Flags().StringP("idms", "d", "", "Path to ImageDigestMirrorSet YAML file (deprecated, use --mirror-policy)")
	snapshotCmd.Flags().StringP("output", "o", "", "Output file for generated snapshot (default: stdout)")
	snapshotCmd.Flags().StringP("namespace", "n", "", "Target namespace for the snapshot (optional - omit to use current namespace when applying)")
	snapshotCmd.Flags().StringP("application", "a", "", "Application name (required when bundle provenance unavailable)")
	snapshotCmd.Flags().StringP("bundle-repo", "", "", "Bundle source repository URL (required for build-time usage when bundle provenance unavailable)")
	snapshotCmd.Flags().StringP("bundle-commit", "", "", "Bundle source commit SHA (required for build-time usage when bundle provenance unavailable)")
}
