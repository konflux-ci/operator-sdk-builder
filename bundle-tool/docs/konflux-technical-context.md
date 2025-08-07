# Konflux Technical Context

This document contains technical information extracted from internal Konflux documentation for reference during development. This information helps understand the context and requirements for building tooling to create snapshots from bundle images.

## OLM Migration Overview

### Key Migration Changes
1. **Build System Modifications**
   - No longer manipulates bundle images
   - Does not remap image pullspecs
   - Does not digest-pin image tags
   - Does not generate `relatedImages`

2. **Metadata and Catalog Requirements**
   - Must migrate to File-based Catalog (FBC)
   - One FBC artifact per OpenShift release version
   - FBC defines operator upgrade graph
   - Requires `@konflux-releng` to set `fbc_opt_in` flag in Pyxis

3. **Component Build Process**
   - Still composed of: Operands, Operators, Bundle images

4. **Image Reference Requirements**
   - Bundle images must have digest-pinned references
   - Use `registry.redhat.io` pullspecs
   - Image pullspecs remain "invalid" until images are released

5. **Catalog Publication**
   - Graph published via `redhat-operator-index`
   - Corresponds to specific OpenShift versions

## Building OLM Operands, Operators, and Bundles

### Bundle Image Specifics
- **Must be built in a hermetic environment**
- **Should NOT be multi-arch** (single OCI Image Manifest)
- **Image pullspecs must be pinned at build time**
- **All deployment images must be listed in `relatedImages`**

### Image Pinning Strategies

#### Internal Konflux Pullspecs
- Format: `quay.io/redhat-user-workloads/<namespace>/<application-name>/<component-name>@sha256:<digest>`
- Enables iterative development and testing

#### Registry.redhat.io Pullspecs
- Requires properly configured Release Plan Admissions (RPAs)
- Map components to target push repositories
- Update image references in bundle repository

### Nudging Mechanism
Automatically updates image references in files matching:
- `.*Dockerfile.*`
- `*.yaml`
- `.*Containerfile.*`

### Recommended Build Approaches
- Use multi-stage builds for manifest generation
- Create data files for image references
- Separate internal and release bundle images
- Avoid manual registry.redhat.io mapping

## Building OLM File-based Catalog (FBC) Components

### Directory Structure Requirements
- Create nested directory under `/configs`
- Subdirectory name must match the `package` field of the operator bundle
- Example: `configs/package-name/catalog.json`

### Image Requirements
- **Up to OpenShift v4.14**: `registry.redhat.io/openshift4/ose-operator-registry`
- **OpenShift v4.15 onwards**: `registry.redhat.io/openshift4/ose-operator-registry-rhel9`

### Bundle Image Handling
- **"Bundle images SHOULD NOT be referenced by OCI Image Index or manifest list"**
- Use first referenced image if manifest list is encountered

### Catalog Generation
- For OCP 4.17+, use `--migrate-level=bundle-object-to-csv-metadata` flag

### Release and Testing Strategies
Support multiple release pipelines:
- Generate "stage" index
- Push to production
- Push hotfix index
- Push isolated pre-GA index

### Recommended Practices
- Build multi-arch FBC fragments
- Required for pre-GA indexes and multi-arch testing
- Use semver template for managing upgrade graphs
- Name pre-release channels with `-candidate-`, `-dev-preview-`, or `-pre-ga-` formats

### Testing Approach
- Create CatalogSource in testing cluster
- Use ImageDigestMirrorSet for pullspec mapping
- Validate images through Conforma checks

## Technical Rationale for FBC
- Enables catalog editing
- Supports catalog composability
- Provides extensibility for operator graphs

## Implementation Implications for Snapshot Tooling

### Bundle Analysis Considerations
1. **Bundle Format**: Single OCI Image Manifest (not multi-arch)
2. **Image References**: All images must be in `spec.relatedImages`
3. **Pinning Requirements**: All pullspecs must be digest-pinned
4. **Hermetic Build Context**: Bundles built in isolated environments

### Image Resolution Requirements
1. **Development Pullspecs**: Use `quay.io/redhat-user-workloads/...` format
2. **Production Pullspecs**: Use `registry.redhat.io` format
3. **Nudging Patterns**: Files matching `.*Dockerfile.*`, `*.yaml`, `.*Containerfile.*`

### FBC Integration Points
1. **Directory Structure**: `/configs/<package-name>/catalog.json`
2. **Parent Image Mapping**: Different base images for different OCP versions
3. **Multi-arch Handling**: First image from manifest lists
4. **Release Pipeline Integration**: Multiple index generation strategies

### Testing and Validation
1. **CatalogSource Creation**: For cluster-based testing
2. **ImageDigestMirrorSet**: For pullspec mapping during testing
3. **Conforma Validation**: Image validation requirements

## Konflux User Scripts Integration

### Build Pipeline Script Execution
- **Purpose**: Run custom scripts before container image building
- **Use Case**: Modify source content or generate Containerfiles with external tools
- **Requirement**: Must use trusted artifacts (OCI-TA) pipeline variant

### Implementation Pattern
1. **Task Placement**: Add `run-script-oci-ta` task between `prefetch-dependencies` and `build-images`
2. **Configuration Parameters**:
   - `SCRIPT_RUNNER_IMAGE`: Container image with script dependencies
   - `SCRIPT`: Actual script to execute (command, path, or inline script)
   - `SOURCE_ARTIFACT`: Input source code artifact

### Configuration Example
```yaml
- name: run-script
  params:
    - name: SCRIPT_RUNNER_IMAGE
      value: quay.io/my-script-runner-image:latest@sha256:digest
    - name: SCRIPT
      value: ./my-script.sh build
```

### Required Modifications
- Update `build-images` task to use script task's output artifact
- Potentially modify `push-dockerfile` task if Containerfile is generated

### Key Constraints
- Only works with trusted artifacts pipeline
- Script and dependencies must be in source repository or container image
- Provides flexible, controlled script execution within build pipeline

### Bundle Manipulation Use Cases
- **Image Pinning**: Pin image references in bundle manifests during build
- **RelatedImages Generation**: Automatically populate `spec.relatedImages` from deployment specs
- **Reference Conversion**: Convert between development and production image patterns
- **Manifest Validation**: Validate bundle manifests before container build