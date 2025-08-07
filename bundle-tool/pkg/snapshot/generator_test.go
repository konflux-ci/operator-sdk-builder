package snapshot

import (
	"context"
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/provenance"
)

func TestNewSnapshotGenerator(t *testing.T) {
	appName := "test-app"
	namespace := "test-namespace"
	
	generator := NewSnapshotGenerator(appName, namespace)
	
	if generator == nil {
		t.Fatal("NewSnapshotGenerator returned nil")
	}
	
	if generator.appName != appName {
		t.Errorf("Expected appName %q, got %q", appName, generator.appName)
	}
	
	if generator.namespace != namespace {
		t.Errorf("Expected namespace %q, got %q", namespace, generator.namespace)
	}
}

func TestGenerateSnapshot(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")
	
	imageRefs := []bundle.ImageReference{
		{Image: "quay.io/test/controller:v1.0.0", Name: "controller"},
		{Image: "quay.io/test/webhook:v1.0.0", Name: "webhook"},
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
	}
	
	snapshot, err := generator.GenerateSnapshot(context.Background(), imageRefs, provenanceResults, "quay.io/test/bundle:v1.0.0", nil)
	if err != nil {
		t.Fatalf("GenerateSnapshot failed: %v", err)
	}
	
	// Check basic structure
	if snapshot.APIVersion != "appstudio.redhat.com/v1alpha1" {
		t.Errorf("Expected APIVersion 'appstudio.redhat.com/v1alpha1', got %q", snapshot.APIVersion)
	}
	
	if snapshot.Kind != "Snapshot" {
		t.Errorf("Expected Kind 'Snapshot', got %q", snapshot.Kind)
	}
	
	if snapshot.Spec.Application != "test-app" {
		t.Errorf("Expected Application 'test-app', got %q", snapshot.Spec.Application)
	}
	
	// Check components - should have bundle + 2 related images = 3 total
	if len(snapshot.Spec.Components) != 3 {
		t.Errorf("Expected 3 components (bundle + 2 related images), got %d", len(snapshot.Spec.Components))
	}
	
	// Check first component (bundle)
	bundleComp := snapshot.Spec.Components[0]
	if bundleComp.Name != "bundle" {
		t.Errorf("Expected bundle component name 'bundle', got %q", bundleComp.Name)
	}
	
	if bundleComp.ContainerImage != "quay.io/test/bundle:v1.0.0" {
		t.Errorf("Expected bundle container image 'quay.io/test/bundle:v1.0.0', got %q", bundleComp.ContainerImage)
	}
	
	// Check second component (first related image)
	comp1 := snapshot.Spec.Components[1]
	if comp1.Name != "controller" {
		t.Errorf("Expected component name 'controller', got %q", comp1.Name)
	}
	
	if comp1.ContainerImage != "quay.io/test/controller:v1.0.0" {
		t.Errorf("Expected container image 'quay.io/test/controller:v1.0.0', got %q", comp1.ContainerImage)
	}
	
	if comp1.Source == nil || comp1.Source.Git == nil {
		t.Error("Expected component to have source information")
		return
	}
	
	if comp1.Source.Git.URL != "https://github.com/test/controller" {
		t.Errorf("Expected git URL 'https://github.com/test/controller', got %q", comp1.Source.Git.URL)
	}
	
	if comp1.Source.Git.Revision != "abc123def456" {
		t.Errorf("Expected git revision 'abc123def456', got %q", comp1.Source.Git.Revision)
	}
}

func TestDeduplicateComponents(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")
	
	snapshot := &KonfluxSnapshot{
		Spec: SnapshotSpec{
			Components: []SnapshotComponent{
				{Name: "controller", ContainerImage: "quay.io/test/controller:v1.0.0"},
				{Name: "webhook", ContainerImage: "quay.io/test/webhook:v1.0.0"},
				{Name: "controller", ContainerImage: "quay.io/test/controller:v1.0.0"}, // duplicate
				{Name: "sidecar", ContainerImage: "quay.io/test/sidecar:v1.0.0"},
			},
		},
	}
	
	generator.DeduplicateComponents(snapshot)
	
	// Should have 3 unique components
	if len(snapshot.Spec.Components) != 3 {
		t.Errorf("Expected 3 unique components, got %d", len(snapshot.Spec.Components))
	}
	
	// Check component names
	expectedNames := []string{"controller", "webhook", "sidecar"}
	for i, expected := range expectedNames {
		if snapshot.Spec.Components[i].Name != expected {
			t.Errorf("Expected component %d name %q, got %q", i, expected, snapshot.Spec.Components[i].Name)
		}
	}
}

