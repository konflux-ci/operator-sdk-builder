package resolver

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
)

// TestImageResolverWithDigestImages tests resolution of digest-pinned images
func TestImageResolverWithDigestImages(t *testing.T) {
	tests := []struct {
		name         string
		imageRef     string
		expected     string
		shouldChange bool
	}{
		{
			name:         "digest image gets mirrored",
			imageRef:     "quay.io/upstream/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			expected:     "quay.io/mirrored/upstream/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			shouldChange: true,
		},
		{
			name:         "digest image with nested path gets mirrored",
			imageRef:     "registry.upstream.io/rhel8/postgresql-13@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			expected:     "registry.mirrored.io/rhel8/postgresql-13@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			shouldChange: true,
		},
		{
			name:         "digest image no mirror match",
			imageRef:     "gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
			expected:     "gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
			shouldChange: false,
		},
		{
			name:         "tag image gets mirrored",
			imageRef:     "quay.io/upstream/operator:v1.0.0",
			expected:     "quay.io/mirrored/upstream/operator:v1.0.0",
			shouldChange: true,
		},
		{
			name:         "tag and digest combined image gets mirrored",
			imageRef:     "quay.io/upstream/operator:v1.0.0@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			expected:     "quay.io/mirrored/upstream/operator:v1.0.0@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			shouldChange: true,
		},
	}

	mirrorPolicyContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: digest-mirror-test
spec:
  imageDigestMirrors:
  - source: quay.io/upstream
    mirrors:
    - quay.io/mirrored/upstream
  - source: registry.upstream.io
    mirrors:
    - registry.mirrored.io
`

	tmpFile := createTempMirrorPolicy(t, mirrorPolicyContent)
	defer removeTempFile(t, tmpFile)

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(tmpFile)
	if err != nil {
		t.Fatalf("failed to load mirror policy: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create ImageReference and resolve it
			imageRefs := []bundle.ImageReference{
				{Image: tt.imageRef, Name: "test"},
			}
			
			resolved, err := resolver.ResolveImageReferences(imageRefs)
			if err != nil {
				t.Fatalf("ResolveImageReferences failed: %v", err)
			}

			if len(resolved) != 1 {
				t.Fatalf("Expected 1 resolved image, got %d", len(resolved))
			}

			if resolved[0].Image != tt.expected {
				t.Errorf("ResolveImageReferences(%q) = %q, want %q", tt.imageRef, resolved[0].Image, tt.expected)
			}

			// Check if image was changed by comparing with original
			changed := resolved[0].Image != tt.imageRef
			if changed != tt.shouldChange {
				t.Errorf("ResolveImageReferences(%q) changed = %v, want %v", tt.imageRef, changed, tt.shouldChange)
			}
		})
	}
}

// TestDigestImageMirroringComplexScenarios tests complex digest mirroring scenarios
func TestDigestImageMirroringComplexScenarios(t *testing.T) {
	tests := []struct {
		name       string
		imageRef   string
		expected   string
		changed    bool
	}{
		{
			name:     "multi-level registry with digest",
			imageRef: "registry.redhat.io/ubi8/ubi-minimal@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3",
			expected: "registry.disconnected.redhat.io/ubi8/ubi-minimal@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3",
			changed:  true,
		},
		{
			name:     "nested namespace with digest",
			imageRef: "quay.io/certified/operators/complex-operator@sha256:e5f6789012345678901234567890123456789012345678901234567890a1b2c3d4",
			expected: "mirror.registry.io/certified/operators/complex-operator@sha256:e5f6789012345678901234567890123456789012345678901234567890a1b2c3d4",
			changed:  true,
		},
		{
			name:     "port-based registry with digest",
			imageRef: "localhost:5000/test/operator@sha256:f6789012345678901234567890123456789012345678901234567890a1b2c3d4e5",
			expected: "internal-registry:5000/test/operator@sha256:f6789012345678901234567890123456789012345678901234567890a1b2c3d4e5",
			changed:  true,
		},
		{
			name:     "exact registry match with digest",
			imageRef: "registry.access.redhat.com/ubi8/ubi@sha256:1234567890123456789012345678901234567890123456789012345678901234",
			expected: "disconnected.registry.com/ubi8/ubi@sha256:1234567890123456789012345678901234567890123456789012345678901234",
			changed:  true,
		},
		{
			name:     "no match for private registry digest",
			imageRef: "private.company.com/internal/app@sha256:abcdef1234567890123456789012345678901234567890123456789012345678",
			expected: "private.company.com/internal/app@sha256:abcdef1234567890123456789012345678901234567890123456789012345678",
			changed:  false,
		},
	}

	mirrorPolicyContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: complex-digest-mirror-test
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - registry.disconnected.redhat.io
  - source: quay.io/certified
    mirrors:
    - mirror.registry.io/certified
  - source: localhost:5000
    mirrors:
    - internal-registry:5000
  - source: registry.access.redhat.com
    mirrors:
    - disconnected.registry.com
`

	tmpFile := createTempMirrorPolicy(t, mirrorPolicyContent)
	defer removeTempFile(t, tmpFile)

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(tmpFile)
	if err != nil {
		t.Fatalf("failed to load mirror policy: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create ImageReference and resolve it
			imageRefs := []bundle.ImageReference{
				{Image: tt.imageRef, Name: "test"},
			}
			
			resolved, err := resolver.ResolveImageReferences(imageRefs)
			if err != nil {
				t.Fatalf("ResolveImageReferences failed: %v", err)
			}

			if len(resolved) != 1 {
				t.Fatalf("Expected 1 resolved image, got %d", len(resolved))
			}

			if resolved[0].Image != tt.expected {
				t.Errorf("ResolveImageReferences(%q) = %q, want %q", tt.imageRef, resolved[0].Image, tt.expected)
			}

			// Check if image was changed by comparing with original
			changed := resolved[0].Image != tt.imageRef
			if changed != tt.changed {
				t.Errorf("ResolveImageReferences(%q) changed = %v, want %v", tt.imageRef, changed, tt.changed)
			}
		})
	}
}

