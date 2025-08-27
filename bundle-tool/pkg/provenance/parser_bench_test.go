package provenance

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sigstore/cosign/v2/pkg/cosign"
)

// BenchmarkParseProvenanceData benchmarks the core provenance data parsing
func BenchmarkParseProvenanceData(b *testing.B) {
	parser := NewProvenanceParser()

	// Test data for benchmarking
	testData := `{"predicateType": "https://slsa.dev/provenance/v1", "predicate": {"builder": {"id": "https://github.com/slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@refs/tags/v1.2.0"}, "buildDefinition": {"externalParameters": {"workflow": {"ref": "refs/heads/main", "repository": "https://github.com/example/repo"}}, "resolvedDependencies": [{"uri": "git+https://github.com/example/operator.git", "digest": {"sha1": "abc123def456"}}]}, "invocation": {"environment": {"labels": {"appstudio.openshift.io/component": "my-operator-bundle", "appstudio.openshift.io/application": "my-operator-app"}}}}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info := ProvenanceInfo{
			Metadata: make(map[string]string),
		}
		_ = parser.parseProvenanceData([]byte(testData), &info)
	}
}

// BenchmarkParseAttestationPayload benchmarks attestation payload parsing
func BenchmarkParseAttestationPayload(b *testing.B) {
	parser := NewProvenanceParser()

	testData := `{"predicateType": "https://slsa.dev/provenance/v1", "predicate": {"builder": {"id": "https://example.com/builder"}, "buildDefinition": {"externalParameters": {"workflow": {"ref": "refs/heads/main", "repository": "https://github.com/example/repo"}}, "resolvedDependencies": [{"uri": "git+https://github.com/example/operator.git", "digest": {"sha1": "abc123def456"}}]}, "invocation": {"environment": {"labels": {"appstudio.openshift.io/component": "test-component", "appstudio.openshift.io/application": "test-app"}}}}}`

	// Test both base64 encoded and raw JSON
	attestations := []cosign.AttestationPayload{
		{PayLoad: base64.StdEncoding.EncodeToString([]byte(testData))}, // Base64 encoded
		{PayLoad: testData}, // Raw JSON
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, attestation := range attestations {
			info := ProvenanceInfo{
				Metadata: make(map[string]string),
			}
			_ = parser.parseAttestationPayload(attestation, &info)
		}
	}
}

// BenchmarkParseAttestationPayloadLarge benchmarks parsing with large payloads
func BenchmarkParseAttestationPayloadLarge(b *testing.B) {
	parser := NewProvenanceParser()

	// Create a large but valid payload
	largeData := map[string]interface{}{
		"predicateType": "https://slsa.dev/provenance/v1",
		"predicate": map[string]interface{}{
			"builder": map[string]interface{}{
				"id": "https://example.com/builder",
			},
			"buildDefinition": map[string]interface{}{
				"externalParameters": map[string]interface{}{
					"workflow": map[string]interface{}{
						"ref":        "refs/heads/main",
						"repository": "https://github.com/example/repo",
					},
					"largeData": strings.Repeat("x", 1024*1024), // 1MB of data
				},
			},
			"invocation": map[string]interface{}{
				"environment": map[string]interface{}{
					"labels": map[string]interface{}{
						"appstudio.openshift.io/component":   "test-component",
						"appstudio.openshift.io/application": "test-app",
					},
				},
			},
		},
	}

	jsonData, _ := json.Marshal(largeData)
	attestation := cosign.AttestationPayload{
		PayLoad: base64.StdEncoding.EncodeToString(jsonData),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info := ProvenanceInfo{
			Metadata: make(map[string]string),
		}
		_ = parser.parseAttestationPayload(attestation, &info)
	}
}

// BenchmarkJSONUnmarshal benchmarks JSON unmarshaling performance
func BenchmarkJSONUnmarshal(b *testing.B) {
	testData := `{"predicateType": "https://slsa.dev/provenance/v1", "predicate": {"builder": {"id": "https://github.com/slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@refs/tags/v1.2.0"}, "buildDefinition": {"externalParameters": {"workflow": {"ref": "refs/heads/main", "repository": "https://github.com/example/repo"}}, "resolvedDependencies": [{"uri": "git+https://github.com/example/operator.git", "digest": {"sha1": "abc123def456"}}]}, "invocation": {"environment": {"labels": {"appstudio.openshift.io/component": "my-operator-bundle", "appstudio.openshift.io/application": "my-operator-app"}}}}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var attestation SLSAAttestation
		_ = json.Unmarshal([]byte(testData), &attestation)
	}
}

// BenchmarkBase64Decode benchmarks base64 decoding performance
func BenchmarkBase64Decode(b *testing.B) {
	testData := `{"predicateType": "https://slsa.dev/provenance/v1", "predicate": {"builder": {"id": "https://example.com/builder"}}}`
	encoded := base64.StdEncoding.EncodeToString([]byte(testData))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = base64.StdEncoding.DecodeString(encoded)
	}
}

// BenchmarkGetParsingSummary benchmarks summary generation
func BenchmarkGetParsingSummary(b *testing.B) {
	parser := NewProvenanceParser()

	// Create test results
	results := make([]ProvenanceInfo, 1000)
	for i := 0; i < 1000; i++ {
		results[i] = ProvenanceInfo{
			ImageRef:   "image" + string(rune(i)) + ":latest",
			Verified:   i%2 == 0,
			SourceRepo: "https://github.com/test/repo" + string(rune(i)),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = parser.GetParsingSummary(results)
	}
}
