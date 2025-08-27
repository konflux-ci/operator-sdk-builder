# Konflux Conventions

## IDMS Parameter Pattern

Matches `run-opm-command-oci-ta` and `fips-operator-bundle-check-oci-ta`:

```yaml
- name: IDMS_PATH
  description: Optional, path for ImageDigestMirrorSet file. It defaults to .tekton/images-mirror-set.yaml
  type: string
  default: ".tekton/images-mirror-set.yaml"
```

## File Locations

```
.tekton/images-mirror-set.yaml  # Standard IDMS location
.tekton/images-content-source-policy.yaml  # Legacy ICSP
```

## Task Structure

- **Standard task**: Basic workspace usage
- **OCI-TA variant**: Trusted Artifacts support
- **Conditional execution**: Check file existence before using
- **Proper metadata**: Includes required labels and annotations

## Registry Patterns

**Production**: `registry.redhat.io/*`
**Development**: `quay.io/redhat-user-workloads/*`

## Snapshot Format

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Snapshot
metadata:
  labels:
    appstudio.openshift.io/application: {app}
    bundle-tool.konflux.io/source: bundle-analysis
spec:
  application: {app}
  components:
  - name: {component}
    containerImage: {resolved-image}
    source:
      git:
        url: {repo}
        revision: {commit}
```

## Pipeline Usage

```yaml
# Use default IDMS path
- name: BUNDLE_IMAGE
  value: "quay.io/operator/bundle:v1.0.0"

# Disable IDMS (like fbc-builder pipeline)
- name: IDMS_PATH
  value: ""
```