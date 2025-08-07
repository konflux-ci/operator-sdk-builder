package resolver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
)

func TestLoadMirrorPolicy_IDMS(t *testing.T) {
	// Create temporary IDMS file
	tmpDir := t.TempDir()
	idmsFile := filepath.Join(tmpDir, "idms.yaml")
	
	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: test-idms
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads
`
	
	if err := os.WriteFile(idmsFile, []byte(idmsContent), 0644); err != nil {
		t.Fatalf("Failed to write IDMS file: %v", err)
	}
	
	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(idmsFile)
	
	if err != nil {
		t.Fatalf("LoadMirrorPolicy failed: %v", err)
	}
	
	if len(resolver.idmsPolicies) != 1 {
		t.Errorf("Expected 1 IDMS policy, got %d", len(resolver.idmsPolicies))
	}
	
	if len(resolver.icspPolicies) != 0 {
		t.Errorf("Expected 0 ICSP policies, got %d", len(resolver.icspPolicies))
	}
}

func TestLoadMirrorPolicy_ICSP(t *testing.T) {
	// Create temporary ICSP file
	tmpDir := t.TempDir()
	icspFile := filepath.Join(tmpDir, "icsp.yaml")
	
	icspContent := `apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: test-icsp
spec:
  repositoryDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads
`
	
	if err := os.WriteFile(icspFile, []byte(icspContent), 0644); err != nil {
		t.Fatalf("Failed to write ICSP file: %v", err)
	}
	
	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(icspFile)
	
	if err != nil {
		t.Fatalf("LoadMirrorPolicy failed: %v", err)
	}
	
	if len(resolver.icspPolicies) != 1 {
		t.Errorf("Expected 1 ICSP policy, got %d", len(resolver.icspPolicies))
	}
	
	if len(resolver.idmsPolicies) != 0 {
		t.Errorf("Expected 0 IDMS policies, got %d", len(resolver.idmsPolicies))
	}
}

func TestLoadMirrorPolicy_InvalidKind(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid.yaml")
	
	invalidContent := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
data:
  key: value
`
	
	if err := os.WriteFile(invalidFile, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to write invalid file: %v", err)
	}
	
	resolver := NewImageResolver()
	err := resolver.LoadMirrorPolicy(invalidFile)
	
	if err == nil {
		t.Fatal("Expected error for invalid kind, got nil")
	}
	
	if !contains(err.Error(), "unsupported mirror policy kind") {
		t.Errorf("Expected 'unsupported mirror policy kind' error, got: %v", err)
	}
}

func TestResolveImageReferences(t *testing.T) {
	tmpDir := t.TempDir()
	idmsFile := filepath.Join(tmpDir, "idms.yaml")
	
	idmsContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: test-idms
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads
`
	
	if err := os.WriteFile(idmsFile, []byte(idmsContent), 0644); err != nil {
		t.Fatalf("Failed to write IDMS file: %v", err)
	}
	
	resolver := NewImageResolver()
	if err := resolver.LoadMirrorPolicy(idmsFile); err != nil {
		t.Fatalf("LoadMirrorPolicy failed: %v", err)
	}
	
	// Test image resolution
	imageRefs := []bundle.ImageReference{
		{Image: "registry.redhat.io/operator/controller:v1.0.0", Name: "controller"},
		{Image: "quay.io/other/image:latest", Name: "other"},
	}
	
	resolved, err := resolver.ResolveImageReferences(imageRefs)
	if err != nil {
		t.Fatalf("ResolveImageReferences failed: %v", err)
	}
	
	if len(resolved) != 2 {
		t.Fatalf("Expected 2 resolved references, got %d", len(resolved))
	}
	
	// First image should be resolved
	if resolved[0].Image != "quay.io/redhat-user-workloads/operator/controller:v1.0.0" {
		t.Errorf("Expected resolved image 'quay.io/redhat-user-workloads/operator/controller:v1.0.0', got '%s'", resolved[0].Image)
	}
	
	// Second image should be unchanged
	if resolved[1].Image != "quay.io/other/image:latest" {
		t.Errorf("Expected unchanged image 'quay.io/other/image:latest', got '%s'", resolved[1].Image)
	}
}

func TestGetMappingSummary(t *testing.T) {
	resolver := NewImageResolver()
	
	summary := resolver.GetMappingSummary()
	
	// Check initial state
	if summary["icsp_policies_count"] != 0 {
		t.Errorf("Expected 0 ICSP policies, got %v", summary["icsp_policies_count"])
	}
	
	if summary["idms_policies_count"] != 0 {
		t.Errorf("Expected 0 IDMS policies, got %v", summary["idms_policies_count"])
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || 
		   len(s) > len(substr) && s[len(s)-len(substr):] == substr ||
		   len(substr) < len(s) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}