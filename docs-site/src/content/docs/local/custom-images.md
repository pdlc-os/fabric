---
title: Building Custom Images
description: Build and configure your own Fabric container images using Docker, Podman, GitHub Actions, or Google Cloud Build.
---

Fabric agents run inside container images that bundle an LLM harness (Claude, Gemini, etc.) with the Fabric toolchain. By default, Fabric uses pre-built images from the upstream registry. This guide shows how to build your own images and configure Fabric to use them.

## Why Build Custom Images?

- **Self-hosted registries**: Push images to a registry you control (GHCR, Artifact Registry, ECR, etc.).
- **Pinned versions**: Tag and version images to match your deployment lifecycle.
- **Custom modifications**: Add tools, certificates, or configurations to the base images.

## Image Hierarchy

Fabric images are built in layers:

```
core-base          System dependencies (Go, Node, Python, Git)
  └── fabric-base   Fabric CLI, fabrictool binary, fabric user, entrypoint
        ├── fabric-claude     Claude Code harness
        ├── fabric-gemini     Gemini CLI harness
        ├── fabric-opencode   OpenCode harness
        ├── fabric-codex      Codex harness
        └── fabric-hub        Fabric hub server
```

The `core-base` layer changes infrequently, but needs to be built at least once as it is a prerequisite for all other layers. Most rebuilds only need `fabric-base`, the harness layers, and the hub layer (the `common` build target).

### Non-Root Requirement

For security and compatibility across runtimes (especially Kubernetes), Fabric agents are required to run as a non-root user.

- **User**: The base images create a `fabric` user.
- **UID**: The user must have UID `1000`.
- **Permissions**: Ensure your custom images do not require root privileges at runtime and that any added files or directories are accessible by the `fabric` user. Home directory structure (`/home/fabric`) and environmental variables (`HOME`, `USER`, `LOGNAME`) are automatically injected by the runtime.

## How the Build Tooling Is Organized

A single orchestrator script — `image-build/scripts/build-images.sh` — owns the build DAG (which images depend on which, in what order, with which tags). The execution backend is selected with `--builder`. Three backends ship today:

| Builder | Backend | Multi-arch | Push behavior |
| :--- | :--- | :--- | :--- |
| `local-docker` (default) | `docker buildx` | yes (auto-promotes to `--push`) | honors `--push`; `--load` otherwise |
| `local-podman` | `podman build` | single-arch by default; multi-arch errors out (manual QEMU setup required) | honors `--push`; built images live in the local store automatically |
| `cloud-build` | `gcloud builds submit` against a static `cloudbuild-*.yaml` | always `linux/amd64` + `linux/arm64` (server-side) | always pushes |

The orchestrator computes tags, threads `BASE_IMAGE` between layers, and dispatches to the selected builder. Switching backends is purely a `--builder` flag change — target names and other flags are uniform.

## Quick Start

### Option 1: Local Docker Build

Build all images locally. Once `core-base` has been built, rebuilds can often use the default `common` build target.

```bash
# Build all layers locally without pushing — bare tags
# (fabric-claude:latest, etc.) land in your local docker engine.
image-build/scripts/build-images.sh --target all

# Or build and push to your registry
image-build/scripts/build-images.sh --registry ghcr.io/myorg --push --target all

# When pushing, configure Fabric to use them
fabric config set image_registry ghcr.io/myorg
```

`--registry` is optional for local-only builds. Omit it and the orchestrator tags images with bare names that stay in your local image store. Supply it (without `--push`) if you'd rather have fully-qualified tags locally — nothing leaves the machine until you `docker push` separately. `--registry` becomes required as soon as you pass `--push` or `--builder cloud-build`.

### Option 2: Local Podman Build

```bash
# Single-arch local build, no registry needed
image-build/scripts/build-images.sh --builder local-podman --target all

# Or push to a registry
image-build/scripts/build-images.sh \
  --builder local-podman \
  --registry quay.io/myorg \
  --push
```

Multi-arch Podman builds require manual QEMU `binfmt` setup. Until that is in place, passing `--platform linux/amd64,linux/arm64` to `local-podman` exits with an actionable error.

### Option 3: GitHub Actions (GHCR)

If your project is hosted on GitHub:

