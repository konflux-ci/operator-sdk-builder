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

// BenchmarkExtractDigest benchmarks digest extraction performance
func BenchmarkExtractDigest(b *testing.B) {
	testImages := []string{
		"registry.redhat.io/operator@sha256:abc123def456",
		"registry.redhat.io/operator:v1.0.0",
		"registry.redhat.io/operator:v1.0.0@sha256:abc123def456",
		"registry.redhat.io/operator",
		"registry@domain.com/user@domain/app@sha256:abc123@extra",
		"registry.io/app@md5:xyz789",
		"registry.io/app@blake2b:" + strings.Repeat("x", 128),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, img := range testImages {
			_ = extractDigest(img)
		}
	}
}

// BenchmarkIsManifestFile benchmarks manifest file detection
func BenchmarkIsManifestFile(b *testing.B) {
	testFilenames := []string{
		"manifests/operator.clusterserviceversion.yaml",
		"manifests/operator.crd.yaml",
		"metadata/annotations.yaml",
		"manifests/.hidden.yaml",
		"manifests/README.md",
		"manifests/annotations.yaml",
		"other/file.yaml",
		"manifests/operator.yml",
		"manifests/subdir1/subdir2/operator.yaml",
		"manifests/" + strings.Repeat("very-long-filename-", 50) + ".yaml",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, filename := range testFilenames {
			_ = isManifestFile(filename)
		}
	}
}

// BenchmarkDeduplicateImageReferences benchmarks image deduplication
func BenchmarkDeduplicateImageReferences(b *testing.B) {
	analyzer := NewBundleAnalyzer()

	// Create many image references with duplicates
	var refs []ImageReference
	for i := 0; i < 1000; i++ {
		refs = append(refs, ImageReference{
			Image: fmt.Sprintf("registry.redhat.io/image%d:v1.0.0", i%100), // 100 unique images, 10x duplicates each
			Name:  fmt.Sprintf("image%d", i),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = analyzer.deduplicateImageReferences(refs)
	}
}

// BenchmarkExtractTarToDirectory benchmarks tar extraction
func BenchmarkExtractTarToDirectory(b *testing.B) {
	analyzer := NewBundleAnalyzer()

	// Create a tar archive with multiple files
	var tarBuffer bytes.Buffer
	gw := gzip.NewWriter(&tarBuffer)
	tw := tar.NewWriter(gw)

	// Add 100 files to the tar
	for i := 0; i < 100; i++ {
		filename := fmt.Sprintf("manifests/operator-%d.yaml", i)
		content := fmt.Sprintf("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config-%d", i)

		header := &tar.Header{
			Name:     filename,
			Size:     int64(len(content)),
			Mode:     0644,
			Typeflag: tar.TypeReg,
		}

		if err := tw.WriteHeader(header); err != nil {
			b.Fatalf("Failed to write tar header: %v", err)
		}

		if _, err := tw.Write([]byte(content)); err != nil {
			b.Fatalf("Failed to write tar content: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		b.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		b.Fatalf("Failed to close gzip writer: %v", err)
	}

	tarData := tarBuffer.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir, err := os.MkdirTemp("", "bench-test-*")
		if err != nil {
			b.Fatalf("Failed to create temp directory: %v", err)
		}

		reader := io.NopCloser(bytes.NewReader(tarData))
		_ = analyzer.extractTarToDirectory(reader, tempDir)

		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Warning: failed to remove temp directory: %v", err)
		}
	}
}

// BenchmarkExtractTarToDirectoryLarge benchmarks tar extraction with large files
func BenchmarkExtractTarToDirectoryLarge(b *testing.B) {
	analyzer := NewBundleAnalyzer()

	// Create a tar archive with one large file
	var tarBuffer bytes.Buffer
	gw := gzip.NewWriter(&tarBuffer)
	tw := tar.NewWriter(gw)

	// Create a 1MB file content
	largeContent := strings.Repeat("x", 1024*1024)

	header := &tar.Header{
		Name:     "manifests/large-operator.yaml",
		Size:     int64(len(largeContent)),
		Mode:     0644,
		Typeflag: tar.TypeReg,
	}

	if err := tw.WriteHeader(header); err != nil {
		b.Fatalf("Failed to write tar header: %v", err)
	}

	if _, err := tw.Write([]byte(largeContent)); err != nil {
		b.Fatalf("Failed to write tar content: %v", err)
	}

	if err := tw.Close(); err != nil {
		b.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		b.Fatalf("Failed to close gzip writer: %v", err)
	}

	tarData := tarBuffer.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir, err := os.MkdirTemp("", "bench-large-test-*")
		if err != nil {
			b.Fatalf("Failed to create temp directory: %v", err)
		}

		reader := io.NopCloser(bytes.NewReader(tarData))
		_ = analyzer.extractTarToDirectory(reader, tempDir)

		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Warning: failed to remove temp directory: %v", err)
		}
	}
}

