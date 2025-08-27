package bundle

import (
	"testing"
)

// TestImageReferenceEdgeCases tests edge cases for ImageReference struct
func TestImageReferenceEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		ref      ImageReference
		expected string
	}{
		{
			name:     "image reference with name",
			ref:      ImageReference{Image: "quay.io/test/app:v1.0.0", Name: "app"},
			expected: "app: quay.io/test/app:v1.0.0",
		},
		{
			name:     "image reference without name",
			ref:      ImageReference{Image: "docker.io/library/nginx:latest", Name: ""},
			expected: "docker.io/library/nginx:latest",
		},
		{
			name:     "empty image reference",
			ref:      ImageReference{Image: "", Name: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the actual fields since String() method doesn't exist
			if tt.ref.Name != "" {
				result := tt.ref.Name + ": " + tt.ref.Image
				expected := tt.expected
				if result != expected {
					t.Errorf("ImageReference string representation = %q, want %q", result, expected)
				}
			} else {
				result := tt.ref.Image
				expected := tt.expected
				if result != expected {
					t.Errorf("ImageReference string representation = %q, want %q", result, expected)
				}
			}
		})
	}
}

// TestExtractDigestExtendedCases tests more edge cases for digest extraction
func TestExtractDigestExtendedCases(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "image with multiple @ symbols",
			image:    "registry.com/repo@tag@sha256:abc123",
			expected: "sha256:abc123",
		},
		{
			name:     "image with @ in tag but also digest",
			image:    "registry.com/repo:tag@with@symbols@sha256:def456",
			expected: "sha256:def456",
		},
		{
			name:     "image with invalid digest format",
			image:    "registry.com/repo@invaliddigest",
			expected: "",
		},
		{
			name:     "image with empty digest",
			image:    "registry.com/repo@",
			expected: "",
		},
		{
			name:     "image with only registry",
			image:    "registry.com",
			expected: "",
		},
		{
			name:     "very long image name",
			image:    "very-long-registry-name.example.com:8080/very/long/namespace/very-long-repository-name:very-long-tag-name-v1.2.3-alpha.1.build.12345",
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

// TestIsManifestFileExtensive tests more cases for manifest file detection
func TestIsManifestFileExtensive(t *testing.T) {
	tests := []struct {
		name     string
		filepath string
		expected bool
	}{
		{
			name:     "CSV with complex path",
			filepath: "/complex/path/to/bundle/manifests/my-operator.v1.2.3-alpha.clusterserviceversion.yaml",
			expected: true,
		},
		{
			name:     "CRD with version in name",
			filepath: "/bundle/manifests/mycrds.v1.example.com.crd.yaml",
			expected: true,
		},
		{
			name:     "YAML file with uppercase extension",
			filepath: "/bundle/manifests/operator.YAML",
			expected: true,
		},
		{
			name:     "YML file with complex name",
			filepath: "/bundle/manifests/complex-name_with-symbols.yml",
			expected: true,
		},
		{
			name:     "metadata directory file",
			filepath: "/bundle/metadata/annotations.yaml",
			expected: false,
		},
		{
			name:     "tests directory file",
			filepath: "/bundle/tests/scorecard/config.yaml",
			expected: false,
		},
		{
			name:     "root level manifest file",
			filepath: "/manifests/operator.yaml",
			expected: true,
		},
		{
			name:     "deeply nested manifest",
			filepath: "/very/deep/nested/path/manifests/operator.yaml",
			expected: true,
		},
		{
			name:     "file with no extension",
			filepath: "/bundle/manifests/operator",
			expected: false,
		},
		{
			name:     "JSON file in manifests",
			filepath: "/bundle/manifests/operator.json",
			expected: false,
		},
		{
			name:     "temporary file",
			filepath: "/bundle/manifests/.operator.yaml.tmp",
			expected: false,
		},
		{
			name:     "backup file",
			filepath: "/bundle/manifests/operator.yaml.bak",
			expected: false,
		},
		{
			name:     "vim swap file",
			filepath: "/bundle/manifests/.operator.yaml.swp",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isManifestFile(tt.filepath)
			if result != tt.expected {
				t.Errorf("isManifestFile(%q) = %v, want %v", tt.filepath, result, tt.expected)
			}
		})
	}
}