// TestBatchDigestImageResolution tests resolving multiple digest images at once
func TestBatchDigestImageResolution(t *testing.T) {
	imageRefs := []string{
		"quay.io/upstream/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
		"quay.io/upstream/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
		"registry.redhat.io/ubi8/ubi@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
		"gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3",
		"quay.io/upstream/proxy:v1.0.0", // tag for comparison
	}

	expectedResults := []string{
		"quay.io/mirrored/upstream/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
		"quay.io/mirrored/upstream/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
		"registry.mirrored.redhat.io/ubi8/ubi@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
		"gcr.io/kubebuilder/kube-rbac-proxy@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3", // no change
		"quay.io/mirrored/upstream/proxy:v1.0.0",
	}

	expectedChanges := []bool{true, true, true, false, true}

	mirrorPolicyContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: batch-digest-test
spec:
  imageDigestMirrors:
  - source: quay.io/upstream
    mirrors:
    - quay.io/mirrored/upstream
  - source: registry.redhat.io
    mirrors:
    - registry.mirrored.redhat.io
`

	tmpFile := createTempMirrorPolicy(t, mirrorPolicyContent)
	defer removeTempFile(t, tmpFile)

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(tmpFile)
	if err != nil {
		t.Fatalf("failed to load mirror policy: %v", err)
	}

	// Convert string array to ImageReference array
	inputRefs := make([]bundle.ImageReference, len(imageRefs))
	for i, imageRef := range imageRefs {
		inputRefs[i] = bundle.ImageReference{Image: imageRef, Name: "test"}
	}
	
	resolved, err := resolver.ResolveImageReferences(inputRefs)
	if err != nil {
		t.Fatalf("ResolveImageReferences failed: %v", err)
	}

	if len(resolved) != len(expectedResults) {
		t.Fatalf("expected %d resolved images, got %d", len(expectedResults), len(resolved))
	}

	for i, result := range resolved {
		if result.Image != expectedResults[i] {
			t.Errorf("resolved[%d].Image = %q, want %q", i, result.Image, expectedResults[i])
		}

		// Check if image was changed by comparing with original
		changed := result.Image != imageRefs[i]
		if changed != expectedChanges[i] {
			t.Errorf("resolved[%d] changed = %v, want %v", i, changed, expectedChanges[i])
		}
	}
}

// TestDigestImageICSDResolution tests digest image resolution with ICSP policies
func TestDigestImageICSPResolution(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		expected string
		changed  bool
	}{
		{
			name:     "ICSP digest image mirroring",
			imageRef: "registry.redhat.io/rhel8/postgresql@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			expected: "mirror.registry.com/rhel8/postgresql@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			changed:  true,
		},
		{
			name:     "ICSP nested path digest mirroring",
			imageRef: "quay.io/openshift/origin-node@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			expected: "internal.registry.io/openshift/origin-node@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			changed:  true,
		},
	}

	icspContent := `apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: digest-icsp-test
spec:
  repositoryDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - mirror.registry.com
  - source: quay.io/openshift
    mirrors:
    - internal.registry.io/openshift
`

	tmpFile, err := ioutil.TempFile("", "icsp-digest-test-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write([]byte(icspContent))
	if err != nil {
		t.Fatalf("failed to write ICSP content: %v", err)
	}
	tmpFile.Close()

	resolver := NewImageResolver()
	err = resolver.LoadMirrorPolicy(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to load ICSP policy: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create ImageReference and resolve it
			imageRefs := []bundle.ImageReference{
				{Image: tt.imageRef, Name: "test"},
			}
			
			resolved, err := resolver.ResolveImageReferences(imageRefs)
			if err != nil {
				t.Fatalf("ResolveImageReferences failed: %v", err)
			}

			if len(resolved) != 1 {
				t.Fatalf("Expected 1 resolved image, got %d", len(resolved))
			}

			if resolved[0].Image != tt.expected {
				t.Errorf("ResolveImageReferences(%q) = %q, want %q", tt.imageRef, resolved[0].Image, tt.expected)
			}

			// Check if image was changed by comparing with original
			changed := resolved[0].Image != tt.imageRef
			if changed != tt.changed {
				t.Errorf("ResolveImageReferences(%q) changed = %v, want %v", tt.imageRef, changed, tt.changed)
			}
		})
	}
}

// TestMixedTagAndDigestResolution tests resolution of mixed tag and digest images
func TestMixedTagAndDigestResolution(t *testing.T) {
	imageRefs := []string{
		"quay.io/test/operator:v1.0.0",                                                                           // tag
		"quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",        // digest
		"quay.io/test/webhook:latest",                                                                            // tag
		"quay.io/test/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",         // digest
		"quay.io/test/proxy:v2.0.0@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",     // tag+digest
		"registry.example.com/app@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3",       // digest, no mirror
	}

	expectedResults := []string{
		"mirror.registry.io/test/operator:v1.0.0",
		"mirror.registry.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
		"mirror.registry.io/test/webhook:latest",
		"mirror.registry.io/test/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
		"mirror.registry.io/test/proxy:v2.0.0@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
		"registry.example.com/app@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3", // no change
	}

	expectedChanges := []bool{true, true, true, true, true, false}

	mirrorPolicyContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: mixed-resolution-test
spec:
  imageDigestMirrors:
  - source: quay.io/test
    mirrors:
    - mirror.registry.io/test
`

	tmpFile := createTempMirrorPolicy(t, mirrorPolicyContent)
	defer removeTempFile(t, tmpFile)

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(tmpFile)
	if err != nil {
		t.Fatalf("failed to load mirror policy: %v", err)
	}

	for i, imageRef := range imageRefs {
		t.Run(imageRef, func(t *testing.T) {
			// Create ImageReference and resolve it
			inputRefs := []bundle.ImageReference{
				{Image: imageRef, Name: "test"},
			}
			
			resolved, err := resolver.ResolveImageReferences(inputRefs)
			if err != nil {
				t.Fatalf("ResolveImageReferences failed: %v", err)
			}

			if len(resolved) != 1 {
				t.Fatalf("Expected 1 resolved image, got %d", len(resolved))
			}

			if resolved[0].Image != expectedResults[i] {
				t.Errorf("ResolveImageReferences(%q) = %q, want %q", imageRef, resolved[0].Image, expectedResults[i])
			}

			// Check if image was changed by comparing with original
			changed := resolved[0].Image != imageRef
			if changed != expectedChanges[i] {
				t.Errorf("ResolveImageReferences(%q) changed = %v, want %v", imageRef, changed, expectedChanges[i])
			}
		})
	}
}

