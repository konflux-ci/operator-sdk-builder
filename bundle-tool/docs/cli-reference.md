# CLI Reference

## Commands

### `bundle-tool snapshot [bundle-image]`

Generate Konflux snapshot from OLM bundle image using SLSA provenance for source traceability.

### `bundle-tool generate-related-images [csv-file-or-directory]`

Generate and update relatedImages in ClusterServiceVersion from deployment containers. Optionally resolves image references using mirror policy mapping.

**Arguments (snapshot):**
- `bundle-image` - Bundle image reference (required)

**Arguments (generate-related-images):**
- `csv-file-or-directory` - CSV file path or bundle directory containing manifests/ (required)

**Flags (snapshot):**
- `-m, --mirror-policy string` - Mirror policy file path (IDMS or ICSP - auto-detected)
- `-o, --output string` - Output file (default: stdout)  
- `-n, --namespace string` - Target namespace (optional - omit to use current namespace when applying)
- `-a, --application string` - Application name (required when bundle provenance unavailable)
- `--bundle-repo string` - Bundle source repository URL (required for build-time usage when bundle provenance unavailable)
- `--bundle-commit string` - Bundle source commit SHA (required for build-time usage when bundle provenance unavailable)

**Flags (generate-related-images):**
- `-m, --mirror-policy string` - Mirror policy file path (IDMS or ICSP - auto-detected, optional)
- `--dry-run` - Show changes without modifying files (default: false)

**Examples:**

```bash
# Snapshot generation (test-time - bundle has provenance)
bundle-tool snapshot quay.io/operator/bundle:v1.0.0
bundle-tool snapshot quay.io/operator/bundle:v1.0.0 --mirror-policy .tekton/images-mirror-set.yaml

# Snapshot generation (build-time - bundle without provenance)  
bundle-tool snapshot quay.io/operator/bundle:v1.0.0 \
  --application my-operator \
  --bundle-repo https://github.com/org/operator \
  --bundle-commit abc123def456

# RelatedImages generation (for script runner)
bundle-tool generate-related-images bundle/
bundle-tool generate-related-images bundle/ --mirror-policy .tekton/images-mirror-set.yaml
bundle-tool generate-related-images bundle/ --dry-run --mirror-policy .tekton/images-mirror-set.yaml
```

## Configuration Files

Mirror policy files are auto-detected based on the `kind` field. Both formats use the same core structure.

### IDMS Format (Current)

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

### ICSP Format (Legacy)

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: konflux-dev-mapping
spec:
  repositoryDigestMirrors:
  - source: registry.redhat.io
    mirrors:
    - quay.io/redhat-user-workloads
```

**Note**: Use `--mirror-policy` for both formats. The tool automatically detects ICSP vs IDMS based on the `kind` field.

## Output

Generates Konflux Snapshot YAML with source information extracted from SLSA provenance:

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Snapshot
metadata:
  name: operator-bundle-snapshot-20250107-120000
  labels:
    appstudio.openshift.io/application: operator
  annotations:
    bundle-tool.konflux.io/source-bundle: quay.io/operator/bundle:v1.0.0
spec:
  application: operator
  components:
  - name: operator-bundle
    containerImage: quay.io/operator/bundle:v1.0.0
    source:
      git:
        url: https://github.com/operator/operator
        revision: abc123def456
  - name: operator-controller
    containerImage: quay.io/redhat-user-workloads/operator/controller:v1.0.0
    source:
      git:
        url: https://github.com/operator/controller
        revision: def456abc123
```

**Notes:**
- Only includes images with valid SLSA provenance
- Bundle component always included (uses fallback source if no provenance)
- Component images without provenance are skipped with warning messages

## Exit Codes

- `0` - Success
- `1` - General error
- `2` - Bundle analysis failed
- `3` - Image resolution failed  
- `4` - Provenance verification failed
- `5` - Snapshot generation failed

## Troubleshooting

**Bundle image not found**: Check registry credentials and image reference.

**IDMS file not found**: Verify file path and format.

**Cosign verification fails**: Install cosign or use `--verify-provenance=false`.

**Invalid snapshot**: Check bundle format and image references.