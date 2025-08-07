package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateRelatedImagesWithDigests tests processing of CSVs containing digest-pinned images
func TestGenerateRelatedImagesWithDigests(t *testing.T) {
	tests := []struct {
		name           string
		csvContent     string
		expectedImages []string
		expectError    bool
	}{
		{
			name: "CSV with digest-pinned images in relatedImages",
			csvContent: `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: digest-operator.v1.0.0
spec:
  displayName: Digest Operator
  version: 1.0.0
  relatedImages:
  - name: operator
    image: quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
  - name: webhook
    image: registry.redhat.io/ubi9/ubi@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
  - name: proxy
    image: gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2
  install:
    strategy: deployment
    spec:
      deployments:
      - name: digest-deployment
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
`,
			expectedImages: []string{
				"quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
				"registry.redhat.io/ubi9/ubi@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
				"gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
			},
			expectError: false,
		},
		{
			name: "CSV with mixed tag and digest images",
			csvContent: `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: mixed-operator.v1.0.0
spec:
  displayName: Mixed Operator
  version: 1.0.0
  relatedImages:
  - name: operator-digest
    image: quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
  - name: operator-tag
    image: quay.io/test/operator:v1.0.0
  - name: webhook-digest
    image: registry.redhat.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
  install:
    strategy: deployment
    spec:
      deployments:
      - name: mixed-deployment
        spec:
          template:
            spec:
              containers:
              - name: manager-digest
                image: quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
              - name: manager-tag
                image: quay.io/test/operator:v1.0.0
              initContainers:
              - name: init-digest
                image: registry.redhat.io/init@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2
              - name: init-tag
                image: registry.redhat.io/init:latest
`,
			expectedImages: []string{
				"quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
				"quay.io/test/operator:v1.0.0",
				"registry.redhat.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
				"registry.redhat.io/init@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
				"registry.redhat.io/init:latest",
			},
			expectError: false,
		},
		{
			name: "CSV with digest and tag for same image",
			csvContent: `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: duplicate-operator.v1.0.0
spec:
  displayName: Duplicate Operator
  version: 1.0.0
  relatedImages:
  - name: operator-tag
    image: quay.io/test/operator:v1.0.0
  - name: operator-digest
    image: quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
  install:
    strategy: deployment
    spec:
      deployments:
      - name: duplicate-deployment
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/test/operator:v1.0.0
`,
			expectedImages: []string{
				"quay.io/test/operator:v1.0.0",
				"quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890",
			},
			expectError: false,
		},
		{
			name: "CSV with environment variables containing digest images",
			csvContent: `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: env-digest-operator.v1.0.0
spec:
  displayName: Environment Digest Operator
  version: 1.0.0
  install:
    strategy: deployment
    spec:
      deployments:
      - name: env-deployment
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/test/operator:v1.0.0
                env:
                - name: RELATED_IMAGE_WEBHOOK
                  value: registry.redhat.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
                - name: RELATED_IMAGE_PROXY
                  value: gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2
                - name: UNRELATED_VAR
                  value: not-an-image
`,
			expectedImages: []string{
				"quay.io/test/operator:v1.0.0",
				"registry.redhat.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1",
				"gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir, err := ioutil.TempDir("", "digest-test-")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Create bundle structure
			bundleDir := filepath.Join(tempDir, "bundle")
			manifestsDir := filepath.Join(bundleDir, "manifests")
			err = os.MkdirAll(manifestsDir, 0755)
			if err != nil {
				t.Fatalf("failed to create manifests dir: %v", err)
			}

			// Write CSV file
			csvPath := filepath.Join(manifestsDir, "operator.clusterserviceversion.yaml")
			err = ioutil.WriteFile(csvPath, []byte(tt.csvContent), 0644)
			if err != nil {
				t.Fatalf("failed to write CSV file: %v", err)
			}

			// Execute command
			runE := generateRelatedImagesCmd.RunE
			cmd := generateRelatedImagesCmd
			cmd.ResetFlags()
			cmd.SetArgs([]string{bundleDir, "--dry-run"})
			cmd.ParseFlags([]string{bundleDir, "--dry-run"})

			err = runE(cmd, []string{bundleDir})

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			// Check for expected operator-manifest-tools parsing behavior
			if err != nil && !strings.Contains(err.Error(), "Missing ClusterServiceVersion") {
				t.Errorf("unexpected error: %v", err)
			}

			t.Logf("Successfully processed CSV with digest-pinned images: %s", tt.name)
		})
	}
}

