# CLAUDE.md

Bundle tool for creating Konflux Snapshots from OLM bundle images and generating relatedImages sections in ClusterServiceVersion files.

## Project Structure

```
bundle-tool/
├── cmd/                              # CLI commands
│   ├── main.go                       # Root command
│   ├── snapshot.go                   # Snapshot generation
│   └── generate-related-images.go    # CSV relatedImages generation
├── pkg/                              # Core libraries
│   ├── bundle/analyzer.go            # Bundle image analysis
│   ├── resolver/resolver.go          # ICSP/IDMS image resolution
│   ├── provenance/verifier.go        # Cosign provenance verification
│   └── snapshot/generator.go         # Konflux Snapshot generation
├── tekton/                           # Tekton task definitions
│   ├── tasks/bundle-snapshot-from-bundle/
│   │   ├── bundle-snapshot-from-bundle.yaml
│   │   └── bundle-snapshot-from-bundle-oci-ta.yaml
│   └── README.md
├── examples/                         # Usage examples
├── docs/                             # Documentation
└── BUILD.md                          # Build instructions
```

## Core Components

- **Bundle Analyzer**: Extracts image references from OLM bundle images using containers/image/v5
- **Image Resolver**: Maps images using ICSP/IDMS policies (unified `--mirror-policy` parameter)
- **Provenance Verifier**: Uses cosign for image provenance verification
- **Snapshot Generator**: Creates Konflux Snapshot YAML

## Key Commands

```bash
# Generate relatedImages in CSV files
bundle-tool generate-related-images bundle/ --mirror-policy .tekton/images-mirror-set.yaml

# Create Konflux snapshots from bundles
bundle-tool snapshot quay.io/operator/bundle:v1.0.0 --mirror-policy .tekton/images-mirror-set.yaml
```

## Container Integration

Built into operator-sdk-builder image at `/bin/bundle-tool`. Available for Konflux script runner:

```yaml
- name: SCRIPT_RUNNER_IMAGE
  value: "quay.io/konflux-ci/operator-sdk-builder:latest"
- name: SCRIPT
  value: "bundle-tool generate-related-images bundle/ --mirror-policy .tekton/images-mirror-set.yaml"
```

## Architecture

- **Mirror policy auto-detection**: Single parameter supports both ICSP and IDMS formats
- **Two task variants**: Non-OCI-TA for script runner, OCI-TA for trusted artifacts
- **Hermetic builds**: Compatible with Konflux build requirements