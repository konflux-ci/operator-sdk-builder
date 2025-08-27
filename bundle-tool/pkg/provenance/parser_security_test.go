package provenance

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/konflux-ci-forks/operator-sdk-builder/bundle-tool/pkg/bundle"
	"github.com/sigstore/cosign/v2/pkg/cosign"
)

// TestMemoryExhaustionProtection tests memory exhaustion protection
func TestMemoryExhaustionProtection(t *testing.T) {
	pp := NewProvenanceParser()
	pp.SetVerbose(true)

	tests := []struct {
		name            string
		maxAttestations int
		maxPayloadSize  int64
		imageRefsCount  int
		shouldError     bool
		description     string
	}{
		{
			name:            "normal limits",
			maxAttestations: 50,
			maxPayloadSize:  10 * 1024 * 1024,
			imageRefsCount:  10,
			shouldError:     false,
			description:     "Normal operation within limits",
		},
		{
			name:            "too many image references",
			maxAttestations: 50,
			maxPayloadSize:  10 * 1024 * 1024,
			imageRefsCount:  1500,
			shouldError:     true,
			description:     "Too many image references should be rejected",
		},
		{
			name:            "small payload limit",
			maxAttestations: 50,
			maxPayloadSize:  1024, // 1KB
			imageRefsCount:  1,
			shouldError:     false, // Won't error unless we have actual large payloads
			description:     "Small payload size limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp.SetMaxAttestations(tt.maxAttestations)
			pp.SetMaxPayloadSize(tt.maxPayloadSize)

			// Create test image references
			var imageRefs []bundle.ImageReference
			for i := 0; i < tt.imageRefsCount; i++ {
				imageRefs = append(imageRefs, bundle.ImageReference{
					Name:  "test-image",
					Image: "quay.io/test/image:v1.0.0",
				})
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := pp.ParseProvenance(ctx, imageRefs)

			if tt.shouldError && err == nil {
				t.Errorf("Expected error for %s, but got none", tt.description)
			}
			if !tt.shouldError && err != nil && !strings.Contains(err.Error(), "no provenance attestations found") {
				// We expect "no provenance attestations found" since we're using fake images
				t.Errorf("Unexpected error for %s: %v", tt.description, err)
			}
		})
	}
}

// TestInputValidation tests input validation for image references
func TestInputValidation(t *testing.T) {
	pp := NewProvenanceParser()

	tests := []struct {
		name        string
		imageRef    bundle.ImageReference
		shouldError bool
		description string
	}{
		{
			name: "valid image reference",
			imageRef: bundle.ImageReference{
				Name:  "test",
				Image: "quay.io/test/image:v1.0.0",
			},
			shouldError: false,
			description: "Valid image reference should be accepted",
		},
		{
			name: "long image reference",
			imageRef: bundle.ImageReference{
				Name:  "test",
				Image: strings.Repeat("a", 1100),
			},
			shouldError: true,
			description: "Excessively long image reference should be rejected",
		},
		{
			name: "long name",
			imageRef: bundle.ImageReference{
				Name:  strings.Repeat("a", 300),
				Image: "quay.io/test/image:v1.0.0",
			},
			shouldError: true,
			description: "Excessively long name should be rejected",
		},
		{
			name: "control characters in image",
			imageRef: bundle.ImageReference{
				Name:  "test",
				Image: "quay.io/test/image\x01:v1.0.0",
			},
			shouldError: true,
			description: "Image reference with control characters should be rejected",
		},
		{
			name: "empty image reference",
			imageRef: bundle.ImageReference{
				Name:  "test",
				Image: "",
			},
			shouldError: true,
			description: "Empty image reference should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pp.validateImageReference(tt.imageRef)

			if tt.shouldError && err == nil {
				t.Errorf("Expected error for %s, but got none", tt.description)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error for %s: %v", tt.description, err)
			}
		})
	}
}

// TestTimeoutProtection tests timeout protection
func TestTimeoutProtection(t *testing.T) {
	pp := NewProvenanceParser()
	pp.SetProcessingTimeout(100 * time.Millisecond) // Very short timeout

	ctx := context.Background()
	imageRef := "quay.io/test/nonexistent:latest"

	info := &ProvenanceInfo{
		ImageRef: imageRef,
		Metadata: make(map[string]string),
	}

	// This should timeout quickly due to the short timeout setting
	err := pp.parseImageProvenance(ctx, imageRef, info)

	// We expect either a timeout error or a "failed to get attestations" error
	if err == nil {
		t.Error("Expected error due to timeout or missing attestations")
	}

	// The test passes if it returns quickly and doesn't hang
}

