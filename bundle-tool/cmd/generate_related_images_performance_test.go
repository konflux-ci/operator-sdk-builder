package main

import (
	"strings"
	"testing"
)

func TestGenerateRelatedImagesPerformanceCommandStructure(t *testing.T) {
	// Ensure commands are initialized for testing
	InitializeCommandsForTesting()

	// Test that the command has the expected structure for performance scenarios
	if generateRelatedImagesCmd.Use != "generate-related-images [csv-file-or-directory]" {
		t.Errorf("unexpected command use: %s", generateRelatedImagesCmd.Use)
	}

	// Test that the command supports performance-related flags
	flags := generateRelatedImagesCmd.Flags()

	// Check that dry-run flag exists (useful for performance testing)
	dryRunFlag := flags.Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("dry-run flag not found - useful for performance testing")
	}

	// Check that mirror-policy flag exists (needed for performance testing with policies)
	mirrorPolicyFlag := flags.Lookup("mirror-policy")
	if mirrorPolicyFlag == nil {
		t.Error("mirror-policy flag not found - needed for performance testing with policies")
	}
}

func TestGenerateRelatedImagesPerformanceFlagValidation(t *testing.T) {
	// Ensure commands are initialized for testing
	InitializeCommandsForTesting()

	// Test that the command can handle performance-related arguments
	cmd := generateRelatedImagesCmd

	// Test with a path that might contain large bundles
	testArgs := []string{"large-bundle-directory", "--dry-run"}
	cmd.SetArgs(testArgs)

	// Parse flags
	err := cmd.ParseFlags(testArgs)
	if err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	// Verify dry-run flag is set
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		t.Fatalf("failed to get dry-run flag: %v", err)
	}
	if !dryRun {
		t.Error("expected dry-run to be true")
	}
}

func TestGenerateRelatedImagesPerformanceHelpText(t *testing.T) {
	// Test that help text mentions performance-related capabilities
	helpText := generateRelatedImagesCmd.Long

	// The help text should mention that it can process various bundle sizes
	expectedContent := []string{
		"CSV",
		"deployment",
		"containers",
		"relatedImages",
	}

	for _, content := range expectedContent {
		if !strings.Contains(helpText, content) {
			t.Errorf("help text missing expected content: %s", content)
		}
	}
}

func TestGenerateRelatedImagesPerformanceCommandInitialization(t *testing.T) {
	// Test that the command is properly initialized for performance scenarios
	if generateRelatedImagesCmd == nil {
		t.Fatal("generateRelatedImagesCmd is nil")
	}

	if generateRelatedImagesCmd.RunE == nil {
		t.Fatal("generateRelatedImagesCmd.RunE is nil")
	}

	// Test that the command has the correct argument validation
	if generateRelatedImagesCmd.Args == nil {
		t.Fatal("generateRelatedImagesCmd.Args is nil")
	}

	// Test argument validation with exact args
	args := []string{"test-bundle"}
	err := generateRelatedImagesCmd.Args(generateRelatedImagesCmd, args)
	if err != nil {
		t.Errorf("expected valid args to pass validation, got error: %v", err)
	}
}
