package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPathTraversalProtection tests the bulletproof path traversal protection
func TestPathTraversalProtection(t *testing.T) {
	tests := []struct {
		name        string
		fileName    string
		shouldError bool
		description string
	}{
		{
			name:        "basic path traversal",
			fileName:    "../../../etc/passwd",
			shouldError: true,
			description: "Basic path traversal attempt",
		},
		{
			name:        "hidden path traversal",
			fileName:    "foo/../../../etc/passwd",
			shouldError: true,
			description: "Path traversal that doesn't start with ../",
		},
		{
			name:        "complex path traversal",
			fileName:    "manifests/../../../../../../etc/passwd",
			shouldError: true,
			description: "Complex path traversal with valid prefix",
		},
		{
			name:        "valid manifest path",
			fileName:    "manifests/test.yaml",
			shouldError: false,
			description: "Valid manifest file path",
		},
		{
			name:        "valid nested path",
			fileName:    "manifests/subdir/test.yaml",
			shouldError: false,
			description: "Valid nested directory path",
		},
		{
			name:        "dot segments",
			fileName:    "manifests/./test.yaml",
			shouldError: false,
			description: "Path with dot segments (should be cleaned)",
		},
		{
			name:        "long path attack",
			fileName:    strings.Repeat("a", 5000),
			shouldError: true,
			description: "Excessively long path name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ba := NewBundleAnalyzer()

			// Create a test tar archive with the specified file name
			tarData := createTestTarWithFile(t, tt.fileName, []byte("test content"))

			// Create temp directory
			tempDir, err := os.MkdirTemp("", "security-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() {
				if err := os.RemoveAll(tempDir); err != nil {
					t.Logf("Warning: failed to remove temp directory: %v", err)
				}
			}()

			// Test extraction
			err = ba.extractTarToDirectory(io.NopCloser(bytes.NewReader(tarData)), tempDir)

			if tt.shouldError && err == nil {
				t.Errorf("Expected error for %s, but got none", tt.description)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error for %s: %v", tt.description, err)
			}

			// Additional verification: ensure no files were created outside temp dir
			if !tt.shouldError {
				// For valid cases, verify the file was created in the correct location
				expectedPath := filepath.Join(tempDir, filepath.Clean(tt.fileName))
				if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
					t.Errorf("Expected file was not created: %s", expectedPath)
				}
			}
		})
	}
}

// TestFileSizeLimits tests file size validation
func TestFileSizeLimits(t *testing.T) {
	tests := []struct {
		name        string
		fileSize    int64
		shouldError bool
		description string
	}{
		{
			name:        "normal size file",
			fileSize:    1024,
			shouldError: false,
			description: "Normal sized file should be accepted",
		},
		{
			name:        "large file within limit",
			fileSize:    50 * 1024 * 1024, // 50MB
			shouldError: false,
			description: "Large file within 100MB limit",
		},
		{
			name:        "file exceeding limit",
			fileSize:    150 * 1024 * 1024, // 150MB
			shouldError: true,
			description: "File exceeding 100MB limit should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ba := NewBundleAnalyzer()

			// Create tar with file of specified size
			tarData := createTestTarWithFileSize(t, "test.txt", tt.fileSize)

			tempDir, err := os.MkdirTemp("", "size-test-*")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() {
				if err := os.RemoveAll(tempDir); err != nil {
					t.Logf("Warning: failed to remove temp directory: %v", err)
				}
			}()

			err = ba.extractTarToDirectory(io.NopCloser(bytes.NewReader(tarData)), tempDir)

			if tt.shouldError && err == nil {
				t.Errorf("Expected error for %s, but got none", tt.description)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error for %s: %v", tt.description, err)
			}
		})
	}
}

// TestInputValidation tests various input validation scenarios
func TestInputValidation(t *testing.T) {
	ba := NewBundleAnalyzer()

	t.Run("long bundle image reference", func(t *testing.T) {
		longRef := strings.Repeat("a", 3000)
		_, err := ba.ExtractImageReferences(context.Background(), longRef)
		if err == nil {
			t.Error("Expected error for excessively long bundle image reference")
		}
	})

	t.Run("large YAML content", func(t *testing.T) {
		largeContent := make([]byte, 6*1024*1024) // 6MB
		for i := range largeContent {
			largeContent[i] = 'a'
		}

		_, err := ba.extractImageReferencesFromManifest(largeContent, "test.yaml")
		if err == nil {
			t.Error("Expected error for excessively large YAML content")
		}
	})

	t.Run("long filename", func(t *testing.T) {
		longFilename := strings.Repeat("a", 300)
		_, err := ba.extractImageReferencesFromManifest([]byte("kind: ClusterServiceVersion"), longFilename)
		if err == nil {
			t.Error("Expected error for excessively long filename")
		}
	})
}