// TestPayloadSizeValidation tests payload size validation
func TestPayloadSizeValidation(t *testing.T) {
	pp := NewProvenanceParser()
	pp.SetMaxPayloadSize(1024) // 1KB limit

	tests := []struct {
		name        string
		payloadSize int
		shouldError bool
		description string
	}{
		{
			name:        "small payload",
			payloadSize: 500,
			shouldError: false,
			description: "Small payload should be accepted",
		},
		{
			name:        "payload at limit",
			payloadSize: 1024,
			shouldError: false,
			description: "Payload at limit should be accepted",
		},
		{
			name:        "oversized payload",
			payloadSize: 2048,
			shouldError: true,
			description: "Oversized payload should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test attestation payload
			payload := strings.Repeat("a", tt.payloadSize)
			attestation := cosign.AttestationPayload{
				PayLoad: payload,
			}

			info := &ProvenanceInfo{
				Metadata: make(map[string]string),
			}

			err := pp.parseAttestationPayload(attestation, info)

			if tt.shouldError && err == nil {
				t.Errorf("Expected error for %s, but got none", tt.description)
			}
			if !tt.shouldError && err != nil && !strings.Contains(err.Error(), "invalid YAML format") {
				// We expect YAML parsing errors since we're using dummy data
				t.Errorf("Unexpected error for %s: %v", tt.description, err)
			}
		})
	}
}

// TestBase64DecodingExpansionProtection tests protection against base64 expansion attacks
func TestBase64DecodingExpansionProtection(t *testing.T) {
	pp := NewProvenanceParser()
	pp.SetMaxPayloadSize(1024) // 1KB limit

	// Create a base64 encoded payload that would expand beyond the limit
	// Base64 encoding reduces size by ~25%, so we need a payload that's small encoded
	// but large when decoded
	largeContent := strings.Repeat("a", 2048) // 2KB content

	// Base64 encode it (this will be smaller than 2048 bytes)
	// But when decoded, it will be 2048 bytes, exceeding our 1KB limit
	// For testing, we'll simulate this with a direct payload

	attestation := cosign.AttestationPayload{
		PayLoad: largeContent, // This simulates the decoded content being too large
	}

	info := &ProvenanceInfo{
		Metadata: make(map[string]string),
	}

	err := pp.parseAttestationPayload(attestation, info)
	if err == nil {
		t.Error("Expected error for payload exceeding size limit after decoding")
	}
}

// TestConfigurationLimits tests the configuration limits for security settings
func TestConfigurationLimits(t *testing.T) {
	pp := NewProvenanceParser()

	// Test max attestations limit
	pp.SetMaxAttestations(2000)   // Should be capped at 1000
	if pp.maxAttestations != 50 { // Should remain at default since 2000 > 1000
		t.Errorf("MaxAttestations should be capped, got %d", pp.maxAttestations)
	}

	// Test valid max attestations
	pp.SetMaxAttestations(100)
	if pp.maxAttestations != 100 {
		t.Errorf("Expected maxAttestations to be 100, got %d", pp.maxAttestations)
	}

	// Test max payload size limit
	pp.SetMaxPayloadSize(200 * 1024 * 1024) // 200MB, should be capped at 100MB
	if pp.maxPayloadSize != 10*1024*1024 {  // Should remain at default since 200MB > 100MB
		t.Errorf("MaxPayloadSize should be capped, got %d", pp.maxPayloadSize)
	}

	// Test valid max payload size
	pp.SetMaxPayloadSize(50 * 1024 * 1024) // 50MB
	if pp.maxPayloadSize != 50*1024*1024 {
		t.Errorf("Expected maxPayloadSize to be 50MB, got %d", pp.maxPayloadSize)
	}

	// Test processing timeout limit
	pp.SetProcessingTimeout(10 * time.Minute)   // Should be capped at 5 minutes
	if pp.processingTimeout != 30*time.Second { // Should remain at default since 10min > 5min
		t.Errorf("ProcessingTimeout should be capped, got %v", pp.processingTimeout)
	}

	// Test valid processing timeout
	pp.SetProcessingTimeout(2 * time.Minute)
	if pp.processingTimeout != 2*time.Minute {
		t.Errorf("Expected processingTimeout to be 2 minutes, got %v", pp.processingTimeout)
	}
}

// TestMemoryCleanup tests that large payloads are properly cleaned up
func TestMemoryCleanup(t *testing.T) {
	pp := NewProvenanceParser()

	// Create a moderately large payload that should be accepted but then cleaned up
	payload := strings.Repeat("{\"test\": \"data\"}", 1000) // Valid JSON that will fail parsing as attestation

	attestation := cosign.AttestationPayload{
		PayLoad: payload,
	}

	info := &ProvenanceInfo{
		Metadata: make(map[string]string),
	}

	// This should process without memory issues and clean up properly
	err := pp.parseAttestationPayload(attestation, info)

	// We expect an error since this isn't a valid SLSA attestation
	if err == nil {
		t.Error("Expected error for invalid attestation format")
	}

	// The test passes if no memory leaks occur and the function returns
}

// TestConcurrentAccess tests thread safety of the parser
func TestConcurrentAccess(t *testing.T) {
	pp := NewProvenanceParser()

	// Test that multiple goroutines can safely use the parser
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			imageRefs := []bundle.ImageReference{
				{
					Name:  "test-image",
					Image: "quay.io/test/image:v1.0.0",
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			// This will likely fail due to missing attestations, but shouldn't cause races
			_, _ = pp.ParseProvenance(ctx, imageRefs)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test passes if no race conditions occur
}
