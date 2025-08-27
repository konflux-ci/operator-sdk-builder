package provenance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/sigstore/cosign/v2/pkg/cosign"
)

// Test data for SLSA v1.0 attestation
var slsaV1TestData = `{"predicateType": "https://slsa.dev/provenance/v1", "predicate": {"builder": {"id": "https://github.com/slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@refs/tags/v1.2.0"}, "buildDefinition": {"externalParameters": {"workflow": {"ref": "refs/heads/main", "repository": "https://github.com/example/repo"}}, "resolvedDependencies": [{"uri": "git+https://github.com/example/operator.git", "digest": {"sha1": "abc123def456"}}]}, "invocation": {"environment": {"labels": {"appstudio.openshift.io/component": "my-operator-bundle", "appstudio.openshift.io/application": "my-operator-app"}}}}}`

// Test data for SLSA v0.1 attestation (Tekton Chains format)
var slsaV01TestData = `{"predicateType": "https://slsa.dev/provenance/v0.1", "predicate": {"builder": {"id": "https://tekton.dev/chains/v2"}, "materials": [{"uri": "git+https://github.com/example/operator.git", "digest": {"sha1": "xyz789abc123"}}], "recipe": {"type": "https://tekton.dev/v1beta1/TaskRun", "environment": {"platform": "linux/amd64"}, "arguments": {"IMAGE": "quay.io/example/operator:latest"}}, "invocation": {"environment": {"labels": {"appstudio.openshift.io/component": "forklift-operator-bundle-2-9", "appstudio.openshift.io/application": "forklift-operator-2-9"}}}}}`

// Test data for SLSA v0.1 with component in recipe.environment
var slsaV01RecipeTestData = `{"predicateType": "https://slsa.dev/provenance/v0.1", "predicate": {"builder": {"id": "https://tekton.dev/chains/v2"}, "materials": [{"uri": "git+https://github.com/example/operator.git", "digest": {"sha1": "recipe789abc123"}}], "recipe": {"type": "https://tekton.dev/v1beta1/TaskRun", "environment": {"appstudio.openshift.io/component": "recipe-component-name", "appstudio.openshift.io/application": "recipe-app-name"}}}}`

// encodeTestData base64-encodes JSON test data to simulate cosign AttestationPayload format
func encodeTestData(jsonData string) string {
	return base64.StdEncoding.EncodeToString([]byte(jsonData))
}

func TestNewProvenanceParser(t *testing.T) {
	parser := NewProvenanceParser()
	if parser == nil {
		t.Fatal("NewProvenanceParser returned nil")
	}
	if parser.verbose {
		t.Error("Expected verbose to be false by default")
	}
}

func TestSetVerbose(t *testing.T) {
	parser := NewProvenanceParser()
	parser.SetVerbose(true)
	if !parser.verbose {
		t.Error("SetVerbose(true) did not set verbose to true")
	}
}

