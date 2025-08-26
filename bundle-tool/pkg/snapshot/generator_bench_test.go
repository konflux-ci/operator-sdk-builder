package snapshot

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/provenance"
)

// BenchmarkGenerateSnapshot benchmarks snapshot generation
func BenchmarkGenerateSnapshot(b *testing.B) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	imageRefs := []bundle.ImageReference{
		{Image: "quay.io/test/controller:v1.0.0", Name: "controller"},
		{Image: "quay.io/test/webhook:v1.0.0", Name: "webhook"},
		{Image: "quay.io/test/manager:v1.0.0", Name: "manager"},
	}

	provenanceResults := []provenance.ProvenanceInfo{
		{
			Verified:     true,
			SourceRepo:   "https://github.com/test/controller",
			SourceCommit: "abc123def456",
		},
		{
			Verified:     true,
			SourceRepo:   "https://github.com/test/webhook",
			SourceCommit: "def456abc123",
		},
		{
			Verified:     true,
			SourceRepo:   "https://github.com/test/manager",
			SourceCommit: "789abc123def",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generator.GenerateSnapshot(
			context.Background(),
			imageRefs,
			provenanceResults,
			"quay.io/test/bundle:v1.0.0",
			nil,
		)
	}
}

// BenchmarkGenerateSnapshotLarge benchmarks snapshot generation with many images
func BenchmarkGenerateSnapshotLarge(b *testing.B) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	// Create 1000 image references
	var imageRefs []bundle.ImageReference
	var provenanceResults []provenance.ProvenanceInfo

	for i := 0; i < 1000; i++ {
		imageRefs = append(imageRefs, bundle.ImageReference{
			Image: fmt.Sprintf("quay.io/test/image%d:v1.0.0", i),
			Name:  fmt.Sprintf("image%d", i),
		})

		provenanceResults = append(provenanceResults, provenance.ProvenanceInfo{
			Verified:     true,
			SourceRepo:   fmt.Sprintf("https://github.com/test/repo%d", i),
			SourceCommit: fmt.Sprintf("commit%d", i),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generator.GenerateSnapshot(
			context.Background(),
			imageRefs,
			provenanceResults,
			"quay.io/test/bundle:v1.0.0",
			nil,
		)
	}
}

// BenchmarkDeduplicateComponents benchmarks component deduplication
func BenchmarkDeduplicateComponents(b *testing.B) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	// Create snapshot with many duplicate components
	components := make([]SnapshotComponent, 1000)
	for i := 0; i < 1000; i++ {
		components[i] = SnapshotComponent{
			Name:           fmt.Sprintf("component%d", i%100), // 100 unique names, 10x duplicates each
			ContainerImage: fmt.Sprintf("quay.io/test/image%d:v1.0.0", i%100),
		}
	}

	_ = components // Use the components variable

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make a copy for each iteration since deduplication modifies the slice
		testSnapshot := &KonfluxSnapshot{
			Spec: SnapshotSpec{
				Components: make([]SnapshotComponent, len(components)),
			},
		}
		copy(testSnapshot.Spec.Components, components)
		generator.DeduplicateComponents(testSnapshot)
	}
}