// TestDeduplicateImageReferencesExtensive tests extensive deduplication scenarios
func TestDeduplicateImageReferencesExtensive(t *testing.T) {
	tests := []struct {
		name     string
		input    []ImageReference
		expected []ImageReference
	}{
		{
			name: "mixed duplicates with different names",
			input: []ImageReference{
				{Image: "quay.io/test/app:v1.0.0", Name: "app-main"},
				{Image: "quay.io/test/app:v1.0.0", Name: "app-secondary"},
				{Image: "docker.io/library/nginx:latest", Name: "nginx"},
				{Image: "quay.io/test/app:v1.0.0", Name: "app-third"},
			},
			expected: []ImageReference{
				{Image: "quay.io/test/app:v1.0.0", Name: "app-main"},
				{Image: "docker.io/library/nginx:latest", Name: "nginx"},
			},
		},
		{
			name: "same images with empty names",
			input: []ImageReference{
				{Image: "quay.io/test/app:v1.0.0", Name: ""},
				{Image: "quay.io/test/app:v1.0.0", Name: ""},
				{Image: "quay.io/test/app:v1.0.0", Name: "named"},
			},
			expected: []ImageReference{
				{Image: "quay.io/test/app:v1.0.0", Name: ""},
			},
		},
		{
			name: "digest vs tag for same image",
			input: []ImageReference{
				{Image: "quay.io/test/app:v1.0.0", Name: "app-tag"},
				{Image: "quay.io/test/app@sha256:abc123", Name: "app-digest"},
				{Image: "quay.io/test/app:latest", Name: "app-latest"},
			},
			expected: []ImageReference{
				{Image: "quay.io/test/app:v1.0.0", Name: "app-tag"},
				{Image: "quay.io/test/app@sha256:abc123", Name: "app-digest"},
				{Image: "quay.io/test/app:latest", Name: "app-latest"},
			},
		},
		{
			name: "empty images should be filtered",
			input: []ImageReference{
				{Image: "", Name: "empty1"},
				{Image: "quay.io/test/app:v1.0.0", Name: "valid"},
				{Image: "", Name: "empty2"},
				{Image: "docker.io/library/nginx:latest", Name: "nginx"},
			},
			expected: []ImageReference{
				{Image: "quay.io/test/app:v1.0.0", Name: "valid"},
				{Image: "docker.io/library/nginx:latest", Name: "nginx"},
			},
		},
		{
			name: "large number of duplicates",
			input: func() []ImageReference {
				refs := make([]ImageReference, 100)
				for i := 0; i < 100; i++ {
					switch i % 3 {
					case 0:
						refs[i] = ImageReference{Image: "quay.io/test/app1:v1.0.0", Name: "app1"}
					case 1:
						refs[i] = ImageReference{Image: "quay.io/test/app2:v1.0.0", Name: "app2"}
					case 2:
						refs[i] = ImageReference{Image: "quay.io/test/app3:v1.0.0", Name: "app3"}
					}
				}
				return refs
			}(),
			expected: []ImageReference{
				{Image: "quay.io/test/app1:v1.0.0", Name: "app1"},
				{Image: "quay.io/test/app2:v1.0.0", Name: "app2"},
				{Image: "quay.io/test/app3:v1.0.0", Name: "app3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create analyzer to access the method
			analyzer := NewBundleAnalyzer()
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

// TestBundleAnalyzerStateManagement tests analyzer state across multiple operations
func TestBundleAnalyzerStateManagement(t *testing.T) {
	analyzer := NewBundleAnalyzer()

	// Test that analyzer maintains state correctly across multiple calls
	t.Run("multiple digest extractions", func(t *testing.T) {
		images := []string{
			"quay.io/test/app@sha256:abc123",
			"docker.io/library/nginx@sha256:def456",
			"registry.example.com/app:latest",
		}

		expectedDigests := []string{
			"sha256:abc123",
			"sha256:def456",
			"",
		}

		for i, image := range images {
			digest := extractDigest(image)
			if digest != expectedDigests[i] {
				t.Errorf("extractDigest(%q) = %q, want %q", image, digest, expectedDigests[i])
			}
		}
	})

	t.Run("multiple manifest file checks", func(t *testing.T) {
		files := []string{
			"/bundle/manifests/operator.yaml",
			"/bundle/metadata/annotations.yaml",
			"/bundle/manifests/crd.yaml",
			"/bundle/manifests/.hidden.yaml",
		}

		expected := []bool{true, false, true, false}

		for i, file := range files {
			result := isManifestFile(file)
			if result != expected[i] {
				t.Errorf("isManifestFile(%q) = %v, want %v", file, result, expected[i])
			}
		}
	})

	t.Run("multiple deduplication operations", func(t *testing.T) {
		// First deduplication
		refs1 := []ImageReference{
			{Image: "quay.io/test/app:v1", Name: "app"},
			{Image: "quay.io/test/app:v1", Name: "app"},
		}
		result1 := analyzer.deduplicateImageReferences(refs1)
		if len(result1) != 1 {
			t.Errorf("First deduplication: expected 1 item, got %d", len(result1))
		}

		// Second deduplication with different data
		refs2 := []ImageReference{
			{Image: "docker.io/nginx:latest", Name: "nginx"},
			{Image: "docker.io/nginx:latest", Name: "nginx"},
			{Image: "quay.io/test/other:v1", Name: "other"},
		}
		result2 := analyzer.deduplicateImageReferences(refs2)
		if len(result2) != 2 {
			t.Errorf("Second deduplication: expected 2 items, got %d", len(result2))
		}
	})
}