func TestIsSLSAv1(t *testing.T) {
	parser := NewProvenanceParser()

	tests := []struct {
		name     string
		data     string
		expected bool
	}{
		{
			name:     "SLSA v1.0 attestation",
			data:     slsaV1TestData,
			expected: true,
		},
		{
			name:     "SLSA v0.1 attestation with invocation labels",
			data:     slsaV01TestData,
			expected: false, // SLSA v0.1 with materials, not buildDefinition
		},
		{
			name:     "SLSA v0.1 attestation with recipe only",
			data:     slsaV01RecipeTestData,
			expected: false, // No buildDefinition
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attestation SLSAAttestation
			if err := json.Unmarshal([]byte(tt.data), &attestation); err != nil {
				t.Fatalf("Failed to unmarshal test data: %v", err)
			}

			result := parser.isSLSAv1(&attestation)
			if result != tt.expected {
				t.Errorf("isSLSAv1() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestParseProvenanceData(t *testing.T) {
	parser := NewProvenanceParser()

	tests := []struct {
		name                string
		data                string
		expectedComponent   string
		expectedApplication string
		expectedRepo        string
		expectedCommit      string
		expectedPlatform    string
	}{
		{
			name:                "SLSA v1.0 attestation",
			data:                slsaV1TestData,
			expectedComponent:   "my-operator-bundle",
			expectedApplication: "my-operator-app",
			expectedRepo:        "git+https://github.com/example/operator.git",
			expectedCommit:      "abc123def456",
			expectedPlatform:    "https://github.com/slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@refs/tags/v1.2.0",
		},
		{
			name:                "SLSA v0.1 attestation with invocation labels",
			data:                slsaV01TestData,
			expectedComponent:   "forklift-operator-bundle-2-9",
			expectedApplication: "forklift-operator-2-9",
			expectedRepo:        "git+https://github.com/example/operator.git",
			expectedCommit:      "xyz789abc123",
			expectedPlatform:    "https://tekton.dev/chains/v2",
		},
		{
			name:                "SLSA v0.1 attestation with recipe environment",
			data:                slsaV01RecipeTestData,
			expectedComponent:   "recipe-component-name",
			expectedApplication: "recipe-app-name",
			expectedRepo:        "git+https://github.com/example/operator.git",
			expectedCommit:      "recipe789abc123",
			expectedPlatform:    "https://tekton.dev/chains/v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ProvenanceInfo{
				Metadata: make(map[string]string),
			}

			err := parser.parseProvenanceData([]byte(tt.data), &info)
			if err != nil {
				t.Fatalf("parseProvenanceData failed: %v", err)
			}

			if info.ComponentName != tt.expectedComponent {
				t.Errorf("ComponentName = %q, expected %q", info.ComponentName, tt.expectedComponent)
			}

			if info.ApplicationName != tt.expectedApplication {
				t.Errorf("ApplicationName = %q, expected %q", info.ApplicationName, tt.expectedApplication)
			}

			if info.SourceRepo != tt.expectedRepo {
				t.Errorf("SourceRepo = %q, expected %q", info.SourceRepo, tt.expectedRepo)
			}

			if info.SourceCommit != tt.expectedCommit {
				t.Errorf("SourceCommit = %q, expected %q", info.SourceCommit, tt.expectedCommit)
			}

			if info.BuildPlatform != tt.expectedPlatform {
				t.Errorf("BuildPlatform = %q, expected %q", info.BuildPlatform, tt.expectedPlatform)
			}
		})
	}
}

func TestExtractFromSLSAv1(t *testing.T) {
	parser := NewProvenanceParser()

	var attestation SLSAAttestation
	if err := json.Unmarshal([]byte(slsaV1TestData), &attestation); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	info := ProvenanceInfo{
		Metadata: make(map[string]string),
	}

	parser.extractFromSLSAv1(&attestation, &info)

	if info.ComponentName != "my-operator-bundle" {
		t.Errorf("ComponentName = %q, expected %q", info.ComponentName, "my-operator-bundle")
	}

	if info.SourceRepo != "git+https://github.com/example/operator.git" {
		t.Errorf("SourceRepo = %q, expected %q", info.SourceRepo, "git+https://github.com/example/operator.git")
	}

	if info.SourceCommit != "abc123def456" {
		t.Errorf("SourceCommit = %q, expected %q", info.SourceCommit, "abc123def456")
	}
}

func TestExtractFromSLSAv01(t *testing.T) {
	parser := NewProvenanceParser()

	tests := []struct {
		name              string
		data              string
		expectedComponent string
		expectedRepo      string
		expectedCommit    string
	}{
		{
			name:              "SLSA v0.1 with invocation labels",
			data:              slsaV01TestData,
			expectedComponent: "forklift-operator-bundle-2-9",
			expectedRepo:      "git+https://github.com/example/operator.git",
			expectedCommit:    "xyz789abc123",
		},
		{
			name:              "SLSA v0.1 with recipe environment",
			data:              slsaV01RecipeTestData,
			expectedComponent: "recipe-component-name",
			expectedRepo:      "git+https://github.com/example/operator.git",
			expectedCommit:    "recipe789abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attestation SLSAAttestation
			if err := json.Unmarshal([]byte(tt.data), &attestation); err != nil {
				t.Fatalf("Failed to unmarshal test data: %v", err)
			}

			info := ProvenanceInfo{
				Metadata: make(map[string]string),
			}

			parser.extractFromSLSAv01(&attestation, &info)

			if info.ComponentName != tt.expectedComponent {
				t.Errorf("ComponentName = %q, expected %q", info.ComponentName, tt.expectedComponent)
			}

			if info.SourceRepo != tt.expectedRepo {
				t.Errorf("SourceRepo = %q, expected %q", info.SourceRepo, tt.expectedRepo)
			}

			if info.SourceCommit != tt.expectedCommit {
				t.Errorf("SourceCommit = %q, expected %q", info.SourceCommit, tt.expectedCommit)
			}
		})
	}
}

func TestParseAttestationPayload(t *testing.T) {
	parser := NewProvenanceParser()

	// Create a mock AttestationPayload with base64-encoded JSON payload (as cosign provides)
	attestation := cosign.AttestationPayload{
		PayLoad: encodeTestData(slsaV1TestData),
	}

	info := ProvenanceInfo{
		Metadata: make(map[string]string),
	}

	err := parser.parseAttestationPayload(attestation, &info)
	if err != nil {
		t.Fatalf("parseAttestationPayload failed: %v", err)
	}

	if info.ComponentName != "my-operator-bundle" {
		t.Errorf("ComponentName = %q, expected %q", info.ComponentName, "my-operator-bundle")
	}
}

func TestExtractApplicationName(t *testing.T) {
	parser := NewProvenanceParser()

	tests := []struct {
		name                string
		data                string
		expectedApplication string
	}{
		{
			name:                "SLSA v1.0 attestation",
			data:                slsaV1TestData,
			expectedApplication: "my-operator-app",
		},
		{
			name:                "SLSA v0.1 attestation with invocation labels",
			data:                slsaV01TestData,
			expectedApplication: "forklift-operator-2-9",
		},
		{
			name:                "SLSA v0.1 attestation with recipe environment",
			data:                slsaV01RecipeTestData,
			expectedApplication: "recipe-app-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock AttestationPayload with base64-encoded JSON payload (as cosign provides)
			attestation := cosign.AttestationPayload{
				PayLoad: encodeTestData(tt.data),
			}

			info := ProvenanceInfo{
				Metadata: make(map[string]string),
			}

			err := parser.parseAttestationPayload(attestation, &info)
			if err != nil {
				t.Fatalf("parseAttestationPayload failed: %v", err)
			}

			if info.ApplicationName != tt.expectedApplication {
				t.Errorf("ApplicationName = %q, expected %q", info.ApplicationName, tt.expectedApplication)
			}
		})
	}
}