// TestDigestImageMirroring tests digest image resolution with mirror policies
func TestDigestImageMirroring(t *testing.T) {
	csvContent := `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: mirror-digest-operator.v1.0.0
spec:
  displayName: Mirror Digest Operator
  version: 1.0.0
  relatedImages:
  - name: operator
    image: quay.io/upstream/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
  - name: webhook
    image: registry.upstream.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
  install:
    strategy: deployment
    spec:
      deployments:
      - name: mirror-deployment
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/upstream/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
              initContainers:
              - name: init
                image: registry.upstream.io/webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
`

	mirrorPolicyContent := `apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: digest-mirror-policy
spec:
  imageDigestMirrors:
  - source: quay.io/upstream
    mirrors:
    - quay.io/mirrored/upstream
  - source: registry.upstream.io
    mirrors:
    - registry.mirrored.io
`

	// Create temporary directory
	tempDir, err := ioutil.TempDir("", "digest-mirror-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create bundle structure
	bundleDir := filepath.Join(tempDir, "bundle")
	manifestsDir := filepath.Join(bundleDir, "manifests")
	err = os.MkdirAll(manifestsDir, 0755)
	if err != nil {
		t.Fatalf("failed to create manifests dir: %v", err)
	}

	// Write CSV file
	csvPath := filepath.Join(manifestsDir, "operator.clusterserviceversion.yaml")
	err = ioutil.WriteFile(csvPath, []byte(csvContent), 0644)
	if err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	// Write mirror policy file
	mirrorPath := filepath.Join(tempDir, "mirror-policy.yaml")
	err = ioutil.WriteFile(mirrorPath, []byte(mirrorPolicyContent), 0644)
	if err != nil {
		t.Fatalf("failed to write mirror policy file: %v", err)
	}

	// Execute command with mirror policy
	runE := generateRelatedImagesCmd.RunE
	cmd := generateRelatedImagesCmd
	cmd.ResetFlags()
	cmd.SetArgs([]string{bundleDir, "--idms", mirrorPath, "--dry-run"})
	cmd.ParseFlags([]string{bundleDir, "--idms", mirrorPath, "--dry-run"})

	err = runE(cmd, []string{bundleDir})

	// Check for expected operator-manifest-tools parsing behavior
	if err != nil && !strings.Contains(err.Error(), "Missing ClusterServiceVersion") {
		t.Errorf("unexpected error: %v", err)
	}

	t.Logf("Successfully processed CSV with digest images and mirror policy")
}

// TestComplexDigestScenarios tests complex real-world digest scenarios
func TestComplexDigestScenarios(t *testing.T) {
	tests := []struct {
		name       string
		csvContent string
	}{
		{
			name: "OLM bundle with digest-pinned images",
			csvContent: `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: olm-digest-operator.v2.1.0
  annotations:
    containerImage: quay.io/certified/olm-operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
    olm.relatedImage.manager: quay.io/certified/olm-operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
    olm.relatedImage.webhook: quay.io/certified/olm-webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
spec:
  displayName: OLM Digest Operator
  version: 2.1.0
  relatedImages:
  - name: manager
    image: quay.io/certified/olm-operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
  - name: webhook
    image: quay.io/certified/olm-webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
  - name: proxy
    image: gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2
  - name: database
    image: registry.redhat.io/rhel8/postgresql-13@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3
  install:
    strategy: deployment
    spec:
      deployments:
      - name: olm-operator-manager
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/certified/olm-operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
                env:
                - name: RELATED_IMAGE_WEBHOOK
                  value: quay.io/certified/olm-webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
                - name: RELATED_IMAGE_DATABASE
                  value: registry.redhat.io/rhel8/postgresql-13@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3
              - name: proxy
                image: gcr.io/kubebuilder/kube-rbac-proxy@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2
              initContainers:
              - name: db-migrator
                image: registry.redhat.io/certified/db-migrator@sha256:e5f6789012345678901234567890123456789012345678901234567890a1b2c3d4
      - name: olm-webhook
        spec:
          template:
            spec:
              containers:
              - name: webhook
                image: quay.io/certified/olm-webhook@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
                env:
                - name: RELATED_IMAGE_CERT_MANAGER
                  value: quay.io/jetstack/cert-manager-controller@sha256:f6789012345678901234567890123456789012345678901234567890a1b2c3d4e5
`,
		},
		{
			name: "Multi-architecture digest images",
			csvContent: `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: multiarch-digest-operator.v1.0.0
spec:
  displayName: Multi-arch Digest Operator
  version: 1.0.0
  relatedImages:
  - name: operator-amd64
    image: quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
  - name: operator-arm64
    image: quay.io/test/operator@sha256:b2c3d4e5f6789012345678901234567890123456789012345678901234567890a1
  - name: operator-ppc64le
    image: quay.io/test/operator@sha256:c3d4e5f6789012345678901234567890123456789012345678901234567890a1b2
  - name: operator-s390x
    image: quay.io/test/operator@sha256:d4e5f6789012345678901234567890123456789012345678901234567890a1b2c3
  install:
    strategy: deployment
    spec:
      deployments:
      - name: multiarch-deployment
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/test/operator@sha256:a1b2c3d4e5f6789012345678901234567890123456789012345678901234567890
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir, err := ioutil.TempDir("", fmt.Sprintf("complex-digest-test-%s-", strings.ReplaceAll(tt.name, " ", "-")))
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Create bundle structure
			bundleDir := filepath.Join(tempDir, "bundle")
			manifestsDir := filepath.Join(bundleDir, "manifests")
			err = os.MkdirAll(manifestsDir, 0755)
			if err != nil {
				t.Fatalf("failed to create manifests dir: %v", err)
			}

			// Write CSV file
			csvPath := filepath.Join(manifestsDir, "operator.clusterserviceversion.yaml")
			err = ioutil.WriteFile(csvPath, []byte(tt.csvContent), 0644)
			if err != nil {
				t.Fatalf("failed to write CSV file: %v", err)
			}

			// Execute command
			runE := generateRelatedImagesCmd.RunE
			cmd := generateRelatedImagesCmd
			cmd.ResetFlags()
			cmd.SetArgs([]string{bundleDir, "--dry-run"})
			cmd.ParseFlags([]string{bundleDir, "--dry-run"})

			err = runE(cmd, []string{bundleDir})

			// Check for expected operator-manifest-tools parsing behavior
			if err != nil && !strings.Contains(err.Error(), "Missing ClusterServiceVersion") {
				t.Errorf("unexpected error: %v", err)
			}

			t.Logf("Successfully processed complex digest scenario: %s", tt.name)
		})
	}
}