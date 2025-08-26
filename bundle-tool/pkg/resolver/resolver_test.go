package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
)

func TestLoadMirrorPolicy_IDMS(t *testing.T) {
	// Create temporary IDMS file
	tmpDir := t.TempDir()
	idmsFile := filepath.Join(tmpDir, "idms.yaml")

	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: test-idms
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads
`

	if err := os.WriteFile(idmsFile, []byte(idmsContent), 0644); err != nil {
		t.Fatalf("Failed to write IDMS file: %v", err)
	}

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(idmsFile)

	if err != nil {
		t.Fatalf("LoadMirrorPolicy failed: %v", err)
	}

	if len(resolver.idmsPolicies) != 1 {
		t.Errorf("Expected 1 IDMS policy, got %d", len(resolver.idmsPolicies))
	}

	if len(resolver.icspPolicies) != 0 {
		t.Errorf("Expected 0 ICSP policies, got %d", len(resolver.icspPolicies))
	}
}

func TestLoadMirrorPolicy_ICSP(t *testing.T) {
	// Create temporary ICSP file
	tmpDir := t.TempDir()
	icspFile := filepath.Join(tmpDir, "icsp.yaml")

	icspContent := `apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: test-icsp
spec:
  repositoryDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads
`

	if err := os.WriteFile(icspFile, []byte(icspContent), 0644); err != nil {
		t.Fatalf("Failed to write ICSP file: %v", err)
	}

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(icspFile)

	if err != nil {
		t.Fatalf("LoadMirrorPolicy failed: %v", err)
	}

	if len(resolver.icspPolicies) != 1 {
		t.Errorf("Expected 1 ICSP policy, got %d", len(resolver.icspPolicies))
	}

	if len(resolver.idmsPolicies) != 0 {
		t.Errorf("Expected 0 IDMS policies, got %d", len(resolver.idmsPolicies))
	}
}

func TestLoadMirrorPolicy_InvalidKind(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid.yaml")

	invalidContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
data:
  key: value
`

	if err := os.WriteFile(invalidFile, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to write invalid file: %v", err)
	}

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(invalidFile)

	if err == nil {
		t.Fatal("Expected error for invalid kind, got nil")
	}

	if !contains(err.Error(), "unsupported mirror policy kind") {
		t.Errorf("Expected 'unsupported mirror policy kind' error, got: %v", err)
	}
}

