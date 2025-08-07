# Operator SDK Builder

This repository contains the build configurations for building
the operator-sdk-builder image and operator-related tooling.

The builder image contains the following tools:

- [operator-sdk cli](https://sdk.operatorframework.io/docs/installation/)
- [controller-gen](https://github.com/kubernetes-sigs/controller-tools)
- [opm](https://github.com/operator-framework/operator-registry)
- [kustomize](https://kustomize.io/)
- [bundle-tool](bundle-tool/) - Konflux Snapshot generation from OLM bundles
- [envsubst](https://www.gnu.org/software/gettext/manual/html_node/envsubst-Invocation.html)

Refer to the [.gitmodules](.gitmodules) file in order
to see the versions of `operator-sdk`, `controller-tools`, `operator-registry`, and `kustomize`.

## Additional Tools

### Bundle Tool

The `bundle-tool` subdirectory contains a CLI tool for creating Konflux Snapshots from OLM bundle images. This tool addresses the challenge of mismatched image references between bundle and component images in Konflux CI workflows.

For more information, see [bundle-tool/README.md](bundle-tool/README.md).

## Usage

### Bundle Generation

```dockerfile
FROM konflux-ci/operator-sdk-builder:latest as builder

COPY ./. /repo
WORKDIR /repo
RUN kustomize build config/manifests/ \
    | operator-sdk generate bundle --output-dir build

FROM scratch

COPY --from=builder /repo/build/manifests /manifests/
COPY --from=builder /repo/build/metadata /metadata/
```

### Bundle Tool Usage

```bash
# Generate relatedImages in CSV
bundle-tool generate-related-images bundle/ --mirror-policy .tekton/images-mirror-set.yaml

# Generate Konflux snapshot
bundle-tool snapshot quay.io/operator/bundle:v1.0.0 --mirror-policy .tekton/images-mirror-set.yaml
```