// TestDigestImageEdgeCases tests edge cases specific to digest images
func TestDigestImageEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		imageRef    string
		expected    string
		changed     bool
		description string
	}{
		{
			name:        "digest with @ in tag",
			imageRef:    "registry.com/test:tag@with@symbols@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			expected:    "mirror.com/test:tag@with@symbols@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			changed:     true,
			description: "handles multiple @ symbols correctly",
		},
		{
			name:        "very long digest",
			imageRef:    "registry.com/test@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890abcdef",
			expected:    "mirror.com/test@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890abcdef",
			changed:     true,
			description: "handles longer than normal digests",
		},
		{
			name:        "digest with port number",
			imageRef:    "unmatched.registry.com:8080/test@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			expected:    "unmatched.registry.com:8080/test@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
			changed:     false,
			description: "port numbers don't interfere with digest parsing",
		},
		{
			name:        "empty digest after @",
			imageRef:    "unmatched.registry.com/test@",
			expected:    "unmatched.registry.com/test@",
			changed:     false,
			description: "handles malformed digest gracefully",
		},
	}

	mirrorPolicyContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: digest-edge-case-test
spec:
  imageDigestMirrors:
  - source: registry.com
    mirrors:
    - mirror.com
`

	tmpFile := createTempMirrorPolicy(t, mirrorPolicyContent)
	defer removeTempFile(t, tmpFile)

	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(tmpFile)
	if err != nil {
		t.Fatalf("failed to load mirror policy: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create ImageReference and resolve it
			imageRefs := []bundle.ImageReference{
				{Image: tt.imageRef, Name: "test"},
			}
			
			resolved, err := resolver.ResolveImageReferences(imageRefs)
			if err != nil {
				t.Fatalf("ResolveImageReferences failed: %v", err)
			}

			if len(resolved) != 1 {
				t.Fatalf("Expected 1 resolved image, got %d", len(resolved))
			}

			if resolved[0].Image != tt.expected {
				t.Errorf("ResolveImageReferences(%q) = %q, want %q (%s)", tt.imageRef, resolved[0].Image, tt.expected, tt.description)
			}

			// Check if image was changed by comparing with original
			changed := resolved[0].Image != tt.imageRef
			if changed != tt.changed {
				t.Errorf("ResolveImageReferences(%q) changed = %v, want %v (%s)", tt.imageRef, changed, tt.changed, tt.description)
			}
		})
	}
}