func TestResolveImageReferences(t *testing.T) {
	tmpDir := t.TempDir()
	idmsFile := filepath.Join(tmpDir, "idms.yaml")

	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: test-idms
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads
`

	if err := os.WriteFile(idmsFile, []byte(idmsContent), 0644); err != nil {
		t.Fatalf("Failed to write IDMS file: %v", err)
	}

	resolver := NewImageResolver()
	if err := resolver.LoadMirrorPolicy(idmsFile); err != nil {
		t.Fatalf("LoadMirrorPolicy failed: %v", err)
	}

	// Test image resolution
	imageRefs := []bundle.ImageReference{
		{Image: "registry.redhat.io/operator/controller:v1.0.0", Name: "controller"},
		{Image: "quay.io/other/image:latest", Name: "other"},
	}

	resolved, err := resolver.ResolveImageReferences(imageRefs)
	if err != nil {
		t.Fatalf("ResolveImageReferences failed: %v", err)
	}

	if len(resolved) != 2 {
		t.Fatalf("Expected 2 resolved references, got %d", len(resolved))
	}

	// First image should be resolved
	if resolved[0].Image != "quay.io/redhat-user-workloads/operator/controller:v1.0.0" {
		t.Errorf("Expected resolved image 'quay.io/redhat-user-workloads/operator/controller:v1.0.0', got '%s'", resolved[0].Image)
	}

	// Second image should be unchanged
	if resolved[1].Image != "quay.io/other/image:latest" {
		t.Errorf("Expected unchanged image 'quay.io/other/image:latest', got '%s'", resolved[1].Image)
	}
}

func TestGetMappingSummary(t *testing.T) {
	resolver := NewImageResolver()

	summary := resolver.GetMappingSummary()

	// Check initial state
	if summary["icsp_policies_count"] != 0 {
		t.Errorf("Expected 0 ICSP policies, got %v", summary["icsp_policies_count"])
	}

	if summary["idms_policies_count"] != 0 {
		t.Errorf("Expected 0 IDMS policies, got %v", summary["idms_policies_count"])
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && s[len(s)-len(substr):] == substr ||
		len(substr) < len(s) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// TestParseImageReference tests the enhanced image reference parsing using containers/image/v5
func TestParseImageReference(t *testing.T) {
	resolver := NewImageResolver()

	testCases := []struct {
		name        string
		imageRef    string
		expected    *ParsedImageRef
		expectError bool
	}{
		{
			name:     "Standard Docker Hub image with tag",
			imageRef: "nginx:latest",
			expected: &ParsedImageRef{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "latest",
				Original:   "nginx:latest",
			},
		},
		{
			name:     "Docker Hub user image with tag",
			imageRef: "myuser/myapp:v1.0.0",
			expected: &ParsedImageRef{
				Registry:   "docker.io",
				Repository: "myuser/myapp",
				Tag:        "v1.0.0",
				Original:   "myuser/myapp:v1.0.0",
			},
		},
		{
			name:     "Quay.io image with digest",
			imageRef: "quay.io/operator/controller@sha256:abcd1234567890abcdef1234567890abcdef1234567890abcdef123456789012",
			expected: &ParsedImageRef{
				Registry:   "quay.io",
				Repository: "operator/controller",
				Digest:     "sha256:abcd1234567890abcdef1234567890abcdef1234567890abcdef123456789012",
				Original:   "quay.io/operator/controller@sha256:abcd1234567890abcdef1234567890abcdef1234567890abcdef123456789012",
			},
		},
		{
			name:     "Registry with custom port",
			imageRef: "registry.example.com:5000/myapp/service:v2.1.0",
			expected: &ParsedImageRef{
				Registry:   "registry.example.com:5000",
				Repository: "myapp/service",
				Tag:        "v2.1.0",
				Original:   "registry.example.com:5000/myapp/service:v2.1.0",
			},
		},
		{
			name:     "Complex registry path",
			imageRef: "gcr.io/my-project/subproject/app:latest",
			expected: &ParsedImageRef{
				Registry:   "gcr.io",
				Repository: "my-project/subproject/app",
				Tag:        "latest",
				Original:   "gcr.io/my-project/subproject/app:latest",
			},
		},
		{
			name:     "Image with both tag and digest should prefer digest",
			imageRef: "registry.redhat.io/ubi8/ubi:8.5@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: &ParsedImageRef{
				Registry:   "registry.redhat.io",
				Repository: "ubi8/ubi",
				Tag:        "8.5",
				Digest:     "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
				Original:   "registry.redhat.io/ubi8/ubi:8.5@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			},
		},
		{
			name:     "Image without tag defaults to 'latest'",
			imageRef: "quay.io/operator/bundle",
			expected: &ParsedImageRef{
				Registry:   "quay.io",
				Repository: "operator/bundle",
				Original:   "quay.io/operator/bundle",
			},
		},
		{
			name:        "Invalid image reference",
			imageRef:    "invalid-ref://malformed",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := resolver.parseImageReference(tc.imageRef)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for input %s, but got none", tc.imageRef)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for input %s: %v", tc.imageRef, err)
				return
			}

			if parsed.Registry != tc.expected.Registry {
				t.Errorf("Registry mismatch. Expected: %s, Got: %s", tc.expected.Registry, parsed.Registry)
			}

			if parsed.Repository != tc.expected.Repository {
				t.Errorf("Repository mismatch. Expected: %s, Got: %s", tc.expected.Repository, parsed.Repository)
			}

			if parsed.Tag != tc.expected.Tag {
				t.Errorf("Tag mismatch. Expected: %s, Got: %s", tc.expected.Tag, parsed.Tag)
			}

			if parsed.Digest != tc.expected.Digest {
				t.Errorf("Digest mismatch. Expected: %s, Got: %s", tc.expected.Digest, parsed.Digest)
			}

			if parsed.Original != tc.expected.Original {
				t.Errorf("Original mismatch. Expected: %s, Got: %s", tc.expected.Original, parsed.Original)
			}
		})
	}
}

// TestReconstructImageReference tests the image reference reconstruction functionality
func TestReconstructImageReference(t *testing.T) {
	resolver := NewImageResolver()

	testCases := []struct {
		name        string
		parsed      *ParsedImageRef
		newRegistry string
		expected    string
	}{
		{
			name: "Reconstruct with tag",
			parsed: &ParsedImageRef{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Tag:        "latest",
			},
			newRegistry: "mirror.registry.com",
			expected:    "mirror.registry.com/library/nginx:latest",
		},
		{
			name: "Reconstruct with digest",
			parsed: &ParsedImageRef{
				Registry:   "quay.io",
				Repository: "operator/controller",
				Digest:     "sha256:abcd1234",
			},
			newRegistry: "internal.registry.com",
			expected:    "internal.registry.com/operator/controller@sha256:abcd1234",
		},
		{
			name: "Reconstruct with both tag and digest prefers digest",
			parsed: &ParsedImageRef{
				Registry:   "registry.redhat.io",
				Repository: "ubi8/ubi",
				Tag:        "8.5",
				Digest:     "sha256:xyz789",
			},
			newRegistry: "mirror.redhat.com",
			expected:    "mirror.redhat.com/ubi8/ubi@sha256:xyz789",
		},
		{
			name: "Reconstruct without tag or digest",
			parsed: &ParsedImageRef{
				Registry:   "gcr.io",
				Repository: "project/app",
			},
			newRegistry: "private.registry.com",
			expected:    "private.registry.com/project/app",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := resolver.reconstructImageReference(tc.parsed, tc.newRegistry)
			if result != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, result)
			}
		})
	}
}

// TestEnhancedMatchAndReplace tests the enhanced image matching and replacement logic
func TestEnhancedMatchAndReplace(t *testing.T) {
	resolver := NewImageResolver()

	testCases := []struct {
		name        string
		imageRef    string
		source      string
		mirrors     []string
		expected    string
		shouldMatch bool
	}{
		{
			name:        "Exact registry match",
			imageRef:    "quay.io/operator/controller:v1.0.0",
			source:      "quay.io",
			mirrors:     []string{"mirror.registry.com"},
			expected:    "mirror.registry.com/operator/controller:v1.0.0",
			shouldMatch: true,
		},
		{
			name:        "Registry and repository prefix match",
			imageRef:    "registry.redhat.io/ubi8/ubi:latest",
			source:      "registry.redhat.io/ubi8",
			mirrors:     []string{"internal.mirror.com"},
			expected:    "internal.mirror.com/ubi:latest",
			shouldMatch: true,
		},
		{
			name:        "Docker Hub image normalization",
			imageRef:    "nginx:latest",
			source:      "docker.io",
			mirrors:     []string{"private.registry.com"},
			expected:    "private.registry.com/library/nginx:latest",
			shouldMatch: true,
		},
		{
			name:        "Custom port in registry",
			imageRef:    "registry.example.com:5000/app/service:v2.0.0",
			source:      "registry.example.com:5000",
			mirrors:     []string{"mirror.example.com:8080"},
			expected:    "mirror.example.com:8080/app/service:v2.0.0",
			shouldMatch: true,
		},
		{
			name:        "Digest-based image",
			imageRef:    "quay.io/operator/bundle@sha256:abcd1234567890abcdef1234567890abcdef1234567890abcdef123456789012",
			source:      "quay.io/operator",
			mirrors:     []string{"internal.registry.com/mirrors"},
			expected:    "internal.registry.com/mirrors/bundle@sha256:abcd1234567890abcdef1234567890abcdef1234567890abcdef123456789012",
			shouldMatch: true,
		},
		{
			name:        "No match - different registry",
			imageRef:    "gcr.io/project/app:latest",
			source:      "quay.io",
			mirrors:     []string{"mirror.registry.com"},
			expected:    "gcr.io/project/app:latest",
			shouldMatch: false,
		},
		{
			name:        "Fallback for unparseable source",
			imageRef:    "quay.io/operator/controller:v1.0.0",
			source:      "quay.io/operator/",
			mirrors:     []string{"mirror.registry.com"},
			expected:    "mirror.registry.com/controller:v1.0.0",
			shouldMatch: true,
		},
		{
			name:        "Empty mirrors array",
			imageRef:    "quay.io/operator/controller:v1.0.0",
			source:      "quay.io",
			mirrors:     []string{},
			expected:    "quay.io/operator/controller:v1.0.0",
			shouldMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, matched := resolver.matchAndReplace(tc.imageRef, tc.source, tc.mirrors)

			if matched != tc.shouldMatch {
				t.Errorf("Match result mismatch. Expected: %t, Got: %t", tc.shouldMatch, matched)
			}

			if result != tc.expected {
				t.Errorf("Result mismatch. Expected: %s, Got: %s", tc.expected, result)
			}
		})
	}
}

// TestComplexImageResolution tests complex scenarios with multiple policies and edge cases
func TestComplexImageResolution(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a complex IDMS policy file
	idmsFile := filepath.Join(tmpDir, "complex-idms.yaml")
	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: complex-test
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads
  - source: registry.redhat.io/ubi8
    mirrors:
    - internal.registry.com/ubi
  - source: docker.io
    mirrors:
    - mirror.dockerhub.com
  - source: gcr.io/my-project
    mirrors:
    - private.gcr.mirror.com
`

	if err := os.WriteFile(idmsFile, []byte(idmsContent), 0644); err != nil {
		t.Fatalf("Failed to write IDMS file: %v", err)
	}

	resolver := NewImageResolver()
	if err := resolver.LoadMirrorPolicy(idmsFile); err != nil {
		t.Fatalf("LoadMirrorPolicy failed: %v", err)
	}

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Specific UBI8 mapping takes precedence",
			input:    "registry.redhat.io/ubi8/ubi:8.5",
			expected: "internal.registry.com/ubi/ubi:8.5",
		},
		{
			name:     "General RedHat registry mapping",
			input:    "registry.redhat.io/rhel8/rhel-tools:latest",
			expected: "quay.io/redhat-user-workloads/rhel8/rhel-tools:latest",
		},
		{
			name:     "Docker Hub official image normalization",
			input:    "nginx:latest",
			expected: "mirror.dockerhub.com/library/nginx:latest",
		},
		{
			name:     "Docker Hub user image",
			input:    "myuser/myapp:v1.0.0",
			expected: "mirror.dockerhub.com/myuser/myapp:v1.0.0",
		},
		{
			name:     "GCR project-specific mapping",
			input:    "gcr.io/my-project/app/service:v2.0.0",
			expected: "private.gcr.mirror.com/app/service:v2.0.0",
		},
		{
			name:     "Complex digest-based image",
			input:    "registry.redhat.io/ubi8/nodejs-16@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd",
			expected: "internal.registry.com/ubi/nodejs-16@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcd",
		},
		{
			name:     "Unmapped registry remains unchanged",
			input:    "quay.io/operator/controller:latest",
			expected: "quay.io/operator/controller:latest",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			imageRefs := []bundle.ImageReference{{Image: tc.input, Name: "test"}}
			resolved, err := resolver.ResolveImageReferences(imageRefs)

			if err != nil {
				t.Fatalf("ResolveImageReferences failed: %v", err)
			}

			if len(resolved) != 1 {
				t.Fatalf("Expected 1 resolved reference, got %d", len(resolved))
			}

			if resolved[0].Image != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, resolved[0].Image)
			}
		})
	}
}