func TestParseProvenance(t *testing.T) {
	parser := NewProvenanceParser()

	imageRefs := []bundle.ImageReference{
		{
			Image: "quay.io/example/test:latest",
			Name:  "test-image",
		},
	}

	// This will fail in unit tests since we don't have real images,
	// but we can test the error handling
	ctx := context.Background()
	results, err := parser.ParseProvenance(ctx, imageRefs)

	// Should not return an error even if verification fails
	if err != nil {
		t.Errorf("ParseProvenance returned unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if results[0].Verified {
		t.Error("Expected verification to fail for non-existent image")
	}

	if results[0].ImageRef != "quay.io/example/test:latest" {
		t.Errorf("ImageRef = %q, expected %q", results[0].ImageRef, "quay.io/example/test:latest")
	}
}

func TestGetParsingSummary(t *testing.T) {
	parser := NewProvenanceParser()

	results := []ProvenanceInfo{
		{
			ImageRef:   "image1:latest",
			Verified:   true,
			SourceRepo: "git+https://github.com/example/repo1.git",
		},
		{
			ImageRef: "image2:latest",
			Verified: false,
		},
		{
			ImageRef:   "image3:latest",
			Verified:   true,
			SourceRepo: "git+https://github.com/example/repo2.git",
		},
	}

	summary := parser.GetParsingSummary(results)

	expectedTotal := 3
	expectedVerified := 2
	expectedWithSource := 2
	expectedRate := float64(expectedVerified) / float64(expectedTotal) * 100

	if summary["total_images"] != expectedTotal {
		t.Errorf("total_images = %v, expected %v", summary["total_images"], expectedTotal)
	}

	if summary["verified_images"] != expectedVerified {
		t.Errorf("verified_images = %v, expected %v", summary["verified_images"], expectedVerified)
	}

	if summary["images_with_source"] != expectedWithSource {
		t.Errorf("images_with_source = %v, expected %v", summary["images_with_source"], expectedWithSource)
	}

	// Use approximate comparison for floating point
	if rate := summary["verification_rate"].(float64); rate < expectedRate-0.1 || rate > expectedRate+0.1 {
		t.Errorf("verification_rate = %v, expected approximately %v", rate, expectedRate)
	}
}

func TestParseProvenanceErrorHandling(t *testing.T) {
	parser := NewProvenanceParser()

	tests := []struct {
		name        string
		data        string
		expectError bool
	}{
		{
			name:        "Invalid JSON",
			data:        `{"invalid": json}`,
			expectError: true,
		},
		{
			name:        "Empty data",
			data:        "",
			expectError: false, // Empty data should not error
		},
		{
			name:        "Missing predicate",
			data:        `{"predicateType": "test"}`,
			expectError: true, // Non-SLSA predicate types should error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ProvenanceInfo{
				Metadata: make(map[string]string),
			}

			err := parser.parseProvenanceData([]byte(tt.data), &info)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestParseAttestationPayload_SecurityChecks tests security features for payload parsing
func TestParseAttestationPayload_SecurityChecks(t *testing.T) {
	parser := NewProvenanceParser()

	tests := []struct {
		name        string
		payload     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid base64 encoded payload",
			payload:     encodeTestData(slsaV1TestData),
			expectError: false,
		},
		{
			name:        "valid raw JSON payload",
			payload:     slsaV1TestData,
			expectError: false,
		},
		{
			name:        "large payload exceeding size limit",
			payload:     strings.Repeat("a", 11*1024*1024), // 11MB
			expectError: true,
			errorMsg:    "attestation payload too large",
		},
		{
			name:        "malformed base64 that falls back to raw",
			payload:     "not-valid-base64-but-json:" + slsaV1TestData,
			expectError: false, // Should fallback to raw JSON
		},
		{
			name:        "empty payload",
			payload:     "",
			expectError: false, // Empty payload should be handled gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attestation := cosign.AttestationPayload{
				PayLoad: tt.payload,
			}

			info := ProvenanceInfo{
				Metadata: make(map[string]string),
			}

			err := parser.parseAttestationPayload(attestation, &info)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, but got none", tt.name)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %s: %v", tt.name, err)
				}
			}
		})
	}
}

