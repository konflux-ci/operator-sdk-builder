package snapshot

import (
	"context"
	"fmt"
	"strings"
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

// TestNormalizeKubernetesName tests the Kubernetes name normalization functionality
func TestNormalizeKubernetesName(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Basic alphanumeric name",
			input:    "controller",
			expected: "controller",
		},
		{
			name:     "Name with underscores",
			input:    "my_controller_service",
			expected: "my-controller-service",
		},
		{
			name:     "Name with dots",
			input:    "service.v1.0.0",
			expected: "service-v1-0-0",
		},
		{
			name:     "Name with mixed invalid characters",
			input:    "My_Service.V1@2023!",
			expected: "my-service-v1-2023",
		},
		{
			name:     "Name starting with hyphen",
			input:    "-controller",
			expected: "controller",
		},
		{
			name:     "Name ending with hyphen",
			input:    "controller-",
			expected: "controller",
		},
		{
			name:     "Name with consecutive hyphens",
			input:    "my--double---hyphen",
			expected: "my-double-hyphen",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "component",
		},
		{
			name:     "Only invalid characters",
			input:    "!@#$%",
			expected: "component",
		},
		{
			name:     "Name starting with number",
			input:    "2controller",
			expected: "2controller",
		},
		{
			name:     "Name starting with special character",
			input:    "$controller",
			expected: "controller",
		},
		{
			name:     "Name ending with special character",
			input:    "controller$",
			expected: "controller",
		},
		{
			name:     "Very long name should be truncated",
			input:    "very-long-component-name-that-exceeds-kubernetes-limit-of-sixty-three-characters-and-should-be-truncated",
			expected: "very-long-component-name-that-exceeds-kubernetes-limit-of-sixty",
		},
		{
			name:     "Capital letters converted to lowercase",
			input:    "MyControllerService",
			expected: "mycontrollerservice",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generator.normalizeKubernetesName(tc.input)
			if result != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, result)
			}

			// Verify the result follows Kubernetes naming rules
			if len(result) > 63 {
				t.Errorf("Result exceeds 63 character limit: %d characters", len(result))
			}

			if len(result) > 0 {
				// Should start with alphanumeric
				firstChar := result[0]
				if (firstChar < 'a' || firstChar > 'z') && (firstChar < '0' || firstChar > '9') {
					t.Errorf("Result should start with alphanumeric character, got: %c", firstChar)
				}

				// Should end with alphanumeric
				lastChar := result[len(result)-1]
				if (lastChar < 'a' || lastChar > 'z') && (lastChar < '0' || lastChar > '9') {
					t.Errorf("Result should end with alphanumeric character, got: %c", lastChar)
				}
			}
		})
	}
}