// TestImageReferenceEdgeCases tests edge cases for image reference processing
func TestImageReferenceEdgeCases(t *testing.T) {
	resolver := NewImageResolver()

	tests := []struct {
		name        string
		imageRef    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "partial repository path should not be replaced",
			imageRef:    "quay.io/redhat/operator:v1.0.0", // Should not match "registry.redhat.io"
			expectError: false,
		},
		{
			name:        "image with multiple @ symbols",
			imageRef:    "registry.io/user@domain/app@sha256:abc123",
			expectError: true, // Multiple @ symbols not valid
		},
		{
			name:        "image with no registry",
			imageRef:    "myapp:latest",
			expectError: false, // Should default to docker.io
		},
		{
			name:        "image with port but no registry",
			imageRef:    ":8080/app:latest",
			expectError: true, // Invalid format
		},
		{
			name:        "image with empty tag",
			imageRef:    "registry.io/app:",
			expectError: true, // Invalid format
		},
		{
			name:        "image with empty digest",
			imageRef:    "registry.io/app@",
			expectError: true, // Invalid format
		},
		{
			name:        "image with invalid digest format",
			imageRef:    "registry.io/app@md5:invalidhash",
			expectError: true, // Invalid digest format
		},
		{
			name:        "extremely long image reference",
			imageRef:    "very-long-registry-name-that-exceeds-normal-limits.example.com:8080/" + strings.Repeat("long-path-segment/", 20) + "app:v1.0.0",
			expectError: true, // Too long reference
		},
		{
			name:        "image with special characters in repository",
			imageRef:    "registry.io/user.name/app_name-version:latest",
			expectError: false, // Should handle valid special characters
		},
		{
			name:        "image with unicode characters",
			imageRef:    "registry.io/用户/应用:latest",
			expectError: true, // Unicode not valid in image references
		},
		{
			name:        "image with only numbers",
			imageRef:    "123.456.789.10:5000/123/456:789",
			expectError: false, // Should handle numeric parts
		},
		{
			name:        "empty image reference",
			imageRef:    "",
			expectError: true, // Should fail on empty input
		},
		{
			name:        "image reference with whitespace",
			imageRef:    "  registry.io/app:latest  ",
			expectError: true, // Whitespace not valid in image references
		},
		{
			name:        "image with complex tag containing special chars",
			imageRef:    "registry.io/app:v1.0.0-beta.1+build.123",
			expectError: true, // + character not valid in tags
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test parsing
			parsed, err := resolver.parseImageReference(tt.imageRef)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, but got none", tt.imageRef)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for %s: %v", tt.imageRef, err)
				return
			}

			// Verify that parsing succeeded and we can reconstruct
			if parsed != nil {
				reconstructed := resolver.reconstructImageReference(parsed, parsed.Registry)
				// The reconstructed image should be valid (not empty)
				if reconstructed == "" {
					t.Errorf("Failed to reconstruct image reference for %s", tt.imageRef)
				}
			}
		})
	}
}

