package config

import (
	"fmt"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/resolver"
)

// MirrorPolicyConfig represents the unified configuration for mirror policies
type MirrorPolicyConfig struct {
	// Unified field that supports both ICSP and IDMS
	MirrorPolicyFile string
}

// MirrorPolicyLoader provides a unified way to load mirror policies
type MirrorPolicyLoader struct {
	config MirrorPolicyConfig
}

// NewMirrorPolicyLoader creates a new MirrorPolicyLoader with the given configuration
func NewMirrorPolicyLoader(config MirrorPolicyConfig) *MirrorPolicyLoader {
	return &MirrorPolicyLoader{
		config: config,
	}
}

// LoadIntoResolver loads the configured mirror policies into the provided ImageResolver
func (mpl *MirrorPolicyLoader) LoadIntoResolver(imageResolver *resolver.ImageResolver) error {
	if mpl.config.MirrorPolicyFile != "" {
		if err := imageResolver.LoadMirrorPolicy(mpl.config.MirrorPolicyFile); err != nil {
			return fmt.Errorf("failed to load mirror policy: %w", err)
		}
		return nil
	}

	return nil
}

// HasMirrorPolicy returns true if any mirror policy is configured
func (mpl *MirrorPolicyLoader) HasMirrorPolicy() bool {
	return mpl.config.MirrorPolicyFile != ""
}

// Validate checks if the configuration is valid
func (mpl *MirrorPolicyLoader) Validate() error {
	// No validation needed for single field configuration
	return nil
}

// GetDescription returns a human-readable description of the loaded policies
func (mpl *MirrorPolicyLoader) GetDescription() string {
	if mpl.config.MirrorPolicyFile != "" {
		return fmt.Sprintf("mirror policy: %s", mpl.config.MirrorPolicyFile)
	}

	return "no mirror policy"
}