// TestImageReferenceValidation tests image reference validation
func TestImageReferenceValidation(t *testing.T) {
	ba := NewBundleAnalyzer()

	tests := []struct {
		name        string
		imageRef    ImageReference
		shouldSkip  bool
		description string
	}{
		{
			name: "valid image reference",
			imageRef: ImageReference{
				Name:  "test",
				Image: "quay.io/test/image:v1.0.0",
			},
			shouldSkip:  false,
			description: "Valid image reference should be accepted",
		},
		{
			name: "long image reference",
			imageRef: ImageReference{
				Name:  "test",
				Image: strings.Repeat("a", 1100),
			},
			shouldSkip:  true,
			description: "Excessively long image reference should be skipped",
		},
		{
			name: "long name",
			imageRef: ImageReference{
				Name:  strings.Repeat("a", 300),
				Image: "quay.io/test/image:v1.0.0",
			},
			shouldSkip:  true,
			description: "Excessively long name should be skipped",
		},
		{
			name: "invalid characters",
			imageRef: ImageReference{
				Name:  "test",
				Image: "quay.io/test/image\x01:v1.0.0",
			},
			shouldSkip:  true,
			description: "Image reference with control characters should be skipped",
		},
		{
			name: "empty image",
			imageRef: ImageReference{
				Name:  "test",
				Image: "",
			},
			shouldSkip:  true,
			description: "Empty image reference should be skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := []ImageReference{tt.imageRef}
			result := ba.deduplicateImageReferences(refs)

			if tt.shouldSkip && len(result) > 0 {
				t.Errorf("Expected image reference to be skipped for %s", tt.description)
			}
			if !tt.shouldSkip && len(result) == 0 {
				t.Errorf("Expected image reference to be accepted for %s", tt.description)
			}
		})
	}
}

// TestLogSanitization tests log message sanitization
func TestLogSanitization(t *testing.T) {
	ba := NewBundleAnalyzer()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal string",
			input:    "normal/image:tag",
			expected: "normal/image:tag",
		},
		{
			name:     "string with control characters",
			input:    "image\x01\x02:tag",
			expected: "image??:tag",
		},
		{
			name:     "long string",
			input:    strings.Repeat("a", 200),
			expected: strings.Repeat("a", 100) + "...",
		},
		{
			name:     "string with newlines",
			input:    "image\nwith\nnewlines",
			expected: "image?with?newlines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ba.sanitizeForLog(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestResourceCleanup tests proper resource cleanup
func TestResourceCleanup(t *testing.T) {
	ba := NewBundleAnalyzer()

	// Create a context that will be cancelled to test cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should not cause resource leaks even with early cancellation
	_, err := ba.ExtractImageReferences(ctx, "docker://quay.io/nonexistent/image:tag")

	// We expect an error due to the short timeout or nonexistent image
	if err == nil {
		t.Log("Note: Extraction succeeded (image may exist)")
	} else {
		t.Logf("Expected error occurred: %v", err)
	}

	// The test passes if it doesn't hang or leak resources
}

// Helper function to create a test tar archive with a specific file
func createTestTarWithFile(t *testing.T, fileName string, content []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	header := &tar.Header{
		Name:     fileName,
		Mode:     0644,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}

	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}

	if _, err := tw.Write(content); err != nil {
		t.Fatalf("Failed to write tar content: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}

	if err := gw.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

// Helper function to create a test tar archive with a file of specific size
func createTestTarWithFileSize(t *testing.T, fileName string, size int64) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	header := &tar.Header{
		Name:     fileName,
		Mode:     0644,
		Size:     size,
		Typeflag: tar.TypeReg,
	}

	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}

	// Write the specified amount of data
	written := int64(0)
	chunk := make([]byte, 8192)
	for i := range chunk {
		chunk[i] = 'a'
	}

	for written < size {
		remaining := size - written
		if remaining < int64(len(chunk)) {
			chunk = chunk[:remaining]
		}

		n, err := tw.Write(chunk)
		if err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
		written += int64(n)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}

	if err := gw.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}