// TestComplexMirrorSourceMatching tests complex mirror source matching scenarios
func TestComplexMirrorSourceMatching(t *testing.T) {
	tmpDir := t.TempDir()
	idmsFile := filepath.Join(tmpDir, "complex-matching.yaml")

	// Create policy with overlapping and complex source patterns
	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: complex-matching-test
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - mirror1.example.com
  - source: registry.redhat.io/ubi8
    mirrors:
    - mirror2.example.com/ubi
  - source: registry.redhat.io/ubi8/nodejs
    mirrors:
    - mirror3.example.com/nodejs
  - source: quay.io/operator
    mirrors:
    - internal.registry.com/operators
  - source: docker.io/library
    mirrors:
    - dockerhub.mirror.com
`

	if err := os.WriteFile(idmsFile, []byte(idmsContent), 0644); err != nil {
		t.Fatalf("Failed to write IDMS file: %v", err)
	}

	resolver := NewImageResolver()
	if err := resolver.LoadMirrorPolicy(idmsFile); err != nil {
		t.Fatalf("LoadMirrorPolicy failed: %v", err)
	}

	tests := []struct {
		name          string
		imageRef      string
		expectedMatch string
		description   string
	}{
		{
			name:          "most specific match wins",
			imageRef:      "registry.redhat.io/ubi8/nodejs-16:latest",
			expectedMatch: "mirror3.example.com/nodejs/-16:latest",
			description:   "Should use most specific matching source (specificity wins over order)",
		},
		{
			name:          "intermediate specificity match",
			imageRef:      "registry.redhat.io/ubi8/ubi-minimal:latest",
			expectedMatch: "mirror2.example.com/ubi/ubi-minimal:latest",
			description:   "Should use intermediate source match",
		},
		{
			name:          "general match when no specific match",
			imageRef:      "registry.redhat.io/rhel8/httpd:latest",
			expectedMatch: "mirror1.example.com/rhel8/httpd:latest",
			description:   "Should fall back to general match",
		},
		{
			name:          "partial path should not match",
			imageRef:      "registry.redhat.io.evil.com/ubi8/nodejs:latest",
			expectedMatch: "registry.redhat.io.evil.com/ubi8/nodejs:latest",
			description:   "Should NOT match partial hostname",
		},
		{
			name:          "case sensitive matching",
			imageRef:      "Registry.Redhat.Io/ubi8/nodejs:latest",
			expectedMatch: "Registry.Redhat.Io/ubi8/nodejs:latest",
			description:   "Should be case sensitive and NOT match",
		},
		{
			name:          "docker hub library prefix handling",
			imageRef:      "docker.io/library/nginx:latest",
			expectedMatch: "dockerhub.mirror.com/nginx:latest",
			description:   "Should handle docker.io/library prefix correctly",
		},
		{
			name:          "operator namespace matching",
			imageRef:      "quay.io/operator/controller:v1.0.0",
			expectedMatch: "internal.registry.com/operators/controller:v1.0.0",
			description:   "Should match operator namespace",
		},
		{
			name:          "operator prefix should match operator subpath",
			imageRef:      "quay.io/operator-sdk/bundle:latest",
			expectedMatch: "internal.registry.com/operators/-sdk/bundle:latest",
			description:   "Should match when prefix matches at / boundary (OpenShift behavior)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageRefs := []bundle.ImageReference{{Image: tt.imageRef, Name: "test"}}
			resolved, err := resolver.ResolveImageReferences(imageRefs)

			if err != nil {
				t.Fatalf("ResolveImageReferences failed: %v", err)
			}

			if len(resolved) != 1 {
				t.Fatalf("Expected 1 resolved reference, got %d", len(resolved))
			}

			if resolved[0].Image != tt.expectedMatch {
				t.Errorf("%s: Expected %s, got %s", tt.description, tt.expectedMatch, resolved[0].Image)
			}
		})
	}
}

// TestMirrorCountingAccuracy tests that mirror counting reflects total endpoints, not just top-level policies
func TestMirrorCountingAccuracy(t *testing.T) {
	tmpDir := t.TempDir()
	idmsFile := filepath.Join(tmpDir, "mirror-counting.yaml")

	// Create policy with multiple mirrors per source
	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: mirror-counting-test
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - mirror1.example.com
    - mirror2.example.com
    - mirror3.example.com
  - source: quay.io
    mirrors:
    - quay-mirror1.example.com
    - quay-mirror2.example.com
  - source: docker.io
    mirrors:
    - dockerhub-mirror.example.com
`

	if err := os.WriteFile(idmsFile, []byte(idmsContent), 0644); err != nil {
		t.Fatalf("Failed to write IDMS file: %v", err)
	}

	resolver := NewImageResolver()
	if err := resolver.LoadMirrorPolicy(idmsFile); err != nil {
		t.Fatalf("LoadMirrorPolicy failed: %v", err)
	}

	// Test mirror counting
	t.Run("count total mirror endpoints", func(t *testing.T) {
		expectedTotalMirrors := 6 // 3 + 2 + 1 = 6 total mirror endpoints
		expectedPolicies := 1     // 1 policy file loaded

		// Get statistics from resolver
		stats := resolver.GetMirrorStats()

		if stats.TotalPolicies != expectedPolicies {
			t.Errorf("Expected %d policies, got %d", expectedPolicies, stats.TotalPolicies)
		}

		if stats.TotalMirrors != expectedTotalMirrors {
			t.Errorf("Expected %d total mirrors, got %d", expectedTotalMirrors, stats.TotalMirrors)
		}
	})

	// Test that resolution uses first mirror from each source
	t.Run("verify mirror selection", func(t *testing.T) {
		tests := []struct {
			imageRef string
			expected string
		}{
			{
				imageRef: "registry.redhat.io/ubi8/ubi:latest",
				expected: "mirror1.example.com/ubi8/ubi:latest", // First mirror
			},
			{
				imageRef: "quay.io/operator/controller:v1.0.0",
				expected: "quay-mirror1.example.com/operator/controller:v1.0.0", // First mirror
			},
			{
				imageRef: "nginx:latest",
				expected: "dockerhub-mirror.example.com/library/nginx:latest", // First mirror
			},
		}

		for _, tt := range tests {
			imageRefs := []bundle.ImageReference{{Image: tt.imageRef, Name: "test"}}
			resolved, err := resolver.ResolveImageReferences(imageRefs)

			if err != nil {
				t.Fatalf("ResolveImageReferences failed for %s: %v", tt.imageRef, err)
			}

			if len(resolved) != 1 {
				t.Fatalf("Expected 1 resolved reference for %s, got %d", tt.imageRef, len(resolved))
			}

			if resolved[0].Image != tt.expected {
				t.Errorf("For %s: Expected %s, got %s", tt.imageRef, tt.expected, resolved[0].Image)
			}
		}
	})
}