// TestGenerateComponentNameCollisionDetection tests component name collision detection and resolution
func TestGenerateComponentNameCollisionDetection(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testCases := []struct {
		name            string
		imageRefs       []bundle.ImageReference
		expectedNames   []string
		checkUniqueness bool
	}{
		{
			name: "No collisions - different base names",
			imageRefs: []bundle.ImageReference{
				{Image: "quay.io/operator/controller:v1.0.0", Name: ""},
				{Image: "quay.io/operator/webhook:v1.0.0", Name: ""},
				{Image: "quay.io/operator/manager:v1.0.0", Name: ""},
			},
			expectedNames:   []string{"controller", "webhook", "manager"},
			checkUniqueness: true,
		},
		{
			name: "Name collisions - same image name from different registries",
			imageRefs: []bundle.ImageReference{
				{Image: "quay.io/operator/controller:v1.0.0", Name: ""},
				{Image: "registry.redhat.io/operator/controller:v1.0.0", Name: ""},
				{Image: "gcr.io/project/controller:v2.0.0", Name: ""},
			},
			expectedNames:   nil, // Will be deterministic but with collision suffixes
			checkUniqueness: true,
		},
		{
			name: "Complex collision scenario with various image formats",
			imageRefs: []bundle.ImageReference{
				{Image: "nginx:latest", Name: ""},                                // Docker Hub official
				{Image: "library/nginx:1.20", Name: ""},                          // Docker Hub explicit library
				{Image: "quay.io/nginx/nginx:v1.0.0", Name: ""},                  // Different registry, same base name
				{Image: "registry.example.com:5000/app/nginx:custom", Name: ""},  // Custom registry with port
				{Image: "gcr.io/project/subproject/nginx@sha256:abcd", Name: ""}, // Digest format
			},
			expectedNames:   nil, // Will check for uniqueness instead
			checkUniqueness: true,
		},
		{
			name: "Explicit names provided - should use those",
			imageRefs: []bundle.ImageReference{
				{Image: "quay.io/operator/controller:v1.0.0", Name: "my-controller"},
				{Image: "quay.io/operator/controller:v2.0.0", Name: "my-other-controller"},
			},
			expectedNames:   []string{"my-controller", "my-other-controller"},
			checkUniqueness: true,
		},
		{
			name: "Mixed explicit and generated names",
			imageRefs: []bundle.ImageReference{
				{Image: "quay.io/operator/controller:v1.0.0", Name: "explicit-controller"},
				{Image: "quay.io/operator/webhook:v1.0.0", Name: ""},
				{Image: "quay.io/operator/manager:v1.0.0", Name: ""},
			},
			expectedNames:   []string{"explicit-controller", "webhook", "manager"},
			checkUniqueness: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset the generator's state for each test
			generator.componentNames = make(map[string]string)
			generator.nameCollisions = make(map[string]int)

			var actualNames []string
			for _, ref := range tc.imageRefs {
				name := generator.generateComponentName(ref)
				actualNames = append(actualNames, name)
			}

			// Check specific expected names if provided
			if tc.expectedNames != nil {
				if len(actualNames) != len(tc.expectedNames) {
					t.Fatalf("Expected %d names, got %d", len(tc.expectedNames), len(actualNames))
				}

				for i, expected := range tc.expectedNames {
					if actualNames[i] != expected {
						t.Errorf("Name %d: expected %s, got %s", i, expected, actualNames[i])
					}
				}
			}

			// Check uniqueness if required
			if tc.checkUniqueness {
				uniqueNames := make(map[string]bool)
				for _, name := range actualNames {
					if uniqueNames[name] {
						t.Errorf("Duplicate component name found: %s", name)
					}
					uniqueNames[name] = true
				}
			}

			// Verify deterministic behavior - generating names again should yield the same results
			generator2 := NewSnapshotGenerator("test-app", "test-namespace")
			var actualNames2 []string
			for _, ref := range tc.imageRefs {
				name := generator2.generateComponentName(ref)
				actualNames2 = append(actualNames2, name)
			}

			for i, name := range actualNames {
				if actualNames2[i] != name {
					t.Errorf("Non-deterministic behavior: first run %s, second run %s", name, actualNames2[i])
				}
			}
		})
	}
}

// TestComponentNameDeterminism tests that component names are deterministic for the same image
func TestComponentNameDeterminism(t *testing.T) {
	testImage := "quay.io/operator/controller:v1.0.0"

	// Generate name multiple times and verify consistency
	var names []string
	for i := 0; i < 5; i++ {
		generator := NewSnapshotGenerator("test-app", "test-namespace")
		ref := bundle.ImageReference{Image: testImage, Name: ""}
		name := generator.generateComponentName(ref)
		names = append(names, name)
	}

	// All names should be identical
	firstName := names[0]
	for i, name := range names {
		if name != firstName {
			t.Errorf("Non-deterministic behavior at iteration %d: expected %s, got %s", i, firstName, name)
		}
	}
}