// BenchmarkMemoryUsageWithManyFiles benchmarks memory usage with many small files
func BenchmarkMemoryUsageWithManyFiles(b *testing.B) {
	analyzer := NewBundleAnalyzer()

	// Create a tar archive with many small files
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
			b.Fatalf("Failed to write tar header: %v", err)
		}

		if _, err := tw.Write([]byte(content)); err != nil {
			b.Fatalf("Failed to write tar content: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		b.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		b.Fatalf("Failed to close gzip writer: %v", err)
	}

	tarData := tarBuffer.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir, err := os.MkdirTemp("", "bench-memory-test-*")
		if err != nil {
			b.Fatalf("Failed to create temp directory: %v", err)
		}

		reader := io.NopCloser(bytes.NewReader(tarData))
		_ = analyzer.extractTarToDirectory(reader, tempDir)

		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Warning: failed to remove temp directory: %v", err)
		}
	}
}

// BenchmarkPathTraversalChecks benchmarks security path validation
func BenchmarkPathTraversalChecks(b *testing.B) {
	analyzer := NewBundleAnalyzer()

	// Create a tar archive with various path types (both safe and unsafe)
	var tarBuffer bytes.Buffer
	gw := gzip.NewWriter(&tarBuffer)
	tw := tar.NewWriter(gw)

	testPaths := []string{
		"manifests/operator.yaml",
		"../../../etc/passwd",
		"..",
		"/etc/passwd",
		"manifests/../../../tmp/malicious",
		"manifests/subdir/operator.yaml",
	}

	for _, path := range testPaths {
		content := "test content"
		header := &tar.Header{
			Name:     path,
			Size:     int64(len(content)),
			Mode:     0644,
			Typeflag: tar.TypeReg,
		}

		if err := tw.WriteHeader(header); err != nil {
			b.Fatalf("Failed to write tar header: %v", err)
		}

		if _, err := tw.Write([]byte(content)); err != nil {
			b.Fatalf("Failed to write tar content: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		b.Fatalf("Failed to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		b.Fatalf("Failed to close gzip writer: %v", err)
	}

	tarData := tarBuffer.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir, err := os.MkdirTemp("", "bench-security-test-*")
		if err != nil {
			b.Fatalf("Failed to create temp directory: %v", err)
		}

		reader := io.NopCloser(bytes.NewReader(tarData))
		_ = analyzer.extractTarToDirectory(reader, tempDir)

		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Warning: failed to remove temp directory: %v", err)
		}
	}
}

// BenchmarkFilePathOperations benchmarks filepath operations used in validation
func BenchmarkFilePathOperations(b *testing.B) {
	testPaths := []string{
		"manifests/operator.yaml",
		"../../../etc/passwd",
		"manifests/../../../tmp/malicious",
		"manifests/subdir/operator.yaml",
		"/etc/passwd",
		"..",
		strings.Repeat("very-long-path-segment/", 20) + "file.yaml",
	}

	tempDir := "/tmp/test-base"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, path := range testPaths {
			// Benchmark the security checks that are performed
			cleanPath := filepath.Clean(path)
			_ = filepath.IsAbs(cleanPath)
			_ = strings.Contains(cleanPath, "..")
			fullPath := filepath.Join(tempDir, cleanPath)
			_, _ = filepath.Rel(tempDir, fullPath)
		}
	}
}

// BenchmarkImageReferenceCreation benchmarks creating ImageReference structs
func BenchmarkImageReferenceCreation(b *testing.B) {
	testImages := []string{
		"registry.redhat.io/operator:v1.0.0",
		"quay.io/test/controller:latest",
		"gcr.io/project/app@sha256:abc123",
		"nginx:latest",
		"registry.example.com:5000/app:v2.0.0",
	}

	testNames := []string{
		"operator",
		"controller",
		"app",
		"nginx",
		"app-service",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var refs []ImageReference
		for j, img := range testImages {
			refs = append(refs, ImageReference{
				Image: img,
				Name:  testNames[j%len(testNames)],
			})
		}
		_ = refs
	}
}
