# AGENTS.md — operator-sdk-builder

## Project Overview

This repository builds a multi-architecture container image (`operator-sdk-builder`) that bundles key Kubernetes Operator Framework tools for building OLM operators, bundles, and catalogs. It is part of the [Konflux](https://konflux-ci.dev/) CI/CD ecosystem.

The final image includes:
- **operator-sdk** — Operator Framework CLI
- **controller-gen** — Kubernetes API code generator
- **opm** — Operator Registry package manager
- **kustomize** — Kubernetes configuration management
- **envsubst** — GNU gettext environment variable substitution

## Repository Structure

```
.
├── Containerfile              # Multi-stage build (the core of this repo)
├── .gitmodules                # Git submodule definitions
├── operator-sdk/              # Submodule: operator-framework/operator-sdk
├── kustomize/                 # Submodule: kubernetes-sigs/kustomize
├── operator-registry/         # Submodule: operator-framework/operator-registry
├── controller-tools/          # Submodule: kubernetes-sigs/controller-tools
├── files/                     # Container config (policy.json, registry configs)
├── .tekton/                   # Tekton CI/CD pipelines
│   ├── build-pipeline.yaml
│   ├── operator-sdk-builder-push.yaml
│   └── operator-sdk-builder-pull-request.yaml
├── rpms.in.yaml               # RPM dependency specification
├── rpms.lock.yaml             # Locked RPM versions with checksums (all arches)
├── ubi.repo                   # UBI 9 YUM repository config
├── CODEOWNERS
├── LICENSE                    # Apache 2.0
└── README.md
```

## Build System

There is no Makefile. The project is entirely built via `Containerfile` using multi-stage builds:

1. **osdk-builder** — builds `operator-sdk` binary
2. **opm-builder** — builds `opm` binary
3. **kustomize-builder** — builds `kustomize` binary
4. **controller-gen-builder** — builds `controller-gen` binary
5. **Final stage** — copies all binaries into a UBI 9 Go toolset image

All binaries are built statically: `CGO_ENABLED=0 GOOS=linux`.

**Base image:** `registry.access.redhat.com/ubi9/go-toolset`

**Target architectures:** x86_64, aarch64, ppc64le, s390x

### Local Build

```bash
git submodule update --init --recursive
buildah bud -f Containerfile .
# or: podman build -f Containerfile .
```

## CI/CD (Tekton)

Pipelines live in `.tekton/`. There is no GitHub Actions or Makefile-based CI.

- **Push to main** → `operator-sdk-builder-push.yaml` — builds all 4 architectures, publishes to Quay.io, creates source image
- **Pull requests** → `operator-sdk-builder-pull-request.yaml` — builds x86_64 only, images expire after 5 days

All builds are hermetic (network-isolated) and use Konflux trusted artifact pipelines.

## Dependencies

- **Git submodules** — pinned by commit SHA in the repo's git tree (metadata in `.gitmodules`). Check current pins with `git submodule status`. Automated updates come from Konflux `mintmaker` branches.
- **RPMs** — specified in `rpms.in.yaml`, locked in `rpms.lock.yaml` with per-architecture checksums. Currently: `gettext`, `make`.
- **Base image** — UBI 9 Go toolset, version-pinned in `Containerfile`.

## Conventions

- No application code lives in this repo — it is purely a container image build definition.
- Changes typically involve: bumping submodule versions, updating the base image, modifying the Containerfile, or adjusting Tekton pipeline parameters.
- Dependency updates are largely automated via Konflux `mintmaker` and `konflux/references` branches.
- Container image labels follow OCI/OpenShift conventions.
- Signed image verification is enforced via `files/policy.json` for Red Hat registries.

## Common Tasks

| Task | How |
|---|---|
| Update a tool version | Update the git submodule pointer (e.g., `cd operator-sdk && git fetch && git checkout <tag>`) |
| Add an RPM dependency | Add to `rpms.in.yaml`, regenerate `rpms.lock.yaml`, add to `INSTALLED_RPMS` in Containerfile |
| Update base image | Change the `FROM` tag in all stages of `Containerfile` |
| Test a build locally | `git submodule update --init --recursive && podman build -f Containerfile .` |
| Modify CI pipelines | Edit YAML files in `.tekton/` |

## Code Owners

@arewm @nmars @yashvardhannanavati
