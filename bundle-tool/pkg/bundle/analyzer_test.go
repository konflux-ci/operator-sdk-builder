package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// TestExtractTarToDirectory_SecurityChecks tests the path traversal protection
func TestExtractTarToDirectory_SecurityChecks(t *testing.T) {
	analyzer := NewBundleAnalyzer()

	tests := []struct {
		name       string
		filename   string
		content    string
		shouldFail bool
		errorMsg   string
	}{
		{
			name:       "valid file path",
			filename:   "manifests/operator.yaml",
			content:    "apiVersion: v1\nkind: ConfigMap",
			shouldFail: false,
		},
		{
			name:       "path traversal with ../",
			filename:   "../../../etc/passwd",
			content:    "sensitive content",
			shouldFail: true,
			errorMsg:   "path traversal attempt detected",
		},
		{
			name:       "path traversal with ..",
			filename:   "..",
			content:    "sensitive content",
			shouldFail: true,
			errorMsg:   "path traversal attempt detected",
		},
		{
			name:       "absolute path attack",
			filename:   "/etc/passwd",
			content:    "sensitive content",
			shouldFail: true,
			errorMsg:   "absolute path not allowed",
		},
		{
			name:       "complex path traversal",
			filename:   "manifests/../../../tmp/malicious",
			content:    "malicious content",
			shouldFail: true,
			errorMsg:   "path traversal attempt detected",
		},
		{
			name:       "valid nested path",
			filename:   "manifests/subdir/operator.yaml",
			content:    "apiVersion: v1\nkind: ConfigMap",
			shouldFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for testing
			tempDir, err := os.MkdirTemp("", "bundle-security-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp directory: %v", err)
			}
			defer func() {
				if err := os.RemoveAll(tempDir); err != nil {
					t.Logf("Warning: failed to remove temp directory: %v", err)
				}
			}()

			// Create a gzipped tar archive with the test file
			var tarBuffer bytes.Buffer
			gw := gzip.NewWriter(&tarBuffer)
			tw := tar.NewWriter(gw)

			header := &tar.Header{
				Name:     tt.filename,
				Size:     int64(len(tt.content)),
				Mode:     0644,
				Typeflag: tar.TypeReg,
			}

			if err := tw.WriteHeader(header); err != nil {
				t.Fatalf("Failed to write tar header: %v", err)
			}

			if _, err := tw.Write([]byte(tt.content)); err != nil {
				t.Fatalf("Failed to write tar content: %v", err)
			}

			if err := tw.Close(); err != nil {
				t.Fatalf("Failed to close tar writer: %v", err)
			}
			if err := gw.Close(); err != nil {
				t.Fatalf("Failed to close gzip writer: %v", err)
			}

			// Test extraction
			reader := io.NopCloser(bytes.NewReader(tarBuffer.Bytes()))
			err = analyzer.extractTarToDirectory(reader, tempDir)

			if tt.shouldFail {
				if err == nil {
					t.Errorf("Expected extraction to fail for %s, but it succeeded", tt.filename)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected extraction to succeed for %s, but got error: %v", tt.filename, err)
				} else {
					// Verify the file was created in the expected location
					expectedPath := filepath.Join(tempDir, tt.filename)
					if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
						t.Errorf("Expected file %s to be created, but it doesn't exist", expectedPath)
					}
				}
			}
		})
	}
}

// TestExtractTarToDirectory_SymlinkSecurity tests that symlinks are properly handled
func TestExtractTarToDirectory_SymlinkSecurity(t *testing.T) {
	analyzer := NewBundleAnalyzer()

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "bundle-symlink-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp directory: %v", err)
		}
	}()

	// Create a gzipped tar archive with a symlink
	var tarBuffer bytes.Buffer
	gw := gzip.NewWriter(&tarBuffer)
	tw := tar.NewWriter(gw)

	// Add a symlink that points outside the extraction directory
	header := &tar.Header{
		Name:     "malicious-symlink",
		Linkname: "../../sensitive-file",
		Typeflag: tar.TypeSymlink,
	}

	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	// Test extraction - should succeed but skip the symlink
	reader := io.NopCloser(bytes.NewReader(tarBuffer.Bytes()))
	err = analyzer.extractTarToDirectory(reader, tempDir)

	if err != nil {
		t.Errorf("Expected extraction to succeed (skipping symlink), but got error: %v", err)
	}

	// Verify that the symlink was not created
	symlinkPath := filepath.Join(tempDir, "malicious-symlink")
	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Errorf("Expected symlink to be skipped, but it was created")
	}
}

