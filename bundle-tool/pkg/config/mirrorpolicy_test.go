package config

import (
	"testing"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/resolver"
)

func TestMirrorPolicyLoader_HasMirrorPolicy(t *testing.T) {
	tests := []struct {
		name   string
		config MirrorPolicyConfig
		want   bool
	}{
		{
			name:   "no policy configured",
			config: MirrorPolicyConfig{},
			want:   false,
		},
		{
			name: "mirror policy file configured",
			config: MirrorPolicyConfig{
				MirrorPolicyFile: "policy.yaml",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpl := NewMirrorPolicyLoader(tt.config)
			if got := mpl.HasMirrorPolicy(); got != tt.want {
				t.Errorf("MirrorPolicyLoader.HasMirrorPolicy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMirrorPolicyLoader_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  MirrorPolicyConfig
		wantErr bool
	}{
		{
			name:    "no policy configured",
			config:  MirrorPolicyConfig{},
			wantErr: false,
		},
		{
			name: "mirror policy configured",
			config: MirrorPolicyConfig{
				MirrorPolicyFile: "policy.yaml",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpl := NewMirrorPolicyLoader(tt.config)
			if err := mpl.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("MirrorPolicyLoader.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMirrorPolicyLoader_GetDescription(t *testing.T) {
	tests := []struct {
		name   string
		config MirrorPolicyConfig
		want   string
	}{
		{
			name:   "no policy configured",
			config: MirrorPolicyConfig{},
			want:   "no mirror policy",
		},
		{
			name: "mirror policy file configured",
			config: MirrorPolicyConfig{
				MirrorPolicyFile: "policy.yaml",
			},
			want: "mirror policy: policy.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpl := NewMirrorPolicyLoader(tt.config)
			if got := mpl.GetDescription(); got != tt.want {
				t.Errorf("MirrorPolicyLoader.GetDescription() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMirrorPolicyLoader_LoadIntoResolver(t *testing.T) {
	// Note: These tests require actual YAML files to be present
	// For now, we'll test the validation logic without file I/O
	tests := []struct {
		name    string
		config  MirrorPolicyConfig
		wantErr bool
	}{
		{
			name:    "no policy configured",
			config:  MirrorPolicyConfig{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mpl := NewMirrorPolicyLoader(tt.config)
			imageResolver := resolver.NewImageResolver()

			if err := mpl.LoadIntoResolver(imageResolver); (err != nil) != tt.wantErr {
				t.Errorf("MirrorPolicyLoader.LoadIntoResolver() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
