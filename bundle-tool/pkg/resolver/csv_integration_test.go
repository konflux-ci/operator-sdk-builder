package resolver

import (
	"os"
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
)

// Helper function to create temporary mirror policy files for testing
func createTempMirrorPolicy(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "mirror-policy-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	return tmpFile.Name()
}

// Helper function to remove temporary files
func removeTempFile(t *testing.T, filename string) {
	t.Helper()
	if err := os.Remove(filename); err != nil {
		t.Logf("failed to remove temp file %s: %v", filename, err)
	}
}

func TestImageResolverWithCSVImages(t *testing.T) {
	tests := []struct {
		name           string
		imageRefs      []bundle.ImageReference
		icspContent    string
		expectedImages []string
	}{
		{
			name: "resolve operator images with IDMS",
			imageRefs: []bundle.ImageReference{
				{Image: "quay.io/test/operator:v1.0.0", Name: "operator"},
				{Image: "registry.redhat.io/ubi9/ubi-minimal:latest", Name: "ubi"},
				{Image: "quay.io/test/utility:latest", Name: "utility"},
			},
			icspContent: `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: test-mapping
spec:
  imageDigestMirrors:
  - source: quay.io/test
    mirrors:
    - quay.io/redhat-user-workloads/test
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads/registry
`,
			expectedImages: []string{
				"quay.io/redhat-user-workloads/test/operator:v1.0.0",
				"quay.io/redhat-user-workloads/registry/ubi9/ubi-minimal:latest",
				"quay.io/redhat-user-workloads/test/utility:latest",
			},
		},
		{
			name: "resolve with partial matches",
			imageRefs: []bundle.ImageReference{
				{Image: "quay.io/test/operator:v1.0.0", Name: "operator"},
				{Image: "docker.io/library/nginx:latest", Name: "nginx"},
			},
			icspContent: `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: partial-mapping
spec:
  imageDigestMirrors:
  - source: quay.io/test
    mirrors:
    - quay.io/redhat-user-workloads/test
`,
			expectedImages: []string{
				"quay.io/redhat-user-workloads/test/operator:v1.0.0",
				"docker.io/library/nginx:latest", // unchanged
			},
		},
		{
			name: "no mirror policy matches",
			imageRefs: []bundle.ImageReference{
				{Image: "docker.io/library/redis:latest", Name: "redis"},
				{Image: "gcr.io/example/app:v1.0.0", Name: "app"},
			},
			icspContent: `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: no-matches
spec:
  imageDigestMirrors:
  - source: quay.io/different
    mirrors:
    - quay.io/redhat-user-workloads/different
`,
			expectedImages: []string{
				"docker.io/library/redis:latest",
				"gcr.io/example/app:v1.0.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file for mirror policy
			tmpFile := createTempMirrorPolicy(t, tt.icspContent)
			defer removeTempFile(t, tmpFile)

			// Create resolver and load mirror policy
			resolver := NewImageResolver()
			err := resolver.LoadMirrorPolicy(tmpFile)
			if err != nil {
				t.Fatalf("failed to load mirror policy: %v", err)
			}

			// Resolve image references
			resolvedRefs, err := resolver.ResolveImageReferences(tt.imageRefs)
			if err != nil {
				t.Fatalf("failed to resolve image references: %v", err)
			}

			// Verify results
			if len(resolvedRefs) != len(tt.expectedImages) {
				t.Errorf("expected %d resolved images, got %d", len(tt.expectedImages), len(resolvedRefs))
			}

			for i, expected := range tt.expectedImages {
				if i >= len(resolvedRefs) {
					t.Errorf("missing resolved image at index %d", i)
					continue
				}

				if resolvedRefs[i].Image != expected {
					t.Errorf("resolved image %d: expected '%s', got '%s'", i, expected, resolvedRefs[i].Image)
				}

				// Verify name is preserved
				if resolvedRefs[i].Name != tt.imageRefs[i].Name {
					t.Errorf("resolved image %d name: expected '%s', got '%s'", i, tt.imageRefs[i].Name, resolvedRefs[i].Name)
				}
			}

			// Check mapping summary
			summary := resolver.GetMappingSummary()
			if summary["idms_policies_count"] != 1 {
				t.Errorf("expected 1 IDMS policy, got %v", summary["idms_policies_count"])
			}
		})
	}
}

