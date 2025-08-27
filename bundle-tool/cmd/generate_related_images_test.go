package main

import (
	"strings"
	"testing"
)

func TestGenerateRelatedImagesCommandStructure(t *testing.T) {
	// Ensure commands are initialized for testing
	InitializeCommandsForTesting()

	// Test that the command has the expected structure
	if generateRelatedImagesCmd.Use != "generate-related-images [csv-file-or-directory]" {
		t.Errorf("unexpected command use: %s", generateRelatedImagesCmd.Use)
	}

	if generateRelatedImagesCmd.Short != "Generate and update relatedImages in ClusterServiceVersion" {
		t.Errorf("unexpected command short description: %s", generateRelatedImagesCmd.Short)
	}

	// Check that the command has the expected long description
	expectedLongDesc := "Generate and update the relatedImages section in a ClusterServiceVersion YAML file"
	if !strings.Contains(generateRelatedImagesCmd.Long, expectedLongDesc) {
		t.Errorf("expected long description to contain: %s", expectedLongDesc)
	}

	// Check flags
	flags := generateRelatedImagesCmd.Flags()

	expectedFlags := []string{"mirror-policy", "dry-run"}
	for _, flagName := range expectedFlags {
		if flags.Lookup(flagName) == nil {
			t.Errorf("expected flag '%s' not found", flagName)
		}
	}

	// Check flag details
	mirrorPolicyFlag := flags.Lookup("mirror-policy")
	if mirrorPolicyFlag == nil {
		t.Fatal("mirror-policy flag not found")
	}
	if mirrorPolicyFlag.Shorthand != "m" {
		t.Errorf("expected mirror-policy shorthand 'm', got '%s'", mirrorPolicyFlag.Shorthand)
	}

	dryRunFlag := flags.Lookup("dry-run")
	if dryRunFlag == nil {
		t.Fatal("dry-run flag not found")
	}
	if dryRunFlag.Value.Type() != "bool" {
		t.Errorf("expected dry-run flag to be bool type, got %s", dryRunFlag.Value.Type())
	}
}

func TestGenerateRelatedImagesCommandInitialization(t *testing.T) {
	// Test that the command is properly initialized
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
	args := []string{"test-arg"}
	err := generateRelatedImagesCmd.Args(generateRelatedImagesCmd, args)
	if err != nil {
		t.Errorf("expected valid args to pass validation, got error: %v", err)
	}

	// Test argument validation with wrong number of args
	invalidArgs := []string{}
	err = generateRelatedImagesCmd.Args(generateRelatedImagesCmd, invalidArgs)
	if err == nil {
		t.Error("expected invalid args to fail validation")
	}

	tooManyArgs := []string{"arg1", "arg2"}
	err = generateRelatedImagesCmd.Args(generateRelatedImagesCmd, tooManyArgs)
	if err == nil {
		t.Error("expected too many args to fail validation")
	}
}

func TestGenerateRelatedImagesCommandFlags(t *testing.T) {
	// Ensure commands are initialized for testing
	InitializeCommandsForTesting()

	// Test flag parsing without resetting flags
	cmd := generateRelatedImagesCmd

	// Test setting args and flags
	testArgs := []string{"test-directory", "--mirror-policy", "test-policy.yaml", "--dry-run"}
	cmd.SetArgs(testArgs)

	// Parse flags
	err := cmd.ParseFlags(testArgs)
	if err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	// Verify flag values
	mirrorPolicy, err := cmd.Flags().GetString("mirror-policy")
	if err != nil {
		t.Fatalf("failed to get mirror-policy flag: %v", err)
	}
	if mirrorPolicy != "test-policy.yaml" {
		t.Errorf("expected mirror-policy 'test-policy.yaml', got '%s'", mirrorPolicy)
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		t.Fatalf("failed to get dry-run flag: %v", err)
	}
	if !dryRun {
		t.Error("expected dry-run to be true")
	}
}

func TestGenerateRelatedImagesCommandHelp(t *testing.T) {
	// Test that help text is available
	if generateRelatedImagesCmd.Use == "" {
		t.Error("command Use field is empty")
	}

	if generateRelatedImagesCmd.Short == "" {
		t.Error("command Short field is empty")
	}

	if generateRelatedImagesCmd.Long == "" {
		t.Error("command Long field is empty")
	}

	// Test that help text contains expected keywords
	helpText := generateRelatedImagesCmd.Long
	expectedKeywords := []string{
		"relatedImages",
		"ClusterServiceVersion",
		"deployment",
		"ICSP",
		"IDMS",
	}

	for _, keyword := range expectedKeywords {
		if !strings.Contains(helpText, keyword) {
			t.Errorf("help text missing expected keyword: %s", keyword)
		}
	}
}