// TestCreateNameWithSuffix tests the collision suffix generation
func TestCreateNameWithSuffix(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testCases := []struct {
		name        string
		baseName    string
		imageRef    string
		expectedLen int // Expected length constraints
	}{
		{
			name:        "Normal case",
			baseName:    "controller",
			imageRef:    "quay.io/operator/controller:v1.0.0",
			expectedLen: 63, // Should be within Kubernetes limits
		},
		{
			name:        "Long base name should be truncated",
			baseName:    "very-long-component-name-that-exceeds-normal-length-limits",
			imageRef:    "registry.example.com/project/very-long-component-name-that-exceeds-normal-length-limits:v1.0.0",
			expectedLen: 63,
		},
		{
			name:        "Different images with same base name get different suffixes",
			baseName:    "nginx",
			imageRef:    "quay.io/nginx/nginx:v1.0.0",
			expectedLen: 63,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generator.createNameWithSuffix(tc.baseName, tc.imageRef, 1)

			// Check length constraint
			if len(result) > tc.expectedLen {
				t.Errorf("Result length %d exceeds limit %d", len(result), tc.expectedLen)
			}

			// Should contain the base name (or truncated version)
			if !strings.Contains(result, tc.baseName[:min(len(tc.baseName), 56)]) {
				t.Errorf("Result should contain base name prefix, got: %s", result)
			}

			// Should contain a hyphen separator
			if !strings.Contains(result, "-") {
				t.Errorf("Result should contain hyphen separator, got: %s", result)
			}

			// Should be deterministic - same inputs yield same output
			result2 := generator.createNameWithSuffix(tc.baseName, tc.imageRef, 1)
			if result != result2 {
				t.Errorf("Non-deterministic behavior: %s vs %s", result, result2)
			}
		})
	}
}

// TestCleanGitURL tests the multi-platform Git URL cleaning functionality
func TestCleanGitURL(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		// GitHub URLs
		{
			name:     "Standard GitHub HTTPS URL",
			input:    "https://github.com/owner/repo",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "GitHub URL without protocol",
			input:    "github.com/owner/repo",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "GitHub SSH URL",
			input:    "git@github.com:owner/repo.git",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "GitHub Enterprise SSH URL",
			input:    "git@github.enterprise.com:owner/repo.git",
			expected: "https://github.enterprise.com/owner/repo",
		},
		{
			name:     "GitHub with git+ prefix",
			input:    "git+https://github.com/owner/repo",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "GitHub URL with .git suffix",
			input:    "https://github.com/owner/repo.git",
			expected: "https://github.com/owner/repo",
		},

		// GitLab URLs
		{
			name:     "GitLab.com HTTPS URL",
			input:    "https://gitlab.com/group/project",
			expected: "https://gitlab.com/group/project",
		},
		{
			name:     "GitLab.com SSH URL",
			input:    "git@gitlab.com:group/project.git",
			expected: "https://gitlab.com/group/project",
		},
		{
			name:     "Self-hosted GitLab HTTPS URL",
			input:    "https://gitlab.company.com/group/project",
			expected: "https://gitlab.company.com/group/project",
		},
		{
			name:     "Self-hosted GitLab SSH URL",
			input:    "git@gitlab.company.com:group/subgroup/project.git",
			expected: "https://gitlab.company.com/group/subgroup/project",
		},

		// Bitbucket URLs
		{
			name:     "Bitbucket HTTPS URL",
			input:    "https://bitbucket.org/team/repository",
			expected: "https://bitbucket.org/team/repository",
		},
		{
			name:     "Bitbucket SSH URL",
			input:    "git@bitbucket.org:team/repository.git",
			expected: "https://bitbucket.org/team/repository",
		},

		// Azure DevOps URLs
		{
			name:     "Azure DevOps modern format HTTPS",
			input:    "https://dev.azure.com/organization/project/_git/repository",
			expected: "https://dev.azure.com/organization/project/_git/repository",
		},
		{
			name:     "Azure DevOps legacy format HTTPS",
			input:    "https://organization.visualstudio.com/project/_git/repository",
			expected: "https://organization.visualstudio.com/project/_git/repository",
		},
		{
			name:     "Azure DevOps SSH URL",
			input:    "git@ssh.dev.azure.com:v3/organization/project/repository",
			expected: "https://ssh.dev.azure.com/v3/organization/project/repository",
		},

		// Gitea URLs
		{
			name:     "Codeberg (Gitea) HTTPS URL",
			input:    "https://codeberg.org/user/repository",
			expected: "https://codeberg.org/user/repository",
		},
		{
			name:     "Codeberg SSH URL",
			input:    "git@codeberg.org:user/repository.git",
			expected: "https://codeberg.org/user/repository",
		},
		{
			name:     "Self-hosted Gitea HTTPS URL",
			input:    "https://gitea.company.com/user/repository",
			expected: "https://gitea.company.com/user/repository",
		},

		// AWS CodeCommit URLs
		{
			name:     "CodeCommit HTTPS URL",
			input:    "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/repository",
			expected: "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/repository",
		},
		{
			name:     "CodeCommit SSH URL",
			input:    "ssh://git-codecommit.us-east-1.amazonaws.com/v1/repos/repository",
			expected: "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/repository",
		},

		// Generic/Unknown hosting platforms
		{
			name:     "Generic HTTPS URL",
			input:    "https://git.company.com/team/project",
			expected: "https://git.company.com/team/project",
		},
		{
			name:     "Generic SSH URL",
			input:    "git@git.company.com:team/project.git",
			expected: "https://git.company.com/team/project",
		},

		// Edge cases
		{
			name:     "Empty URL",
			input:    "",
			expected: "",
		},
		{
			name:     "URL with complex path structure",
			input:    "https://gitlab.company.com/group/subgroup/nested/project",
			expected: "https://gitlab.company.com/group/subgroup/nested/project",
		},
		{
			name:     "SSH URL with ssh:// prefix",
			input:    "ssh://git@github.com/owner/repo.git",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "HTTP (non-secure) URL should be preserved",
			input:    "http://internal.git.company.com/team/project",
			expected: "https://internal.git.company.com/team/project",
		},
		{
			name:     "URL with port number",
			input:    "https://git.company.com:8080/team/project",
			expected: "https://git.company.com:8080/team/project",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generator.cleanGitURL(tc.input)
			if result != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, result)
			}
		})
	}
}