// TestExtractDigestEdgeCases tests edge cases for digest extraction
func TestExtractDigestEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "malformed image with multiple @ symbols",
			image:    "registry@domain.com/user@domain/app@sha256:abc123@extra",
			expected: "", // Invalid format - @extra makes it not a valid sha256 digest
		},
		{
			name:     "image with @ in registry hostname",
			image:    "user@registry.com/app@sha256:def456",
			expected: "sha256:def456",
		},
		{
			name:     "empty string after last @",
			image:    "registry.io/app@",
			expected: "",
		},
		{
			name:     "no @ symbol at all",
			image:    "registry.io/app:latest",
			expected: "",
		},
		{
			name:     "only @ symbol",
			image:    "@",
			expected: "",
		},
		{
			name:     "@ at the beginning",
			image:    "@sha256:abc123",
			expected: "sha256:abc123",
		},
		{
			name:     "non-sha256 digest formats",
			image:    "registry.io/app@md5:xyz789",
			expected: "", // Only sha256 digests are supported
		},
		{
			name:     "malformed digest without colon",
			image:    "registry.io/app@sha256abc123",
			expected: "", // Invalid format - missing colon after sha256
		},
		{
			name:     "extremely long digest",
			image:    "registry.io/app@sha256:" + strings.Repeat("a", 1000),
			expected: "sha256:" + strings.Repeat("a", 1000),
		},
		{
			name:     "digest with special characters",
			image:    "registry.io/app@algorithm:digest-with-special_chars.123",
			expected: "", // Only sha256 digests are supported
		},
		{
			name:     "empty image string",
			image:    "",
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

// TestDeduplicateImageReferencesEdgeCases tests edge cases for image deduplication
func TestDeduplicateImageReferencesEdgeCases(t *testing.T) {
	analyzer := NewBundleAnalyzer()

	tests := []struct {
		name     string
		input    []ImageReference
		expected []ImageReference
	}{
		{
			name:     "empty input",
			input:    []ImageReference{},
			expected: []ImageReference{},
		},
		{
			name: "single element",
			input: []ImageReference{
				{Image: "registry.io/app:latest", Name: "app"},
			},
			expected: []ImageReference{
				{Image: "registry.io/app:latest", Name: "app"},
			},
		},
		{
			name:     "nil input",
			input:    nil,
			expected: []ImageReference{},
		},
		{
			name: "images with different names but same image reference",
			input: []ImageReference{
				{Image: "registry.io/app:latest", Name: "app1"},
				{Image: "registry.io/app:latest", Name: "app2"},
				{Image: "registry.io/app:latest", Name: "app3"},
			},
			expected: []ImageReference{
				{Image: "registry.io/app:latest", Name: "app1"},
			},
		},
		{
			name: "empty image references",
			input: []ImageReference{
				{Image: "", Name: "empty1"},
				{Image: "registry.io/app:latest", Name: "app"},
				{Image: "", Name: "empty2"},
			},
			expected: []ImageReference{
				{Image: "registry.io/app:latest", Name: "app"},
			},
		},
		{
			name: "whitespace in image references",
			input: []ImageReference{
				{Image: "  registry.io/app:latest  ", Name: "app1"},
				{Image: "registry.io/app:latest", Name: "app2"},
				{Image: "registry.io/app:latest  ", Name: "app3"},
			},
			expected: []ImageReference{
				{Image: "  registry.io/app:latest  ", Name: "app1"},
				{Image: "registry.io/app:latest", Name: "app2"},
				{Image: "registry.io/app:latest  ", Name: "app3"},
			},
		},
		{
			name: "case sensitive deduplication",
			input: []ImageReference{
				{Image: "registry.io/app:latest", Name: "app1"},
				{Image: "Registry.io/app:latest", Name: "app2"},
				{Image: "registry.io/App:latest", Name: "app3"},
			},
			expected: []ImageReference{
				{Image: "registry.io/app:latest", Name: "app1"},
				{Image: "Registry.io/app:latest", Name: "app2"},
				{Image: "registry.io/App:latest", Name: "app3"},
			},
		},
		{
			name: "extremely long image references",
			input: []ImageReference{
				{Image: strings.Repeat("very-long-registry-name.", 100) + "com/app:latest", Name: "long1"},
				{Image: strings.Repeat("very-long-registry-name.", 100) + "com/app:latest", Name: "long2"},
			},
			expected: []ImageReference{
				// Security: Long image references are now filtered out
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.deduplicateImageReferences(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d results, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i].Image != expected.Image || result[i].Name != expected.Name {
					t.Errorf("Result %d: expected %+v, got %+v", i, expected, result[i])
				}
			}
		})
	}
}