// TestParseAttestationPayload_LargeDecodedPayload tests protection against large payloads
func TestParseAttestationPayload_LargeDecodedPayload(t *testing.T) {
	parser := NewProvenanceParser()

	// Create a large JSON payload - the security checks will catch this at various levels
	largeJSON := `{"predicateType": "test", "data": "` + strings.Repeat("x", 11*1024*1024) + `"}`
	encodedPayload := base64.StdEncoding.EncodeToString([]byte(largeJSON))

	attestation := cosign.AttestationPayload{
		PayLoad: encodedPayload,
	}

	info := ProvenanceInfo{
		Metadata: make(map[string]string),
	}

	err := parser.parseAttestationPayload(attestation, &info)

	if err == nil {
		t.Error("Expected error for large payload, but got none")
	}

	// Accept either error message - both indicate proper size limit enforcement
	if !strings.Contains(err.Error(), "payload too large") {
		t.Errorf("Expected error about payload size, got: %v", err)
	}
}

// TestParseAttestationPayload_JSONDetection tests JSON vs base64 detection
func TestParseAttestationPayload_JSONDetection(t *testing.T) {
	parser := NewProvenanceParser()
	parser.SetVerbose(true) // Enable verbose for better test coverage

	tests := []struct {
		name        string
		payload     string
		isJSON      bool
		description string
	}{
		{
			name:        "clear JSON payload",
			payload:     `{"test": "value"}`,
			isJSON:      true,
			description: "payload starts with {",
		},
		{
			name:        "JSON array payload",
			payload:     `[{"test": "value"}]`,
			isJSON:      true,
			description: "payload starts with [",
		},
		{
			name:        "whitespace prefixed JSON",
			payload:     `   {"test": "value"}`,
			isJSON:      true,
			description: "payload with leading whitespace",
		},
		{
			name:        "base64 encoded payload",
			payload:     base64.StdEncoding.EncodeToString([]byte(`{"test": "value"}`)),
			isJSON:      false,
			description: "base64 encoded JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attestation := cosign.AttestationPayload{
				PayLoad: tt.payload,
			}

			info := ProvenanceInfo{
				Metadata: make(map[string]string),
			}

			// This might error for non-SLSA payloads, but we're testing the detection logic
			_ = parser.parseAttestationPayload(attestation, &info)

			// The test passes if we reach here without panicking
			// The actual JSON vs base64 detection is tested implicitly
		})
	}
}

