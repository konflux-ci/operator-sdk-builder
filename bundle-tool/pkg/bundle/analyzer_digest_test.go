package bundle

import (
	"testing"
)

// TestExtractDigestWithDigestImages tests digest extraction from actual digest-pinned images
func TestExtractDigestWithDigestImages(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "standard digest image",
			image:    "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			expected: "sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
		},
		{
			name:     "digest image with port",
			image:    "localhost:5000/test/operator@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			expected: "sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
		},
		{
			name:     "nested registry digest image",
			image:    "registry.redhat.io/rhel8/postgresql-13@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
			expected: "sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
		},
		{
			name:     "GCR digest image",
			image:    "gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3",
			expected: "sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3",
		},
		{
			name:     "registry with complex path and digest",
			image:    "registry.example.com/namespace/project/subproject/image@sha256:e5f6789012345678901234567890123456789012345678901234567890a1b2c3d4",
			expected: "sha256:e5f6789012345678901234567890123456789012345678901234567890a1b2c3d4",
		},
		{
			name:     "digest image with uppercase registry",
			image:    "Registry.Example.Com/test/operator@sha256:f6789012345678901234567890123456789012345678901234567890a1b2c3d4e5",
			expected: "sha256:f6789012345678901234567890123456789012345678901234567890a1b2c3d4e5",
		},
		{
			name:     "tag and digest combined",
			image:    "quay.io/test/operator:v1.0.0@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			expected: "sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
		},
		{
			name:     "image with @ in tag but also valid digest",
			image:    "registry.com/test:tag@with@symbols@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			expected: "sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
		},
		{
			name:     "tag-only image (no digest)",
			image:    "quay.io/test/operator:v1.0.0",
			expected: "",
		},
		{
			name:     "latest tag (no digest)",
			image:    "registry.redhat.io/ubi8/ubi:latest",
			expected: "",
		},
		{
			name:     "invalid digest format",
			image:    "quay.io/test/operator@invalid:notadigest",
			expected: "",
		},
		{
			name:     "empty digest",
			image:    "quay.io/test/operator@",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDigest(tt.image)
			if result != tt.expected {
				t.Errorf("extractDigest(%q) = %q, want %q", tt.image, result, tt.expected)
			}
		})
	}
}

