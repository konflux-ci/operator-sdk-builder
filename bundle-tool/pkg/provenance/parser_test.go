package provenance

import (
	"context"
	"encoding/base64"
	"encoding/json"
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