// BenchmarkValidateSnapshot benchmarks snapshot validation
func BenchmarkValidateSnapshot(b *testing.B) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	snapshot := &KonfluxSnapshot{
		APIVersion: "appstudio.redhat.com/v1alpha1",
		Kind:       "Snapshot",
		Metadata: SnapshotMetadata{
			Name: "test-snapshot",
		},
		Spec: SnapshotSpec{
			Application: "test-app",
			Components: []SnapshotComponent{
				{Name: "controller", ContainerImage: "quay.io/test/controller:v1.0.0"},
				{Name: "webhook", ContainerImage: "quay.io/test/webhook:v1.0.0"},
				{Name: "manager", ContainerImage: "quay.io/test/manager:v1.0.0"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generator.ValidateSnapshot(snapshot)
	}
}

// BenchmarkNormalizeKubernetesName benchmarks Kubernetes name normalization
func BenchmarkNormalizeKubernetesName(b *testing.B) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testNames := []string{
		"controller",
		"my_controller_service",
		"service.v1.0.0",
		"My_Service.V1@2023!",
		"-controller",
		"controller-",
		"my--double---hyphen",
		"very-long-component-name-that-exceeds-kubernetes-limit-of-sixty-three-characters-and-should-be-truncated",
		"MyControllerService",
		"用户/应用", // Unicode
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range testNames {
			_ = generator.normalizeKubernetesName(name)
		}
	}
}

// BenchmarkGenerateComponentName benchmarks component name generation
func BenchmarkGenerateComponentName(b *testing.B) {

	testRefs := []bundle.ImageReference{
		{Image: "quay.io/operator/controller:v1.0.0", Name: ""},
		{Image: "registry.redhat.io/ubi8/ubi:latest", Name: ""},
		{Image: "gcr.io/my-project/subproject/app:latest", Name: ""},
		{Image: "nginx:latest", Name: ""},
		{Image: "registry.example.com:5000/myapp/service:v2.1.0", Name: ""},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset generator for each iteration to ensure fresh state
		testGenerator := NewSnapshotGenerator("test-app", "test-namespace")
		for _, ref := range testRefs {
			_ = testGenerator.generateComponentName(ref)
		}
	}
}

// BenchmarkGenerateComponentNameCollisions benchmarks collision detection
func BenchmarkGenerateComponentNameCollisions(b *testing.B) {
	// Create many images that will have name collisions
	var testRefs []bundle.ImageReference
	for i := 0; i < 100; i++ {
		testRefs = append(testRefs, bundle.ImageReference{
			Image: fmt.Sprintf("registry%d.example.com/controller:v1.0.0", i),
			Name:  "",
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset generator state for each iteration
		testGenerator := NewSnapshotGenerator("test-app", "test-namespace")
		for _, ref := range testRefs {
			_ = testGenerator.generateComponentName(ref)
		}
	}
}

// BenchmarkCleanGitURL benchmarks Git URL cleaning
func BenchmarkCleanGitURL(b *testing.B) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testURLs := []string{
		"https://github.com/owner/repo",
		"github.com/owner/repo",
		"git@github.com:owner/repo.git",
		"git@github.enterprise.com:owner/repo.git",
		"git+https://github.com/owner/repo",
		"https://gitlab.com/group/project",
		"git@gitlab.com:group/project.git",
		"https://bitbucket.org/team/repository",
		"https://dev.azure.com/organization/project/_git/repository",
		"ssh://git@git.company.com:2222/team/project.git",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, url := range testURLs {
			_ = generator.cleanGitURL(url)
		}
	}
}

// BenchmarkCreateNameWithSuffix benchmarks collision suffix generation
func BenchmarkCreateNameWithSuffix(b *testing.B) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	baseName := "controller"
	imageRef := "quay.io/operator/controller:v1.0.0"
	collisionIndex := 1

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generator.createNameWithSuffix(baseName, imageRef, collisionIndex)
	}
}

// BenchmarkMemoryEfficiency benchmarks memory usage with large inputs
func BenchmarkMemoryEfficiency(b *testing.B) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	// Create many duplicate image references
	var imageRefs []bundle.ImageReference
	for i := 0; i < 1000; i++ {
		imageRefs = append(imageRefs, bundle.ImageReference{
			Image: "quay.io/test/duplicate:v1.0.0", // Same image
			Name:  fmt.Sprintf("duplicate%d", i),   // Different names
		})
	}

	// Add some unique images
	for i := 0; i < 100; i++ {
		imageRefs = append(imageRefs, bundle.ImageReference{
			Image: fmt.Sprintf("quay.io/test/unique%d:v1.0.0", i),
			Name:  fmt.Sprintf("unique%d", i),
		})
	}

	provenanceResults := make([]provenance.ProvenanceInfo, len(imageRefs))
	for i := range provenanceResults {
		provenanceResults[i] = provenance.ProvenanceInfo{
			Verified:     true,
			SourceRepo:   "https://github.com/test/repo",
			SourceCommit: "abc123",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generator.GenerateSnapshot(
			context.Background(),
			imageRefs,
			provenanceResults,
			"quay.io/test/bundle:v1.0.0",
			nil,
		)
	}
}

// BenchmarkLongImageNames benchmarks handling of extremely long image names
func BenchmarkLongImageNames(b *testing.B) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	longImageName := "quay.io/test/" + strings.Repeat("verylongname", 50) + ":v1.0.0"

	imageRefs := []bundle.ImageReference{
		{Image: longImageName, Name: ""},
	}

	provenanceResults := []provenance.ProvenanceInfo{
		{
			Verified:     true,
			SourceRepo:   "https://github.com/test/repo",
			SourceCommit: "abc123",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generator.GenerateSnapshot(
			context.Background(),
			imageRefs,
			provenanceResults,
			"quay.io/test/bundle:v1.0.0",
			nil,
		)
	}
}