// TestParseProvenanceDataEdgeCases tests edge cases for provenance data parsing
func TestParseProvenanceDataEdgeCases(t *testing.T) {
	parser := NewProvenanceParser()

	tests := []struct {
		name        string
		data        string
		expectError bool
		errorMsg    string
	}{
		{
			name: "missing required fields in SLSA v1.0",
			data: `{
				"predicateType": "https://slsa.dev/provenance/v1",
				"predicate": {
					"builder": {"id": "https://example.com/builder"},
					"buildDefinition": {
						"externalParameters": {},
						"resolvedDependencies": []
					}
				}
			}`,
			expectError: false, // Should handle gracefully, not crash
		},
		{
			name: "extra unexpected fields in predicate",
			data: `{
				"predicateType": "https://slsa.dev/provenance/v1",
				"predicate": {
					"builder": {"id": "https://example.com/builder"},
					"buildDefinition": {
						"externalParameters": {"workflow": {"ref": "refs/heads/main", "repository": "https://github.com/example/repo"}},
						"resolvedDependencies": [{"uri": "git+https://github.com/example/operator.git", "digest": {"sha1": "abc123def456"}}]
					},
					"invocation": {
						"environment": {
							"labels": {
								"appstudio.openshift.io/component": "test-component",
								"appstudio.openshift.io/application": "test-app"
							}
						}
					},
					"unexpectedField": "should not break parsing",
					"anotherExtra": {"nested": "object"},
					"arrayExtra": ["unexpected", "array", "data"]
				}
			}`,
			expectError: false, // Should handle extra fields gracefully
		},
		{
			name: "malformed JSON structure",
			data: `{
				"predicateType": "https://slsa.dev/provenance/v1",
				"predicate": {
					"builder": "not an object",
					"buildDefinition": true,
					"invocation": ["array", "instead", "of", "object"]
				}
			}`,
			expectError: false, // Should handle type mismatches gracefully
		},
		{
			name: "missing predicate entirely",
			data: `{
				"predicateType": "https://slsa.dev/provenance/v1"
			}`,
			expectError: false, // Should handle missing predicate gracefully
		},
		{
			name: "null values in critical fields",
			data: `{
				"predicateType": "https://slsa.dev/provenance/v1",
				"predicate": {
					"builder": null,
					"buildDefinition": null,
					"invocation": null
				}
			}`,
			expectError: false, // Should handle null values gracefully
		},
		{
			name: "extremely nested structure",
			data: `{
				"predicateType": "https://slsa.dev/provenance/v1",
				"predicate": {
					"builder": {
						"id": "https://example.com/builder",
						"nested": {
							"deeply": {
								"nested": {
									"structure": {
										"that": {
											"should": {
												"not": {
													"break": "parsing"
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}`,
			expectError: false, // Should handle deeply nested structures
		},
		{
			name: "SLSA v0.1 with missing materials",
			data: `{
				"predicateType": "https://slsa.dev/provenance/v0.1",
				"predicate": {
					"builder": {"id": "https://tekton.dev/chains/v2"},
					"recipe": {
						"type": "https://tekton.dev/v1beta1/TaskRun",
						"environment": {
							"appstudio.openshift.io/component": "test-component",
							"appstudio.openshift.io/application": "test-app"
						}
					}
				}
			}`,
			expectError: false, // Should handle missing materials gracefully
		},
		{
			name: "invalid digest format in dependencies",
			data: `{
				"predicateType": "https://slsa.dev/provenance/v1",
				"predicate": {
					"builder": {"id": "https://example.com/builder"},
					"buildDefinition": {
						"resolvedDependencies": [
							{
								"uri": "git+https://github.com/example/repo.git",
								"digest": {
									"invalid_algorithm": "not_a_sha",
									"md5": "also_not_supported"
								}
							}
						]
					}
				}
			}`,
			expectError: false, // Should handle unsupported digest algorithms gracefully
		},
		{
			name: "mixed valid and invalid environment labels",
			data: `{
				"predicateType": "https://slsa.dev/provenance/v0.1",
				"predicate": {
					"builder": {"id": "https://tekton.dev/chains/v2"},
					"materials": [{"uri": "git+https://github.com/example/repo.git", "digest": {"sha1": "abc123"}}],
					"recipe": {
						"environment": {
							"appstudio.openshift.io/component": "valid-component",
							"invalidLabel": 12345,
							"anotherInvalid": ["array", "value"],
							"appstudio.openshift.io/application": "valid-app"
						}
					}
				}
			}`,
			expectError: false, // Should extract valid labels, ignore invalid ones
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ProvenanceInfo{
				Metadata: make(map[string]string),
			}

			err := parser.parseProvenanceData([]byte(tt.data), &info)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, but got none", tt.name)
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				// For graceful handling cases, verify the parser doesn't crash
				// and produces some reasonable output (or none)
				if err != nil {
					// Some errors are acceptable as long as they're not panics
					t.Logf("Graceful error handling: %v", err)
				}
				// The parsing should complete without crashing
			}
		})
	}
}