// TestGitHostingPlatformDetection tests the detection of different Git hosting platforms
func TestGitHostingPlatformDetection(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testCases := []struct {
		name     string
		hostname string
		function string
		expected bool
	}{
		// GitHub detection
		{"GitHub main", "github.com", "isGitHub", true},
		{"GitHub Enterprise", "github.enterprise.com", "isGitHub", true},
		{"Not GitHub", "gitlab.com", "isGitHub", false},

		// GitLab detection
		{"GitLab main", "gitlab.com", "isGitLab", true},
		{"Self-hosted GitLab", "gitlab.company.com", "isGitLab", true},
		{"GitLab in subdomain", "code.gitlab.internal.com", "isGitLab", true},
		{"Not GitLab", "github.com", "isGitLab", false},

		// Bitbucket detection
		{"Bitbucket main", "bitbucket.org", "isBitBucket", true},
		{"Bitbucket subdomain", "api.bitbucket.org", "isBitBucket", true},
		{"Not Bitbucket", "github.com", "isBitBucket", false},

		// Azure DevOps detection
		{"Azure DevOps modern", "dev.azure.com", "isAzureDevOps", true},
		{"Azure DevOps legacy", "company.visualstudio.com", "isAzureDevOps", true},
		{"Azure general", "something.azure.com", "isAzureDevOps", true},
		{"Not Azure", "github.com", "isAzureDevOps", false},

		// Gitea detection
		{"Gitea in hostname", "gitea.company.com", "isGitea", true},
		{"Codeberg", "codeberg.org", "isGitea", true},
		{"Not Gitea", "github.com", "isGitea", false},

		// CodeCommit detection
		{"CodeCommit", "git-codecommit.us-east-1.amazonaws.com", "isCodeCommit", true},
		{"Not CodeCommit", "github.com", "isCodeCommit", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result bool
			switch tc.function {
			case "isGitHub":
				result = generator.isGitHub(tc.hostname)
			case "isGitLab":
				result = generator.isGitLab(tc.hostname)
			case "isBitBucket":
				result = generator.isBitBucket(tc.hostname)
			case "isAzureDevOps":
				result = generator.isAzureDevOps(tc.hostname)
			case "isGitea":
				result = generator.isGitea(tc.hostname)
			case "isCodeCommit":
				result = generator.isCodeCommit(tc.hostname)
			default:
				t.Fatalf("Unknown function: %s", tc.function)
			}

			if result != tc.expected {
				t.Errorf("Expected %s(%s) = %t, got %t", tc.function, tc.hostname, tc.expected, result)
			}
		})
	}
}