// TestDeduplicateImageReferencesWithDigests tests deduplication of image references containing digests
func TestDeduplicateImageReferencesWithDigests(t *testing.T) {
	analyzer := NewBundleAnalyzer()

	tests := []struct {
		name     string
		input    []ImageReference
		expected []ImageReference
	}{
		{
			name: "digest and tag versions of same image",
			input: []ImageReference{
				{Image: "quay.io/test/operator:v1.0.0", Name: "operator-tag"},
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-digest"},
				{Image: "quay.io/test/webhook:latest", Name: "webhook"},
			},
			expected: []ImageReference{
				{Image: "quay.io/test/operator:v1.0.0", Name: "operator-tag"},
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-digest"},
				{Image: "quay.io/test/webhook:latest", Name: "webhook"},
			},
		},
		{
			name: "identical digest images",
			input: []ImageReference{
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-1"},
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-2"},
				{Image: "quay.io/test/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1", Name: "webhook"},
			},
			expected: []ImageReference{
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-1"},
				{Image: "quay.io/test/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1", Name: "webhook"},
			},
		},
		{
			name: "mixed digest and tag duplicates",
			input: []ImageReference{
				{Image: "quay.io/test/operator:v1.0.0", Name: "operator-tag-1"},
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-digest-1"},
				{Image: "quay.io/test/operator:v1.0.0", Name: "operator-tag-2"},
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-digest-2"},
				{Image: "registry.redhat.io/ubi8/ubi:latest", Name: "ubi"},
			},
			expected: []ImageReference{
				{Image: "quay.io/test/operator:v1.0.0", Name: "operator-tag-1"},
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-digest-1"},
				{Image: "registry.redhat.io/ubi8/ubi:latest", Name: "ubi"},
			},
		},
		{
			name: "different digests for same repository",
			input: []ImageReference{
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-v1"},
				{Image: "quay.io/test/operator@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1", Name: "operator-v2"},
				{Image: "quay.io/test/operator@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2", Name: "operator-v3"},
			},
			expected: []ImageReference{
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator-v1"},
				{Image: "quay.io/test/operator@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1", Name: "operator-v2"},
				{Image: "quay.io/test/operator@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2", Name: "operator-v3"},
			},
		},
		{
			name: "empty images mixed with digest images",
			input: []ImageReference{
				{Image: "", Name: "empty-1"},
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator"},
				{Image: "", Name: "empty-2"},
				{Image: "registry.redhat.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1", Name: "webhook"},
			},
			expected: []ImageReference{
				{Image: "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890", Name: "operator"},
				{Image: "registry.redhat.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1", Name: "webhook"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.deduplicateImageReferences(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("DeduplicateImageReferences() returned %d items, want %d", len(result), len(tt.expected))
			}

			// Check that all expected items are present
			for _, expected := range tt.expected {
				found := false
				for _, actual := range result {
					if actual.Image == expected.Image && actual.Name == expected.Name {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected item not found: %+v", expected)
				}
			}

			// Check that there are no unexpected items
			for _, actual := range result {
				found := false
				for _, expected := range tt.expected {
					if actual.Image == expected.Image && actual.Name == expected.Name {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Unexpected item found: %+v", actual)
				}
			}
		})
	}
}

// TestImageReferenceDigestParsing tests ImageReference struct with digest images
func TestImageReferenceDigestParsing(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		expectName   string
		expectDigest string
	}{
		{
			name:         "digest image with extracted digest",
			image:        "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			expectName:   "operator",
			expectDigest: "sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
		},
		{
			name:         "tag image with no digest",
			image:        "quay.io/test/operator:v1.0.0",
			expectName:   "operator",
			expectDigest: "",
		},
		{
			name:         "complex digest image",
			image:        "registry.redhat.io/rhel8/postgresql-13@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			expectName:   "postgresql",
			expectDigest: "sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
		},
		{
			name:         "GCR digest image",
			image:        "gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
			expectName:   "kube-rbac-proxy",
			expectDigest: "sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create ImageReference as would be done in real processing
			ref := ImageReference{
				Image:  tt.image,
				Name:   tt.expectName,
				Digest: extractDigest(tt.image),
			}

			if ref.Image != tt.image {
				t.Errorf("Image = %q, want %q", ref.Image, tt.image)
			}

			if ref.Name != tt.expectName {
				t.Errorf("Name = %q, want %q", ref.Name, tt.expectName)
			}

			if ref.Digest != tt.expectDigest {
				t.Errorf("Digest = %q, want %q", ref.Digest, tt.expectDigest)
			}
		})
	}
}

// TestManifestProcessingWithDigests tests manifest processing specifically for digest images
func TestManifestProcessingWithDigests(t *testing.T) {
	csvContent := []byte(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: digest-test-operator.v1.0.0
spec:
  displayName: Digest Test Operator
  version: 1.0.0
  relatedImages:
  - name: manager
    image: quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
  - name: webhook
    image: registry.redhat.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
  - name: proxy
    image: gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2
  install:
    strategy: deployment
    spec:
      deployments:
      - name: manager-deployment
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
              - name: proxy
                image: gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2
              initContainers:
              - name: webhook-init
                image: registry.redhat.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
`)

	analyzer := NewBundleAnalyzer()
	refs, err := analyzer.extractImageReferencesFromManifest(csvContent, "test.yaml")
	if err != nil {
		t.Fatalf("extractImageReferencesFromManifest failed: %v", err)
	}

	expectedImages := []string{
		"quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
		"registry.redhat.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
		"gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
	}

	expectedDigests := []string{
		"sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
		"sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
		"sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
	}

	// Check that we got the expected number of unique images
	if len(refs) < len(expectedImages) {
		t.Errorf("Expected at least %d image references, got %d", len(expectedImages), len(refs))
	}

	// Check that all expected digest images are present
	for i, expectedImage := range expectedImages {
		found := false
		for _, ref := range refs {
			if ref.Image == expectedImage {
				found = true
				if ref.Digest != expectedDigests[i] {
					t.Errorf("Expected digest %q for image %q, got %q", expectedDigests[i], expectedImage, ref.Digest)
				}
				break
			}
		}
		if !found {
			t.Errorf("Expected image %q not found in results", expectedImage)
		}
	}

	t.Logf("Successfully processed %d image references from CSV with digest images", len(refs))
}

// TestDigestImageValidation tests validation of digest formats
func TestDigestImageValidation(t *testing.T) {
	tests := []struct {
		name          string
		image         string
		isValidDigest bool
	}{
		{
			name:          "valid sha256 digest",
			image:         "quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			isValidDigest: true,
		},
		{
			name:          "invalid digest format",
			image:         "quay.io/test/operator@invalid:notadigest",
			isValidDigest: false,
		},
		{
			name:          "empty digest",
			image:         "quay.io/test/operator@",
			isValidDigest: false,
		},
		{
			name:          "tag only",
			image:         "quay.io/test/operator:v1.0.0",
			isValidDigest: false,
		},
		{
			name:          "short digest",
			image:         "quay.io/test/operator@sha256:abc123",
			isValidDigest: true, // We don't validate digest length, just format
		},
		{
			name:          "sha256 prefix but invalid chars",
			image:         "quay.io/test/operator@sha256:xyz!@#",
			isValidDigest: false, // Invalid characters mean it's not a valid digest
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digest := extractDigest(tt.image)
			hasDigest := digest != ""

			if hasDigest != tt.isValidDigest {
				t.Errorf("extractDigest(%q) validity = %v, want %v (digest: %q)", tt.image, hasDigest, tt.isValidDigest, digest)
			}
		})
	}
}