// TestIsManifestFileEdgeCases tests edge cases for manifest file detection
func TestIsManifestFileEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{
			name:     "empty filename",
			filename: "",
			expected: false,
		},
		{
			name:     "filename with only extension",
			filename: ".yaml",
			expected: false,
		},
		{
			name:     "filename with multiple dots",
			filename: "manifests/operator.service.version.yaml",
			expected: true,
		},
		{
			name:     "uppercase extension",
			filename: "manifests/operator.YAML",
			expected: true, // Case-insensitive extension matching
		},
		{
			name:     "mixed case directory",
			filename: "Manifests/operator.yaml",
			expected: false, // Directory should be exactly "manifests"
		},
		{
			name:     "directory with trailing slash",
			filename: "manifests/",
			expected: false,
		},
		{
			name:     "deeply nested in manifests",
			filename: "manifests/subdir1/subdir2/operator.yaml",
			expected: true,
		},
		{
			name:     "file starting with dot in manifests",
			filename: "manifests/.operator.yaml",
			expected: false, // Hidden files should be excluded
		},
		{
			name:     "annotations.yaml in manifests",
			filename: "manifests/annotations.yaml",
			expected: false, // Should be specifically excluded
		},
		{
			name:     "file with no extension in manifests",
			filename: "manifests/operator",
			expected: false,
		},
		{
			name:     "yaml file in metadata directory",
			filename: "metadata/operator.yaml",
			expected: false,
		},
		{
			name:     "path with parent directory references",
			filename: "manifests/../manifests/operator.yaml",
			expected: true, // Should still be detected as in manifests
		},
		{
			name:     "extremely long filename",
			filename: "manifests/" + strings.Repeat("very-long-filename-", 50) + ".yaml",
			expected: true,
		},
		{
			name:     "filename with unicode characters",
			filename: "manifests/操作员.yaml",
			expected: true,
		},
		{
			name:     "filename with special characters",
			filename: "manifests/operator-bundle_v1.0.0.yaml",
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

// TestBundleAnalyzerResourceLimits tests resource limit handling
func TestBundleAnalyzerResourceLimits(t *testing.T) {
	analyzer := NewBundleAnalyzer()

	t.Run("extremely large tar archive", func(t *testing.T) {
		// Create a tar archive with many files to test memory usage
		var tarBuffer bytes.Buffer
		gw := gzip.NewWriter(&tarBuffer)
		tw := tar.NewWriter(gw)

		// Add 1000 small files
		for i := 0; i < 1000; i++ {
			filename := fmt.Sprintf("manifests/operator-%d.yaml", i)
			content := fmt.Sprintf("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config-%d", i)

			header := &tar.Header{
				Name:     filename,
				Size:     int64(len(content)),
				Mode:     0644,
				Typeflag: tar.TypeReg,
			}

			if err := tw.WriteHeader(header); err != nil {
				t.Fatalf("Failed to write tar header: %v", err)
			}

			if _, err := tw.Write([]byte(content)); err != nil {
				t.Fatalf("Failed to write tar content: %v", err)
			}
		}

		if err := tw.Close(); err != nil {
			t.Fatalf("Failed to close tar writer: %v", err)
		}
		if err := gw.Close(); err != nil {
			t.Fatalf("Failed to close gzip writer: %v", err)
		}

		// Test extraction
		tempDir, err := os.MkdirTemp("", "bundle-large-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				t.Logf("Warning: failed to remove temp directory: %v", err)
			}
		}()

		reader := io.NopCloser(bytes.NewReader(tarBuffer.Bytes()))
		err = analyzer.extractTarToDirectory(reader, tempDir)

		if err != nil {
			t.Errorf("Should handle large tar archives gracefully: %v", err)
		}

		// Verify some files were extracted
		manifestsDir := filepath.Join(tempDir, "manifests")
		files, err := os.ReadDir(manifestsDir)
		if err != nil {
			t.Errorf("Failed to read manifests directory: %v", err)
		}

		if len(files) != 1000 {
			t.Errorf("Expected 1000 files in manifests directory, got %d", len(files))
		}
	})

	t.Run("tar archive with extremely large single file", func(t *testing.T) {
		// Create a tar archive with one very large file
		var tarBuffer bytes.Buffer
		gw := gzip.NewWriter(&tarBuffer)
		tw := tar.NewWriter(gw)

		// Create a 10MB file content
		largeContent := strings.Repeat("x", 10*1024*1024)

		header := &tar.Header{
			Name:     "manifests/large-operator.yaml",
			Size:     int64(len(largeContent)),
			Mode:     0644,
			Typeflag: tar.TypeReg,
		}

		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}

		if _, err := tw.Write([]byte(largeContent)); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}

		if err := tw.Close(); err != nil {
			t.Fatalf("Failed to close tar writer: %v", err)
		}
		if err := gw.Close(); err != nil {
			t.Fatalf("Failed to close gzip writer: %v", err)
		}

		// Test extraction
		tempDir, err := os.MkdirTemp("", "bundle-large-file-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				t.Logf("Warning: failed to remove temp directory: %v", err)
			}
		}()

		reader := io.NopCloser(bytes.NewReader(tarBuffer.Bytes()))
		err = analyzer.extractTarToDirectory(reader, tempDir)

		if err != nil {
			t.Errorf("Should handle large files gracefully: %v", err)
		}

		// Verify the file was extracted
		extractedFile := filepath.Join(tempDir, "manifests", "large-operator.yaml")
		if _, err := os.Stat(extractedFile); os.IsNotExist(err) {
			t.Errorf("Large file was not extracted: %v", err)
		}
	})
}
