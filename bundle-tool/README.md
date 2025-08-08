# Bundle Tool

Generate Konflux Snapshots from OLM bundle images using SLSA provenance for source traceability.

## Quick Start

```bash
# Generate snapshot (test-time usage - bundle has provenance)
bundle-tool snapshot quay.io/operator/bundle:v1.0.0 \
  --mirror-policy .tekton/images-mirror-set.yaml

# Generate snapshot (build-time usage - bundle without provenance)
bundle-tool snapshot quay.io/operator/bundle:v1.0.0 \
  --application my-operator \
  --bundle-repo https://github.com/org/operator \
  --bundle-commit abc123def456
```

## Features

- Extract image references from OLM bundles
- Parse SLSA provenance attestations for source information
- Map images using IDMS/ICSP mirror policies  
- Generate Konflux Snapshot YAML with source traceability
- Skip images without valid provenance

## Installation

```bash
go build -o bin/bundle-tool ./cmd
```

## Usage

### CLI

```bash
bundle-tool snapshot [bundle-image] [flags]
```

**Core Flags:**
- `-m, --mirror-policy` - Mirror policy file (IDMS or ICSP)
- `-o, --output` - Output file
- `-n, --namespace` - Target namespace (optional - omit to use current namespace)

**Build-time Flags (when bundle lacks provenance):**
- `-a, --application` - Application name
- `--bundle-repo` - Bundle source repository URL  
- `--bundle-commit` - Bundle source commit SHA

### Tekton Usage

**Build-time (bundle without provenance):**
```yaml
- name: bundle-snapshot
  taskRef:
    resolver: git
    params:
    - name: url
      value: https://github.com/konflux-ci-forks/operator-sdk-builder
    - name: revision
      value: main
    - name: pathInRepo
      value: tekton/tasks/bundle-snapshot-from-bundle/bundle-snapshot-from-bundle.yaml
  params:
  - name: BUNDLE_IMAGE
    value: "$(params.OUTPUT_IMAGE)"
  - name: APPLICATION_NAME  
    value: "$(params.APPLICATION_NAME)"
  - name: BUNDLE_SOURCE_REPO
    value: "$(params.GIT_URL)"
  - name: BUNDLE_SOURCE_COMMIT
    value: "$(params.GIT_COMMIT)"
```

**Test-time (IntegrationTestScenario - bundle has provenance):**
```yaml
- name: bundle-snapshot
  params:
  - name: BUNDLE_IMAGE
    value: "quay.io/operator/bundle:v1.0.0"
  # No source parameters needed - extracted from provenance
```

## Configuration

Create `.tekton/images-mirror-set.yaml`:

```yaml
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: konflux-dev-mapping
spec:
  imageDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads
```

## Output

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Snapshot
spec:
  application: operator
  components:
  - name: operator-controller
    containerImage: quay.io/redhat-user-workloads/operator/controller:v1.0.0
    source:
      git:
        url: https://github.com/operator/operator
        revision: abc123def456
```

## User Script Integration

Use bundle-tool in Konflux build pipelines with the script runner pattern:

```yaml
# In .tekton/build-pipeline.yaml
- name: generate-related-images
  taskRef:
    resolver: bundles
    params:
    - name: bundle
      value: quay.io/konflux-ci/tekton-catalog/task-run-script-oci-ta:latest
    - name: name
      value: run-script-oci-ta
  params:
  - name: SCRIPT
    value: "bundle-tool generate-related-images bundle/ --mirror-policy .tekton/images-mirror-set.yaml"
    # Alternative without mirror policy: "bundle-tool generate-related-images bundle/"
  - name: SCRIPT_RUNNER_IMAGE
    value: "quay.io/konflux-ci/operator-sdk-builder:latest"
```

**Single Command Usage:**
```bash
# Generate relatedImages from deployment containers (no mirror policy needed)
bundle-tool generate-related-images bundle/

# With mirror policy to resolve development -> production registries
bundle-tool generate-related-images bundle/ --mirror-policy .tekton/images-mirror-set.yaml

```

## Documentation

- [CLI Reference](docs/cli-reference.md)
- [Tekton Tasks](../tekton/README.md)
- [Examples](examples/)

## Development

```bash
# Build
go build -o bin/bundle-tool ./cmd

# Test
go test ./... -v

# Format
go fmt ./...
```

## Testing

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# Run specific test suites
go test ./cmd -v                    # CLI command tests
go test ./pkg/resolver -v           # Image resolution tests
go test ./pkg/bundle -v             # Bundle analysis tests
go test ./pkg/snapshot -v           # Snapshot generation tests
```

### Test Coverage

- **`cmd/`** - CLI command structure and generate-related-images functionality
  - Command argument validation and error handling
  - CSV file processing with operator-manifest-tools
  - Mirror policy integration and image resolution
  - Dry-run functionality and file I/O operations
- **`pkg/bundle/`** - Bundle analysis and image extraction
- **`pkg/resolver/`** - ICSP/IDMS mirror policy resolution with CSV integration
- **`pkg/snapshot/`** - Konflux Snapshot generation

### Test Fixtures

Test data is located in `cmd/testdata/` including:
- Sample CSV files with various image reference patterns
- Mirror policy examples (IDMS/ICSP)
- Bundle directory structures for integration testing