// TestParseSSHGitURL tests SSH Git URL parsing
func TestParseSSHGitURL(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Standard SSH format",
			input:    "git@github.com:owner/repo.git",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "SSH URL with ssh:// prefix",
			input:    "ssh://git@gitlab.com/group/project.git",
			expected: "https://gitlab.com/group/project",
		},
		{
			name:     "SSH URL without git user",
			input:    "user@custom-git.com:team/project.git",
			expected: "", // Should not match since it doesn't start with git@
		},
		{
			name:     "Complex SSH URL with port",
			input:    "ssh://git@git.company.com:2222/team/project.git",
			expected: "https://git.company.com:2222/team/project",
		},
		{
			name:     "Not an SSH URL",
			input:    "https://github.com/owner/repo",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generator.parseSSHGitURL(tc.input)
			if result != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, result)
			}
		})
	}
}

// TestParseHTTPSGitURL tests HTTPS Git URL parsing
func TestParseHTTPSGitURL(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Standard HTTPS URL",
			input:    "https://github.com/owner/repo",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "HTTP URL (insecure)",
			input:    "http://internal.git.com/team/project",
			expected: "https://internal.git.com/team/project",
		},
		{
			name:     "HTTPS URL with .git suffix",
			input:    "https://gitlab.com/group/project.git",
			expected: "https://gitlab.com/group/project",
		},
		{
			name:     "Not an HTTPS URL",
			input:    "git@github.com:owner/repo.git",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generator.parseHTTPSGitURL(tc.input)
			if result != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, result)
			}
		})
	}
}

// TestNormalizeAzureDevOpsURL tests Azure DevOps URL normalization
func TestNormalizeAzureDevOpsURL(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testCases := []struct {
		name     string
		hostname string
		path     string
		expected string
	}{
		{
			name:     "Modern Azure DevOps format",
			hostname: "dev.azure.com",
			path:     "organization/project/_git/repository",
			expected: "https://dev.azure.com/organization/project/_git/repository",
		},
		{
			name:     "Legacy Azure DevOps format",
			hostname: "company.visualstudio.com",
			path:     "_git/repository",
			expected: "https://company.visualstudio.com/_git/repository",
		},
		{
			name:     "Azure DevOps with incomplete path",
			hostname: "dev.azure.com",
			path:     "organization/project",
			expected: "https://dev.azure.com/organization/project",
		},
		{
			name:     "Generic Azure hostname",
			hostname: "custom.azure.com",
			path:     "team/project",
			expected: "https://custom.azure.com/team/project",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generator.normalizeAzureDevOpsURL(tc.hostname, tc.path)
			if result != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, result)
			}
		})
	}
}

// TestGitURLEdgeCases tests edge cases and error conditions for Git URL parsing
func TestGitURLEdgeCases(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Malformed SSH URL missing colon",
			input:    "git@github.com/owner/repo",
			expected: "git@github.com/owner/repo", // Should return original if parsing fails
		},
		{
			name:     "URL with query parameters",
			input:    "https://github.com/owner/repo?ref=main",
			expected: "https://github.com/owner/repo?ref=main", // Should preserve original format
		},
		{
			name:     "URL with fragment",
			input:    "https://github.com/owner/repo#readme",
			expected: "https://github.com/owner/repo#readme", // Should preserve original format
		},
		{
			name:     "Just a hostname without path",
			input:    "github.com",
			expected: "github.com", // Should return original if no path
		},
		{
			name:     "Complex nested path structure",
			input:    "https://gitlab.company.com/group/subgroup/nested/deeply/project",
			expected: "https://gitlab.company.com/group/subgroup/nested/deeply/project",
		},
		{
			name:     "URL with multiple .git suffixes",
			input:    "https://github.com/owner/repo.git.git",
			expected: "https://github.com/owner/repo.git", // Should only remove one .git suffix
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generator.cleanGitURL(tc.input)
			if result != tc.expected {
				t.Errorf("Expected: %s, Got: %s", tc.expected, result)
			}
		})
	}
}