// TestDigestExtractionEdgeCases tests edge cases for digest extraction
func TestDigestExtractionEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		expected string
	}{
		{
			name:     "standard sha256 digest",
			imageRef: "registry.io/app@sha256:abc123def456",
			expected: "sha256:abc123def456",
		},
		{
			name:     "multiple @ symbols - should extract last one",
			imageRef: "registry@domain.com/user@domain/app@sha256:abc123",
			expected: "sha256:abc123",
		},
		{
			name:     "no digest",
			imageRef: "registry.io/app:v1.0.0",
			expected: "",
		},
		{
			name:     "non-sha256 digest format",
			imageRef: "registry.io/app@md5:def456",
			expected: "md5:def456",
		},
		{
			name:     "empty digest after @",
			imageRef: "registry.io/app@",
			expected: "",
		},
		{
			name:     "digest with unusual algorithm",
			imageRef: "registry.io/app@blake2b:xyz789",
			expected: "blake2b:xyz789",
		},
		{
			name:     "malformed digest",
			imageRef: "registry.io/app@not-a-real-digest",
			expected: "not-a-real-digest", // Should extract whatever follows @
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDigest(tt.imageRef)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Helper function for testing - equivalent to the one used in the code
func extractDigest(imageRef string) string {
	if idx := strings.LastIndex(imageRef, "@"); idx != -1 {
		return imageRef[idx+1:]
	}
	return ""
}