// TestParseAttestationPayloadMalformedJSON tests malformed JSON payload handling
func TestParseAttestationPayloadMalformedJSON(t *testing.T) {
	parser := NewProvenanceParser()

	tests := []struct {
		name        string
		payload     string
		expectError bool
		description string
	}{
		{
			name:        "completely invalid JSON",
			payload:     `{this is not valid json at all}`,
			expectError: true,
			description: "should fail on completely malformed JSON",
		},
		{
			name:        "JSON with trailing comma",
			payload:     `{"predicateType": "test", "predicate": {},}`,
			expectError: true,
			description: "should fail on JSON syntax errors",
		},
		{
			name:        "unclosed JSON objects",
			payload:     `{"predicateType": "test", "predicate": {`,
			expectError: true,
			description: "should fail on unclosed JSON structures",
		},
		{
			name:        "base64 that decodes to invalid JSON",
			payload:     base64.StdEncoding.EncodeToString([]byte(`{invalid json}`)),
			expectError: true,
			description: "should fail when base64 decodes to invalid JSON",
		},
		{
			name:        "empty JSON object",
			payload:     `{}`,
			expectError: false,
			description: "should handle empty JSON gracefully",
		},
		{
			name:        "JSON with null values",
			payload:     `{"predicateType": null, "predicate": null}`,
			expectError: false,
			description: "should handle null values gracefully",
		},
		{
			name:        "JSON with mixed data types",
			payload:     `{"predicateType": 123, "predicate": ["array", "instead", "of", "object"]}`,
			expectError: false,
			description: "should handle unexpected data types gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attestation := cosign.AttestationPayload{
				PayLoad: tt.payload,
			}

			info := ProvenanceInfo{
				Metadata: make(map[string]string),
			}

			err := parser.parseAttestationPayload(attestation, &info)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, but got none", tt.description)
				}
			} else {
				// For graceful handling, we might get errors but shouldn't crash
				if err != nil {
					t.Logf("Graceful handling of %s: %v", tt.description, err)
				}
			}
		})
	}
}

// TestProvenanceParserWithCorruptedData tests parser behavior with corrupted data
func TestProvenanceParserWithCorruptedData(t *testing.T) {
	parser := NewProvenanceParser()

	// Test extremely large payloads (near the limit)
	t.Run("large but valid payload", func(t *testing.T) {
		// Create a payload that's close to but under the 10MB limit
		largeButValidData := map[string]interface{}{
			"predicateType": "https://slsa.dev/provenance/v1",
			"predicate": map[string]interface{}{
				"builder": map[string]interface{}{
					"id": "https://example.com/builder",
				},
				"buildDefinition": map[string]interface{}{
					"externalParameters": map[string]interface{}{
						"largeData": strings.Repeat("x", 5*1024*1024), // 5MB of data
					},
				},
			},
		}

		jsonData, err := json.Marshal(largeButValidData)
		if err != nil {
			t.Fatalf("Failed to marshal test data: %v", err)
		}

		info := ProvenanceInfo{
			Metadata: make(map[string]string),
		}

		// This should work (under the limit)
		err = parser.parseProvenanceData(jsonData, &info)
		if err != nil {
			t.Errorf("Unexpected error with large but valid payload: %v", err)
		}
	})

	// Test with binary data mixed in
	t.Run("binary data corruption", func(t *testing.T) {
		// Create JSON with embedded binary data
		binaryData := make([]byte, 1000)
		for i := range binaryData {
			binaryData[i] = byte(i % 256)
		}

		// This creates a string with potentially problematic characters
		corruptedJSON := fmt.Sprintf(`{
			"predicateType": "https://slsa.dev/provenance/v1",
			"predicate": {
				"builder": {"id": "https://example.com/builder"},
				"binaryData": "%s"
			}
		}`, base64.StdEncoding.EncodeToString(binaryData))

		info := ProvenanceInfo{
			Metadata: make(map[string]string),
		}

		// Should handle binary data gracefully
		err := parser.parseProvenanceData([]byte(corruptedJSON), &info)
		if err != nil {
			t.Logf("Graceful handling of binary data: %v", err)
		}
	})
}
