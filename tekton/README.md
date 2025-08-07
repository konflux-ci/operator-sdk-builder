# Tekton Tasks

## bundle-snapshot-from-bundle

Creates Konflux Snapshot from OLM bundle image by extracting source information from SLSA provenance.

**Variants:**
- `bundle-snapshot-from-bundle.yaml` - Standard task
- `bundle-snapshot-from-bundle-oci-ta.yaml` - Trusted Artifacts variant

**Usage Patterns:**

### Build-time Usage (Bundle without provenance)
```yaml
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

### Test-time Usage (IntegrationTestScenario with provenance)
```yaml
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
  value: "quay.io/operator/bundle:v1.0.0"
# No source parameters needed - extracted from provenance
```

**Parameters:**

**Core Parameters:**
- `BUNDLE_IMAGE` - Bundle image reference (required)
- `MIRROR_POLICY_PATH` - Mirror policy file path (optional, default: `.tekton/images-mirror-set.yaml`)
- `OUTPUT_FILE` - Snapshot output file (default: `konflux-snapshot.yaml`)

**Build-time Parameters (required when bundle lacks provenance):**
- `APPLICATION_NAME` - Application name in snapshot
- `BUNDLE_SOURCE_REPO` - Bundle source repository URL
- `BUNDLE_SOURCE_COMMIT` - Bundle source commit SHA

**Optional Parameters:**
- `VERIFY_PROVENANCE` - Verify image provenance (default: `true`)
- `NAMESPACE` - Target namespace (omit to use current namespace when applying)