1. Fork the repo (or use it as a template).
2. Go to **Actions** > **Build Fabric Images** > **Run workflow**.
3. Enter `ghcr.io/<your-username>` as the registry.
4. Wait for the build to complete.
5. Configure Fabric:
   ```bash
   fabric config set image_registry ghcr.io/<your-username>
   ```

The workflow shells out to `build-images.sh --builder local-docker` after `docker/setup-buildx-action`, so it shares all the orchestration logic with local builds. It is also available as a reusable workflow via `workflow_call` for downstream repos.

### Option 4: Google Cloud Build

For GCP-based workflows:

```bash
# One-time setup: enable APIs, create Artifact Registry repo, grant permissions
image-build/scripts/setup-cloud-build.sh --project my-gcp-project

# Submit a build
image-build/scripts/build-images.sh \
  --builder cloud-build \
  --registry us-central1-docker.pkg.dev/my-gcp-project/fabric
```

Then point Fabric at the registry:

```bash
fabric config set image_registry us-central1-docker.pkg.dev/my-gcp-project/fabric
```

:::note[Legacy `trigger-cloudbuild.sh`]
The old `trigger-cloudbuild.sh` script is now a thin deprecation shim that forwards to `build-images.sh --builder cloud-build`. New workflows should call the orchestrator directly.
:::

## Configuring Fabric: `image_registry`

The `image_registry` setting tells Fabric to pull images from your registry instead of the upstream default. It rewrites the registry prefix of all standard harness images (those named `fabric-<harness>`) while preserving the image name and tag.

### How It Works

When `image_registry` is set, Fabric transforms the default image reference:

| Default Image | `image_registry` | Resolved Image |
| :--- | :--- | :--- |
| `us-central1-docker.pkg.dev/.../fabric-claude:latest` | `ghcr.io/myorg` | `ghcr.io/myorg/fabric-claude:latest` |
| `us-central1-docker.pkg.dev/.../fabric-gemini:latest` | `ghcr.io/myorg` | `ghcr.io/myorg/fabric-gemini:latest` |

### Setting It

**Globally** (applies to all projects):

```bash
fabric config set image_registry ghcr.io/myorg
```

Or edit `~/.fabric/settings.yaml` directly:

```yaml
schema_version: "1"
image_registry: "ghcr.io/myorg"
```

**Per-profile** (different registries for different environments):

```yaml
profiles:
  local:
    runtime: docker
    image_registry: "ghcr.io/myorg"
  staging:
    runtime: kubernetes
    image_registry: "us-central1-docker.pkg.dev/myproject/staging"
```

Profile-level `image_registry` takes precedence over the top-level setting.

### Override Precedence

The `image_registry` setting is the lowest-priority way to configure images. Explicit overrides always win:

1. **CLI `--image` flag** (highest priority)
2. **Template `fabric-agent.yaml`** image field
3. **Profile `harness_overrides`** image field
4. **`image_registry`** rewrite (lowest priority)

If any higher-priority override specifies a full image path, `image_registry` does not apply to that agent.

:::note
`image_registry` only rewrites images whose name starts with `fabric-`. Fully custom images (e.g., `mycompany/custom-agent:v2`) are never rewritten.
:::

## Build Script Reference

The `image-build/scripts/build-images.sh` orchestrator supports the following options:

| Flag | Description | Default |
| :--- | :--- | :--- |
| `--registry <path>` | Target registry path (e.g., `ghcr.io/myorg`). Required when `--push` is set or with `--builder cloud-build`. When omitted for a local-only build, images are tagged with bare names (e.g., `fabric-claude:latest`) and stay in the local store. | — |
| `--builder <name>` | Backend: `local-docker`, `local-podman`, or `cloud-build`. | `local-docker` |
| `--target <target>` | Build target (see below). | `common` |
| `--tag <tag>` | Mutable image tag. The `:<short-sha>` tag is always added when in a git repo. | `latest` |
| `--platform <plat>` | Target platform(s). Use `all` for `linux/amd64,linux/arm64`. Ignored by `cloud-build`. | builder's native arch |
| `--push` | Push images after building. Auto-enabled for multi-arch local builds. Ignored by `cloud-build` (always pushes). | build only |
| `--dry-run` | Print the resolved steps and the exact builder commands without executing. | off |

