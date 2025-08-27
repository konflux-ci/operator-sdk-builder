package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
)

// BenchmarkParseImageReference benchmarks image reference parsing
func BenchmarkParseImageReference(b *testing.B) {
	resolver := NewImageResolver()

	testImages := []string{
		"nginx:latest",
		"quay.io/operator/controller:v1.0.0",
		"registry.redhat.io/ubi8/ubi@sha256:abc123def456",
		"gcr.io/my-project/subproject/app:latest",
		"registry.example.com:5000/myapp/service:v2.1.0",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, img := range testImages {
			_, _ = resolver.parseImageReference(img)
		}
	}
}

// BenchmarkReconstructImageReference benchmarks image reference reconstruction
func BenchmarkReconstructImageReference(b *testing.B) {
	resolver := NewImageResolver()

	parsed := &ParsedImageRef{
		Registry:   "quay.io",
		Repository: "operator/controller",
		Tag:        "v1.0.0",
		Digest:     "sha256:abc123def456",
	}

	newRegistry := "mirror.registry.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = resolver.reconstructImageReference(parsed, newRegistry)
	}
}

// BenchmarkMatchAndReplace benchmarks the image matching and replacement logic
func BenchmarkMatchAndReplace(b *testing.B) {
	resolver := NewImageResolver()

	imageRef := "registry.redhat.io/ubi8/ubi:latest"
	source := "registry.redhat.io/ubi8"
	mirrors := []string{"internal.registry.com/ubi"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resolver.matchAndReplace(imageRef, source, mirrors)
	}
}

// BenchmarkResolveImageReferences benchmarks the complete resolution process
func BenchmarkResolveImageReferences(b *testing.B) {
	tmpDir := b.TempDir()
	idmsFile := filepath.Join(tmpDir, "bench-idms.yaml")

	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: bench-test
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - mirror1.example.com
  - source: quay.io
    mirrors:
    - mirror2.example.com
  - source: docker.io
    mirrors:
    - mirror3.example.com
`

	if err := os.WriteFile(idmsFile, []byte(idmsContent), 0644); err != nil {
		b.Fatalf("Failed to write IDMS file: %v", err)
	}

	resolver := NewImageResolver()
	if err := resolver.LoadMirrorPolicy(idmsFile); err != nil {
		b.Fatalf("LoadMirrorPolicy failed: %v", err)
	}

	imageRefs := []bundle.ImageReference{
		{Image: "registry.redhat.io/ubi8/ubi:latest", Name: "ubi"},
		{Image: "quay.io/operator/controller:v1.0.0", Name: "controller"},
		{Image: "nginx:latest", Name: "nginx"},
		{Image: "gcr.io/project/app:latest", Name: "app"}, // No mapping
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resolver.ResolveImageReferences(imageRefs)
	}
}

// BenchmarkResolveImageReferencesLarge benchmarks resolution with many images
func BenchmarkResolveImageReferencesLarge(b *testing.B) {
	tmpDir := b.TempDir()
	idmsFile := filepath.Join(tmpDir, "large-bench-idms.yaml")

	// Create a large IDMS policy
	var policyBuilder strings.Builder
	policyBuilder.WriteString(`apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: large-bench-test
spec:
  imageDigestMirrors:
`)

	// Add 100 mirror policies
	for i := 0; i < 100; i++ {
		policyBuilder.WriteString("  - source: registry")
		policyBuilder.WriteString(string(rune(i)))
		policyBuilder.WriteString(".example.com\n    mirrors:\n    - mirror")
		policyBuilder.WriteString(string(rune(i)))
		policyBuilder.WriteString(".example.com\n")
	}

	if err := os.WriteFile(idmsFile, []byte(policyBuilder.String()), 0644); err != nil {
		b.Fatalf("Failed to write large IDMS file: %v", err)
	}

	resolver := NewImageResolver()
	if err := resolver.LoadMirrorPolicy(idmsFile); err != nil {
		b.Fatalf("LoadMirrorPolicy failed: %v", err)
	}

	// Create 1000 image references
	var imageRefs []bundle.ImageReference
	for i := 0; i < 1000; i++ {
		imageRefs = append(imageRefs, bundle.ImageReference{
			Image: "registry" + string(rune(i%100)) + ".example.com/app" + string(rune(i)) + ":latest",
			Name:  "app" + string(rune(i)),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resolver.ResolveImageReferences(imageRefs)
	}
}

// BenchmarkLoadMirrorPolicy benchmarks mirror policy loading
func BenchmarkLoadMirrorPolicy(b *testing.B) {
	tmpDir := b.TempDir()
	idmsFile := filepath.Join(tmpDir, "load-bench-idms.yaml")

	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: load-bench-test
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
		b.Fatalf("Failed to write IDMS file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver := NewImageResolver()
		_ = resolver.LoadMirrorPolicy(idmsFile)
	}
}

// BenchmarkExtractDigest benchmarks digest extraction
func BenchmarkExtractDigest(b *testing.B) {
	testImages := []string{
		"registry.io/app@sha256:abc123def456",
		"registry@domain.com/user@domain/app@sha256:abc123",
		"registry.io/app:v1.0.0", // No digest
		"registry.io/app@md5:def456",
		"registry.io/app@blake2b:" + strings.Repeat("x", 128),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, img := range testImages {
			_ = extractDigest(img)
		}
	}
}

// BenchmarkComplexMirrorMatching benchmarks complex mirror source matching
func BenchmarkComplexMirrorMatching(b *testing.B) {
	tmpDir := b.TempDir()
	idmsFile := filepath.Join(tmpDir, "complex-bench-idms.yaml")

	// Create overlapping mirror policies
	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: complex-bench-test
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
`

	if err := os.WriteFile(idmsFile, []byte(idmsContent), 0644); err != nil {
		b.Fatalf("Failed to write complex IDMS file: %v", err)
	}

	resolver := NewImageResolver()
	if err := resolver.LoadMirrorPolicy(idmsFile); err != nil {
		b.Fatalf("LoadMirrorPolicy failed: %v", err)
	}

	// Test images that will match different levels of specificity
	testImages := []bundle.ImageReference{
		{Image: "registry.redhat.io/ubi8/nodejs-16:latest", Name: "nodejs"}, // Most specific
		{Image: "registry.redhat.io/ubi8/ubi-minimal:latest", Name: "ubi"},  // Intermediate
		{Image: "registry.redhat.io/rhel8/httpd:latest", Name: "httpd"},     // General
		{Image: "quay.io/operator/controller:v1.0.0", Name: "controller"},   // Operator match
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resolver.ResolveImageReferences(testImages)
	}
}