func TestValidateSnapshot(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")
	
	// Valid snapshot
	validSnapshot := &KonfluxSnapshot{
		APIVersion: "appstudio.redhat.com/v1alpha1",
		Kind:       "Snapshot",
		Metadata: SnapshotMetadata{
			Name: "test-snapshot",
		},
		Spec: SnapshotSpec{
			Application: "test-app",
			Components: []SnapshotComponent{
				{Name: "controller", ContainerImage: "quay.io/test/controller:v1.0.0"},
			},
		},
	}
	
	err := generator.ValidateSnapshot(validSnapshot)
	if err != nil {
		t.Errorf("Valid snapshot should not fail validation: %v", err)
	}
	
	// Invalid snapshot - missing application
	invalidSnapshot := &KonfluxSnapshot{
		APIVersion: "appstudio.redhat.com/v1alpha1",
		Kind:       "Snapshot",
		Spec: SnapshotSpec{
			Components: []SnapshotComponent{
				{Name: "controller", ContainerImage: "quay.io/test/controller:v1.0.0"},
			},
		},
	}
	
	err = generator.ValidateSnapshot(invalidSnapshot)
	if err == nil {
		t.Error("Invalid snapshot (missing application) should fail validation")
	}
}

func TestGenerateSnapshotSkipsImagesWithoutProvenance(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")
	
	imageRefs := []bundle.ImageReference{
		{Image: "quay.io/test/controller:v1.0.0", Name: "controller"},
		{Image: "quay.io/test/webhook:v1.0.0", Name: "webhook"},
		{Image: "quay.io/test/kube-rbac-proxy:v1.0.0", Name: "proxy"},
	}
	
	// Only provide provenance for first two images, third should be skipped
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
			Verified:     false, // No valid provenance for proxy
			SourceRepo:   "",
			SourceCommit: "",
		},
	}
	
	snapshot, err := generator.GenerateSnapshot(context.Background(), imageRefs, provenanceResults, "quay.io/test/bundle:v1.0.0", nil)
	if err != nil {
		t.Fatalf("GenerateSnapshot failed: %v", err)
	}
	
	// Should have 3 components: bundle + 2 images with provenance (proxy skipped)
	expectedComponents := 3
	if len(snapshot.Spec.Components) != expectedComponents {
		t.Errorf("Expected %d components, got %d", expectedComponents, len(snapshot.Spec.Components))
	}
	
	// Verify that proxy image is not included
	for _, comp := range snapshot.Spec.Components {
		if comp.ContainerImage == "quay.io/test/kube-rbac-proxy:v1.0.0" {
			t.Error("Expected proxy image to be skipped due to missing provenance")
		}
	}
	
	// Verify the included images have source information
	nonBundleComponents := 0
	for _, comp := range snapshot.Spec.Components {
		if comp.ContainerImage != "quay.io/test/bundle:v1.0.0" {
			nonBundleComponents++
			if comp.Source == nil || comp.Source.Git == nil {
				t.Errorf("Component %s should have source information", comp.ContainerImage)
			}
		}
	}
	
	if nonBundleComponents != 2 {
		t.Errorf("Expected 2 non-bundle components with provenance, got %d", nonBundleComponents)
	}
}

