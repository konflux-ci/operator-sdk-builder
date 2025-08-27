package handlers

import (
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/config"
)

func TestSnapshotHandler_Creation(t *testing.T) {
	handler := NewSnapshotHandler()
	if handler == nil {
		t.Fatal("NewSnapshotHandler() returned nil")
	}

	if handler.analyzer == nil {
		t.Error("analyzer not initialized")
	}

	if handler.imageResolver == nil {
		t.Error("imageResolver not initialized")
	}

	if handler.provenanceParser == nil {
		t.Error("provenanceParser not initialized")
	}
}

func TestGenerateRelatedImagesHandler_Creation(t *testing.T) {
	handler := NewGenerateRelatedImagesHandler()
	if handler == nil {
		t.Fatal("NewGenerateRelatedImagesHandler() returned nil")
	}

	if handler.imageResolver == nil {
		t.Error("imageResolver not initialized")
	}
}

func TestSnapshotRequest_Validation(t *testing.T) {
	handler := NewSnapshotHandler()

	tests := []struct {
		name    string
		req     SnapshotRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: SnapshotRequest{
				BundleImage:  "quay.io/test/bundle:v1.0.0",
				OutputFile:   "snapshot.yaml",
				Namespace:    "test-ns",
				AppName:      "test-app",
				MirrorPolicy: config.MirrorPolicyConfig{},
			},
			wantErr: false,
		},
		{
			name: "missing bundle image",
			req: SnapshotRequest{
				OutputFile:   "snapshot.yaml",
				Namespace:    "test-ns",
				AppName:      "test-app",
				MirrorPolicy: config.MirrorPolicyConfig{},
			},
			wantErr: true,
		},
		{
			name: "mirror policy configured",
			req: SnapshotRequest{
				BundleImage: "quay.io/test/bundle:v1.0.0",
				OutputFile:  "snapshot.yaml",
				Namespace:   "test-ns",
				AppName:     "test-app",
				MirrorPolicy: config.MirrorPolicyConfig{
					MirrorPolicyFile: "policy.yaml",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateRelatedImagesRequest_Validation(t *testing.T) {
	handler := NewGenerateRelatedImagesHandler()

	tests := []struct {
		name    string
		req     GenerateRelatedImagesRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: GenerateRelatedImagesRequest{
				Target:       "bundle/",
				MirrorPolicy: config.MirrorPolicyConfig{},
				DryRun:       true,
			},
			wantErr: false,
		},
		{
			name: "missing target",
			req: GenerateRelatedImagesRequest{
				MirrorPolicy: config.MirrorPolicyConfig{},
				DryRun:       true,
			},
			wantErr: true,
		},
		{
			name: "mirror policy configured",
			req: GenerateRelatedImagesRequest{
				Target: "bundle/",
				MirrorPolicy: config.MirrorPolicyConfig{
					MirrorPolicyFile: "policy.yaml",
				},
				DryRun: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