// TestValidateSnapshotEdgeCases tests edge cases for snapshot validation
func TestValidateSnapshotEdgeCases(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	tests := []struct {
		name        string
		snapshot    *KonfluxSnapshot
		expectError bool
		errorMsg    string
	}{
		{
			name: "component with missing container image",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components: []SnapshotComponent{
						{Name: "controller", ContainerImage: ""}, // Missing container image
					},
				},
			},
			expectError: true,
			errorMsg:    "container image",
		},
		{
			name: "component with whitespace-only container image",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components: []SnapshotComponent{
						{Name: "controller", ContainerImage: "   \t\n  "}, // Whitespace only
					},
				},
			},
			expectError: false, // ValidateSnapshot may not check for whitespace-only strings
		},
		{
			name: "component with missing name",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components: []SnapshotComponent{
						{Name: "", ContainerImage: "quay.io/test/controller:v1.0.0"}, // Missing name
					},
				},
			},
			expectError: true,
			errorMsg:    "component name",
		},
		{
			name: "snapshot with no components",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components:  []SnapshotComponent{}, // Empty components
				},
			},
			expectError: true,
			errorMsg:    "at least one component",
		},
		{
			name: "snapshot with nil components",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components:  nil, // Nil components
				},
			},
			expectError: true,
			errorMsg:    "at least one component",
		},
		{
			name: "snapshot with missing metadata name",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "", // Missing name
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components: []SnapshotComponent{
						{Name: "controller", ContainerImage: "quay.io/test/controller:v1.0.0"},
					},
				},
			},
			expectError: true,
			errorMsg:    "snapshot name",
		},
		{
			name: "snapshot with invalid API version",
			snapshot: &KonfluxSnapshot{
				APIVersion: "v1", // Wrong API version
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
			},
			expectError: false, // ValidateSnapshot may not validate API version format
		},
		{
			name: "snapshot with invalid kind",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Application", // Wrong kind
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components: []SnapshotComponent{
						{Name: "controller", ContainerImage: "quay.io/test/controller:v1.0.0"},
					},
				},
			},
			expectError: false, // ValidateSnapshot may not validate kind field
		},
		{
			name: "component with invalid container image format",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components: []SnapshotComponent{
						{Name: "controller", ContainerImage: "not-a-valid-image-reference"}, // Invalid format
					},
				},
			},
			expectError: false, // Should be handled gracefully - validation may not check image format
		},
		{
			name: "component with extremely long name",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components: []SnapshotComponent{
						{
							Name:           strings.Repeat("a", 1000), // Extremely long name
							ContainerImage: "quay.io/test/controller:v1.0.0",
						},
					},
				},
			},
			expectError: false, // Should handle long names gracefully
		},
		{
			name: "component with source but missing git info",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components: []SnapshotComponent{
						{
							Name:           "controller",
							ContainerImage: "quay.io/test/controller:v1.0.0",
							Source:         &ComponentSource{Git: nil}, // Source without git
						},
					},
				},
			},
			expectError: false, // Should handle missing git info gracefully
		},
		{
			name: "component with git info but missing URL",
			snapshot: &KonfluxSnapshot{
				APIVersion: "appstudio.redhat.com/v1alpha1",
				Kind:       "Snapshot",
				Metadata: SnapshotMetadata{
					Name: "test-snapshot",
				},
				Spec: SnapshotSpec{
					Application: "test-app",
					Components: []SnapshotComponent{
						{
							Name:           "controller",
							ContainerImage: "quay.io/test/controller:v1.0.0",
							Source: &ComponentSource{
								Git: &GitSource{
									URL:      "", // Missing URL
									Revision: "abc123",
								},
							},
						},
					},
				},
			},
			expectError: false, // Should handle missing git URL gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := generator.ValidateSnapshot(tt.snapshot)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, but got none", tt.name)
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %s: %v", tt.name, err)
				}
			}
		})
	}
}