func TestGenerateSnapshotWithMissingProvenanceResults(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")
	
	imageRefs := []bundle.ImageReference{
		{Image: "quay.io/test/controller:v1.0.0", Name: "controller"},
		{Image: "quay.io/test/webhook:v1.0.0", Name: "webhook"},
		{Image: "quay.io/test/proxy:v1.0.0", Name: "proxy"},
	}
	
	// Only provide provenance for first image, others have no provenance results
	provenanceResults := []provenance.ProvenanceInfo{
		{
			Verified:     true,
			SourceRepo:   "https://github.com/test/controller",
			SourceCommit: "abc123def456",
		},
		// Missing provenance results for webhook and proxy
	}
	
	snapshot, err := generator.GenerateSnapshot(context.Background(), imageRefs, provenanceResults, "quay.io/test/bundle:v1.0.0", nil)
	if err != nil {
		t.Fatalf("GenerateSnapshot failed: %v", err)
	}
	
	// Should have 2 components: bundle + 1 image with provenance (webhook and proxy skipped)
	expectedComponents := 2
	if len(snapshot.Spec.Components) != expectedComponents {
		t.Errorf("Expected %d components, got %d", expectedComponents, len(snapshot.Spec.Components))
	}
	
	// Verify only controller is included (besides bundle)
	foundController := false
	for _, comp := range snapshot.Spec.Components {
		if comp.ContainerImage == "quay.io/test/controller:v1.0.0" {
			foundController = true
			if comp.Source == nil || comp.Source.Git == nil {
				t.Error("Controller component should have source information")
			}
		}
		if comp.ContainerImage == "quay.io/test/webhook:v1.0.0" || comp.ContainerImage == "quay.io/test/proxy:v1.0.0" {
			t.Errorf("Expected %s to be skipped due to missing provenance", comp.ContainerImage)
		}
	}
	
	if !foundController {
		t.Error("Expected controller component to be included")
	}
}

func TestGenerateSnapshotWithBundleSourceFallback(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")
	
	imageRefs := []bundle.ImageReference{
		{Image: "quay.io/test/controller:v1.0.0", Name: "controller"},
	}
	
	provenanceResults := []provenance.ProvenanceInfo{
		{
			Verified:     true,
			SourceRepo:   "https://github.com/test/controller",
			SourceCommit: "abc123def456",
		},
	}
	
	// Test with fallback bundle source info
	bundleSourceRepo := "https://github.com/test/bundle-repo"
	bundleSourceCommit := "bundle123abc456"
	
	snapshot, err := generator.GenerateSnapshotWithBundleSource(
		context.Background(), 
		imageRefs, 
		provenanceResults, 
		"quay.io/test/bundle:v1.0.0", 
		bundleSourceRepo,
		bundleSourceCommit,
		nil, // No provenance parser (simulates no bundle provenance)
	)
	if err != nil {
		t.Fatalf("GenerateSnapshotWithBundleSource failed: %v", err)
	}
	
	// Should have 2 components: bundle (with fallback source) + controller
	expectedComponents := 2
	if len(snapshot.Spec.Components) != expectedComponents {
		t.Errorf("Expected %d components, got %d", expectedComponents, len(snapshot.Spec.Components))
	}
	
	// Find and verify bundle component
	var bundleComp *SnapshotComponent
	for i := range snapshot.Spec.Components {
		if snapshot.Spec.Components[i].ContainerImage == "quay.io/test/bundle:v1.0.0" {
			bundleComp = &snapshot.Spec.Components[i]
			break
		}
	}
	
	if bundleComp == nil {
		t.Fatal("Bundle component not found")
	}
	
	// Verify bundle has fallback source information
	if bundleComp.Source == nil || bundleComp.Source.Git == nil {
		t.Error("Bundle component should have source information from fallback")
		return
	}
	
	if bundleComp.Source.Git.URL != bundleSourceRepo {
		t.Errorf("Expected bundle source URL %q, got %q", bundleSourceRepo, bundleComp.Source.Git.URL)
	}
	
	if bundleComp.Source.Git.Revision != bundleSourceCommit {
		t.Errorf("Expected bundle source commit %q, got %q", bundleSourceCommit, bundleComp.Source.Git.Revision)
	}
}

func TestNewSnapshotGeneratorWithSourceFallback(t *testing.T) {
	// Test with fallback application name (no provenance)
	generator, err := NewSnapshotGeneratorWithSourceFallback(
		context.Background(),
		"quay.io/test/bundle:v1.0.0",
		"test-namespace",
		"fallback-app",
		"fallback-namespace",
		nil, // No provenance parser
	)
	
	if err != nil {
		t.Fatalf("NewSnapshotGeneratorWithSourceFallback failed: %v", err)
	}
	
	if generator == nil {
		t.Fatal("Expected non-nil generator")
	}
	
	// Test failure when no fallback app name provided
	_, err = NewSnapshotGeneratorWithSourceFallback(
		context.Background(),
		"quay.io/test/bundle:v1.0.0",
		"test-namespace",
		"", // No fallback app name
		"",
		nil, // No provenance parser
	)
	
	if err == nil {
		t.Error("Expected error when no application name available")
	}
}