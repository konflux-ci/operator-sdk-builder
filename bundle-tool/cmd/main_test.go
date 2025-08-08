package main

import (
	"testing"
)

func TestRootCommand(t *testing.T) {
	// Test that root command initializes without errors
	if rootCmd == nil {
		t.Fatal("rootCmd should not be nil")
	}
	
	if rootCmd.Use != "bundle-tool" {
		t.Errorf("Expected root command use to be 'bundle-tool', got '%s'", rootCmd.Use)
	}
	
	// Check that commands are registered
	commands := rootCmd.Commands()
	if len(commands) < 2 {
		t.Errorf("Expected at least 2 subcommands, got %d", len(commands))
	}
	
	// Check for specific commands
	var hasSnapshot, hasGenerateRelated bool
	for _, cmd := range commands {
		switch cmd.Use {
		case "snapshot [bundle-image]":
			hasSnapshot = true
		case "generate-related-images [csv-file-or-directory]":
			hasGenerateRelated = true
		}
	}
	
	if !hasSnapshot {
		t.Error("snapshot command not found")
	}
	
	if !hasGenerateRelated {
		t.Error("generate-related-images command not found")
	}
}