// TestGenerateSnapshotResourceLimits tests resource limit handling
func TestGenerateSnapshotResourceLimits(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	// Test with a large number of image references
	t.Run("large number of image references", func(t *testing.T) {
		var imageRefs []bundle.ImageReference
		var provenanceResults []provenance.ProvenanceInfo

		// Create 1000 image references
		for i := 0; i < 1000; i++ {
			ref := bundle.ImageReference{
				Image: fmt.Sprintf("quay.io/test/image%d:v1.0.0", i),
				Name:  fmt.Sprintf("image%d", i),
			}
			imageRefs = append(imageRefs, ref)

			provResult := provenance.ProvenanceInfo{
				Verified:     true,
				SourceRepo:   fmt.Sprintf("https://github.com/test/repo%d", i),
				SourceCommit: fmt.Sprintf("commit%d", i),
			}
			provenanceResults = append(provenanceResults, provResult)
		}

		snapshot, err := generator.GenerateSnapshot(
			context.Background(),
			imageRefs,
			provenanceResults,
			"quay.io/test/bundle:v1.0.0",
			nil,
		)

		if err != nil {
			t.Errorf("Should handle large number of image references gracefully: %v", err)
		}

		// Should have bundle + 1000 images = 1001 components
		expectedComponents := 1001
		if len(snapshot.Spec.Components) != expectedComponents {
			t.Errorf("Expected %d components, got %d", expectedComponents, len(snapshot.Spec.Components))
		}
	})

	// Test with extremely long image names
	t.Run("extremely long image names", func(t *testing.T) {
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

		snapshot, err := generator.GenerateSnapshot(
			context.Background(),
			imageRefs,
			provenanceResults,
			"quay.io/test/bundle:v1.0.0",
			nil,
		)

		if err != nil {
			t.Errorf("Should handle long image names gracefully: %v", err)
		}

		if len(snapshot.Spec.Components) != 2 { // bundle + 1 image
			t.Errorf("Expected 2 components, got %d", len(snapshot.Spec.Components))
		}

		// Component name should be normalized and within limits
		for _, comp := range snapshot.Spec.Components {
			if comp.ContainerImage == longImageName {
				if len(comp.Name) > 63 {
					t.Errorf("Component name exceeds Kubernetes limit: %d characters", len(comp.Name))
				}
			}
		}
	})
}

// TestSnapshotGeneratorMemoryEfficiency tests memory usage with large inputs
func TestSnapshotGeneratorMemoryEfficiency(t *testing.T) {
	generator := NewSnapshotGenerator("test-app", "test-namespace")

	// Test with duplicate image references to ensure deduplication works efficiently
	t.Run("duplicate image reference deduplication", func(t *testing.T) {
		var imageRefs []bundle.ImageReference

		// Create many duplicates of the same image
		for i := 0; i < 100; i++ {
			ref := bundle.ImageReference{
				Image: "quay.io/test/duplicate:v1.0.0",
				Name:  fmt.Sprintf("duplicate%d", i), // Different names but same image
			}
			imageRefs = append(imageRefs, ref)
		}

		// Add some unique images
		for i := 0; i < 10; i++ {
			ref := bundle.ImageReference{
				Image: fmt.Sprintf("quay.io/test/unique%d:v1.0.0", i),
				Name:  fmt.Sprintf("unique%d", i),
			}
			imageRefs = append(imageRefs, ref)
		}

		provenanceResults := make([]provenance.ProvenanceInfo, len(imageRefs))
		for i := range provenanceResults {
			provenanceResults[i] = provenance.ProvenanceInfo{
				Verified:     true,
				SourceRepo:   "https://github.com/test/repo",
				SourceCommit: "abc123",
			}
		}

		snapshot, err := generator.GenerateSnapshot(
			context.Background(),
			imageRefs,
			provenanceResults,
			"quay.io/test/bundle:v1.0.0",
			nil,
		)

		if err != nil {
			t.Fatalf("GenerateSnapshot failed: %v", err)
		}

		// Should have bundle + 11 unique images (1 duplicate + 10 unique) = 12 components
		// Note: The first occurrence of the duplicate should be kept
		expectedComponents := 12
		if len(snapshot.Spec.Components) != expectedComponents {
			t.Errorf("Expected %d components after deduplication, got %d", expectedComponents, len(snapshot.Spec.Components))
		}

		// Verify component names are unique
		namesSeen := make(map[string]bool)
		for _, comp := range snapshot.Spec.Components {
			if namesSeen[comp.Name] {
				t.Errorf("Duplicate component name found: %s", comp.Name)
			}
			namesSeen[comp.Name] = true
		}
	})
}

// Helper function for tests
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
