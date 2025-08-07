package bundle

import (
	"testing"
)

func TestNewBundleAnalyzer(t *testing.T) {
	analyzer := NewBundleAnalyzer()
	
	if analyzer == nil {
		t.Fatal("NewBundleAnalyzer returned nil")
	}
	
	if analyzer.systemContext == nil {
		t.Error("systemContext should not be nil")
	}
}

func TestExtractDigest(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "image with digest",
			image:    "registry.redhat.io/operator@sha256:abc123def456",
			expected: "sha256:abc123def456",
		},
		{
			name:     "image with tag only",
			image:    "registry.redhat.io/operator:v1.0.0",
			expected: "",
		},
		{
			name:     "image with both tag and digest",
			image:    "registry.redhat.io/operator:v1.0.0@sha256:abc123def456",
			expected: "sha256:abc123def456",
		},
		{
			name:     "image name only",
			image:    "registry.redhat.io/operator",
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

func TestIsManifestFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{
			name:     "valid CSV manifest",
			filename: "manifests/operator.clusterserviceversion.yaml",
			expected: true,
		},
		{
			name:     "valid CRD manifest",
			filename: "manifests/operator.crd.yaml",
			expected: true,
		},
		{
			name:     "metadata file",
			filename: "metadata/annotations.yaml",
			expected: false,
		},
		{
			name:     "hidden file",
			filename: "manifests/.hidden.yaml",
			expected: false,
		},
		{
			name:     "non-yaml file",
			filename: "manifests/README.md",
			expected: false,
		},
		{
			name:     "annotations file",
			filename: "manifests/annotations.yaml",
			expected: false,
		},
		{
			name:     "file not in manifests",
			filename: "other/file.yaml",
			expected: false,
		},
		{
			name:     "yml extension",
			filename: "manifests/operator.yml",
			expected: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isManifestFile(tt.filename)
			if result != tt.expected {
				t.Errorf("isManifestFile(%q) = %v, want %v", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestDeduplicateImageReferences(t *testing.T) {
	analyzer := NewBundleAnalyzer()
	
	refs := []ImageReference{
		{Image: "registry.redhat.io/operator:v1.0.0", Name: "operator"},
		{Image: "registry.redhat.io/controller:v1.0.0", Name: "controller"},
		{Image: "registry.redhat.io/operator:v1.0.0", Name: "operator-duplicate"},
		{Image: "registry.redhat.io/webhook:v1.0.0", Name: "webhook"},
		{Image: "registry.redhat.io/controller:v1.0.0", Name: "controller-duplicate"},
	}
	
	result := analyzer.deduplicateImageReferences(refs)
	
	// Should have 3 unique images
	if len(result) != 3 {
		t.Errorf("Expected 3 unique images, got %d", len(result))
	}
	
	// Check that we kept the first occurrence of each image
	expectedImages := []string{
		"registry.redhat.io/operator:v1.0.0",
		"registry.redhat.io/controller:v1.0.0", 
		"registry.redhat.io/webhook:v1.0.0",
	}
	
	for i, expected := range expectedImages {
		if result[i].Image != expected {
			t.Errorf("Expected image %d to be %q, got %q", i, expected, result[i].Image)
		}
	}
}