func TestImageResolverCSVWorkflow(t *testing.T) {
	// Test the complete workflow that would happen with a CSV

	// Simulate images extracted from a CSV
	csvImages := []bundle.ImageReference{
		{Image: "quay.io/operator/controller:v1.0.0", Name: "controller"},
		{Image: "quay.io/operator/webhook:v1.0.0", Name: "webhook"},
		{Image: "registry.redhat.io/ubi9/ubi-minimal:9.1", Name: "ubi"},
		{Image: "docker.io/library/busybox:latest", Name: "busybox"},
	}

	mirrorPolicyContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: konflux-dev-mapping
spec:
  imageDigestMirrors:
  - source: quay.io/operator
    mirrors:
    - quay.io/redhat-user-workloads/operator-ns/operator
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads/registry-mirror
`

	tmpFile := createTempMirrorPolicy(t, mirrorPolicyContent)
	defer removeTempFile(t, tmpFile)

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(tmpFile)
	if err != nil {
		t.Fatalf("failed to load mirror policy: %v", err)
	}

	resolvedImages, err := resolver.ResolveImageReferences(csvImages)
	if err != nil {
		t.Fatalf("failed to resolve images: %v", err)
	}

	expectedMappings := map[string]string{
		"quay.io/operator/controller:v1.0.0":      "quay.io/redhat-user-workloads/operator-ns/operator/controller:v1.0.0",
		"quay.io/operator/webhook:v1.0.0":         "quay.io/redhat-user-workloads/operator-ns/operator/webhook:v1.0.0",
		"registry.redhat.io/ubi9/ubi-minimal:9.1": "quay.io/redhat-user-workloads/registry-mirror/ubi9/ubi-minimal:9.1",
		"docker.io/library/busybox:latest":        "docker.io/library/busybox:latest", // no mapping
	}

	for i, resolved := range resolvedImages {
		original := csvImages[i].Image
		expected := expectedMappings[original]

		if resolved.Image != expected {
			t.Errorf("image mapping for '%s': expected '%s', got '%s'", original, expected, resolved.Image)
		}
	}

	// Verify summary
	summary := resolver.GetMappingSummary()
	if summary["idms_policies_count"] != 1 {
		t.Errorf("expected 1 IDMS policy in summary")
	}
	if summary["total_idms_mirrors"] != 2 {
		t.Errorf("expected 2 IDMS mirrors in summary")
	}
}

func TestImageResolverEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		expected string
	}{
		{
			name:     "image with digest",
			imageRef: "quay.io/test/app@sha256:abcd1234",
			expected: "quay.io/redhat-user-workloads/test/app@sha256:abcd1234",
		},
		{
			name:     "image without tag",
			imageRef: "quay.io/test/app",
			expected: "quay.io/redhat-user-workloads/test/app",
		},
		{
			name:     "image with port in registry",
			imageRef: "localhost:5000/test/app:latest",
			expected: "localhost:5000/test/app:latest", // no mapping
		},
	}

	mirrorPolicyContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: edge-case-mapping
spec:
  imageDigestMirrors:
  - source: quay.io/test
    mirrors:
    - quay.io/redhat-user-workloads/test
`

	tmpFile := createTempMirrorPolicy(t, mirrorPolicyContent)
	defer removeTempFile(t, tmpFile)

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(tmpFile)
	if err != nil {
		t.Fatalf("failed to load mirror policy: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageRefs := []bundle.ImageReference{{Image: tt.imageRef, Name: "test"}}
			resolved, err := resolver.ResolveImageReferences(imageRefs)
			if err != nil {
				t.Fatalf("failed to resolve image: %v", err)
			}

			if len(resolved) != 1 {
				t.Fatalf("expected 1 resolved image, got %d", len(resolved))
			}

			if resolved[0].Image != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, resolved[0].Image)
			}
		})
	}
}
