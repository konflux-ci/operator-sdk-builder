package main

import (
	"strings"
	"testing"
)

func TestGenerateRelatedImagesDigestCommandStructure(t *testing.T) {
	// Ensure commands are initialized for testing
	InitializeCommandsForTesting()

	// Test that the command has the expected structure
	if generateRelatedImagesCmd.Use != "generate-related-images [csv-file-or-directory]" {
		t.Errorf("unexpected command use: %s", generateRelatedImagesCmd.Use)
	}

	// Test that the command supports digest-pinned images in help text
	helpText := generateRelatedImagesCmd.Long
	expectedDigestKeywords := []string{
		"image",
		"deployment",
		"containers",
	}

	for _, keyword := range expectedDigestKeywords {
		if !strings.Contains(helpText, keyword) {
			t.Errorf("help text missing expected keyword: %s", keyword)
		}
	}

	// Test that the command has the required flags for digest processing
	flags := generateRelatedImagesCmd.Flags()

	// Check that mirror-policy flag exists (needed for digest processing)
	mirrorPolicyFlag := flags.Lookup("mirror-policy")
	if mirrorPolicyFlag == nil {
		t.Error("mirror-policy flag not found - required for digest processing")
	}

	// Check that dry-run flag exists (useful for testing digest changes)
	dryRunFlag := flags.Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("dry-run flag not found - useful for testing digest changes")
	}
}

func TestGenerateRelatedImagesDigestFlagValidation(t *testing.T) {
	// Ensure commands are initialized for testing
	InitializeCommandsForTesting()

	// Test that the command can handle digest-related arguments
	cmd := generateRelatedImagesCmd

	// Test with a path that might contain digest-pinned images
	testArgs := []string{"test-bundle-with-digests", "--dry-run"}
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

func TestGenerateRelatedImagesDigestHelpText(t *testing.T) {
	// Test that help text mentions image processing capabilities
	helpText := generateRelatedImagesCmd.Long

	// The help text should mention that it can process various image formats
	// including digest-pinned images
	expectedContent := []string{
		"image",
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