### Build Targets

Targets resolve to an ordered list of step IDs (one step per image):

| Target | What It Builds | Notes |
| :--- | :--- | :--- |
| `core-base` | `core-base` | Foundation tools layer. |
| `fabric-base` | `fabric-base` | Adds fabrictool. Reuses existing `core-base:<tag>`. |
| `harnesses` | `fabric-claude`, `fabric-gemini`, `fabric-opencode`, `fabric-codex` | Reuses existing `fabric-base:<tag>`. |
| `hub` | `fabric-hub` | Hub server image. Reuses existing `fabric-base:<tag>`. |
| `common` (default) | `fabric-base` + harnesses + hub | Skips `core-base`. Most common rebuild. |
| `all` | Full DAG | Rebuilds everything from `core-base`. |

### Tagging

Every image is tagged with both `:<tag>` (controlled by `--tag`, defaults to `latest`) and `:<short-sha>` (computed once from `git rev-parse --short HEAD`). When no SHA is available (e.g. running outside a git working tree), only the mutable tag is emitted.

When two steps in the same run depend on each other, the orchestrator threads `BASE_IMAGE=...:<short-sha>` so chained builds are immune to concurrent overwrites of `:latest`. Standalone targets (e.g. `--target harnesses` on its own) reference the parent image as `:<tag>`.

### Authentication

The orchestrator and builders assume the caller is already authenticated to the target registry (via `docker login`, `podman login`, `gcloud auth configure-docker`, etc.) and to any required cloud APIs. No login steps are performed inside the script.

### Examples

```bash
# Full rebuild for all platforms, pushed to GHCR
image-build/scripts/build-images.sh \
  --registry ghcr.io/myorg \
  --target all \
  --platform all \
  --push

# Build only harness images with a specific tag
image-build/scripts/build-images.sh \
  --registry ghcr.io/myorg \
  --target harnesses \
  --tag v1.2.0 \
  --push

# Local build for testing (no push, current architecture only, bare tags)
image-build/scripts/build-images.sh --target all

# Preview what would run, without executing anything
image-build/scripts/build-images.sh \
  --registry ghcr.io/myorg \
  --target all \
  --platform all \
  --dry-run

# Submit the same target DAG to Cloud Build
image-build/scripts/build-images.sh \
  --builder cloud-build \
  --registry us-central1-docker.pkg.dev/myproject/fabric \
  --target all
```

## GitHub Actions Workflow

The workflow at `.github/workflows/build-images.yml` can be used in two ways:

### Manual Trigger (`workflow_dispatch`)

Run it from the GitHub Actions UI with inputs for registry, target, tag, and platform.

### Reusable Workflow (`workflow_call`)

Call it from your own workflows in downstream repos:

```yaml
jobs:
  build-images:
    uses: google/fabric/.github/workflows/build-images.yml@main
    with:
      registry: ghcr.io/myorg
      target: common
      tag: latest
      platform: all
```

The workflow is a runner, not a builder — it shells out to `build-images.sh --builder local-docker` and shares the same Dockerfiles and orchestration as a local build.

## Google Cloud Build Configs

The `cloud-build` builder maps each `--target` to a static YAML file in `image-build/`:

| Target | Config file |
| :--- | :--- |
| `all` | `cloudbuild.yaml` |
| `common` | `cloudbuild-common.yaml` |
| `core-base` | `cloudbuild-core-base.yaml` |
| `fabric-base` | `cloudbuild-fabric-base.yaml` |
| `harnesses` | `cloudbuild-harnesses.yaml` |
| `hub` | `cloudbuild-hub.yaml` |

These YAMLs reference `$_TAG`, `$_SHORT_SHA`, `$_COMMIT_SHA`, and `$_REGISTRY` substitutions, all forwarded by the orchestrator. `_TAG` defaults to `latest` in each YAML's `substitutions:` block, preserving the prior behavior when `--tag` is omitted.

### Initial Setup

Run the one-time setup script to configure your GCP project:

```bash
image-build/scripts/setup-cloud-build.sh --project my-gcp-project
```

This script:
- Enables the Cloud Build and Artifact Registry APIs.
- Creates an Artifact Registry repository named `fabric`.
- Grants Cloud Build the necessary IAM permissions.
