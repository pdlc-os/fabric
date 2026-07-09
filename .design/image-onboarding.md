# Image Onboarding: User-Built Container Images

## Problem Statement

Fabric's container images were originally published to a project-specific GCP Artifact Registry. The registry path was hardcoded into every harness-config's `config.yaml` and the `image-build/` build scripts. New users who wanted to run Fabric had to either:

1. Use the pre-built images from the upstream registry (which may not always be public or up-to-date), or
2. Manually edit multiple harness-config files and build scripts to point to their own registry.

Neither path is smooth for onboarding. We need a streamlined way for users to:
- Build their own set of images to a registry they control.
- Have those images automatically used by the fabric CLI without manual per-harness editing.

## Current State

### Image Hierarchy
```
core-base  (Go, Git, Node, Python, gcloud — rarely changes)
  └── fabric-base  (fabric/fabrictool binaries, fabric user, entrypoint)
        ├── fabric-claude
        ├── fabric-gemini
        ├── fabric-opencode
        └── fabric-codex
```

### Where Images Are Referenced
| Location | Example | Count |
|----------|---------|-------|
| `pkg/harness/claude/embeds/config.yaml` | `image: fabric-claude:latest` | 4 files (one per harness) |
| `image-build/cloudbuild*.yaml` | `$_REGISTRY` substitution variable | 5 files |
| `image-build/scripts/trigger-cloudbuild.sh` | `--project` / `--registry` flags | 1 file |
| `image-build/scripts/pull-containers.sh` | `--registry` flag | 1 file |

### Image Resolution at Runtime
The image used for an agent is resolved through a precedence chain:
1. **Harness-config `config.yaml`** → base image (embedded default)
2. **Settings `harness_configs` map** → can override image per harness-config name
3. **Profile `harness_overrides`** → can override image per profile+harness-config
4. **Template `fabric-agent.yaml`** → can specify image
5. **CLI `--image` flag** → highest priority

Key insight: items 2 and 3 already support image overrides in `settings.yaml`, but there is no mechanism to set a **registry prefix** that applies to all harness-configs at once. Users must override each harness-config's image individually.

---

## Proposed Approach: `image_registry` Setting + Build Script

### Design Principles
- **One setting, all images**: A single `image_registry` field in settings replaces the registry prefix for all harness-config images.
- **Zero harness-config edits**: Users never need to touch individual harness-config files.
- **Build once, configure once**: A single script builds all images and prints/sets the registry path.
- **Two build paths**: Google Cloud Build and GitHub Actions, with the same Dockerfiles.

### 1. New Setting: `image_registry`

Add a top-level `image_registry` field to `VersionedSettings`:

```yaml
# ~/.fabric/settings.yaml
schema_version: "1"
image_registry: "ghcr.io/myorg"   # <-- NEW
active_profile: local
default_harness_config: claude
```

**Semantics:**
- When `image_registry` is set, it replaces the registry prefix portion of any harness-config image at resolution time.
- The image name suffix (e.g., `fabric-claude:latest`) is preserved.
- Only applies to images that follow the fabric naming convention (`fabric-<harness>:<tag>`). Custom images with explicit full paths are not rewritten.

**Resolution logic** (in `ResolveHarnessConfig` / image resolution):
```
embedded default:    fabric-claude:latest
image_registry set:  ghcr.io/myorg
resolved image:      ghcr.io/myorg/fabric-claude:latest
```

The rewrite extracts the image basename (`fabric-claude:latest`) from whatever full path is in the harness-config and prepends the user's `image_registry` value.

**Override precedence:**
- `image_registry` applies as a transform on the harness-config's default image.
- An explicit `image` in a profile `harness_overrides`, template, or `--image` flag takes full precedence (no rewrite).
- This means `image_registry` is the lowest-priority way to set images, but it's the most convenient for the common case.

**Profile-level override:**
`image_registry` can also be set per-profile for users who use different registries in different environments:

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

### 2. Build Script: `image-build/scripts/build-images.sh`

All image-related scripts live under `image-build/scripts/`, consolidating everything image-related in the `image-build/` directory. The exception is GitHub Actions workflows, which remain in `.github/workflows/` per GitHub convention (documented in the `image-build/README.md`).

A unified build script that works for local Docker builds and can be adapted for CI:

```
image-build/scripts/build-images.sh --registry <registry-path> [--target common|all|core-base|harnesses] [--push]
```

**Behavior:**
- `--registry` (required): The target registry path (e.g., `ghcr.io/myorg`, `us-docker.pkg.dev/myproject/fabric`)
- `--target`: Which images to build (default: `common` = fabric-base + harnesses)
- `--push`: Push images after building (default: build only)
- `--platform`: Target platform(s) (default: current architecture; `all` for `linux/amd64,linux/arm64`)
- `--tag`: Image tag (default: `latest`)

**Post-build output:**
```
Images built successfully!

To configure fabric to use these images, run:
  fabric config set image_registry ghcr.io/myorg

Or add to your ~/.fabric/settings.yaml:
  image_registry: "ghcr.io/myorg"
```

### 3. Cloud Build Path (GCP)

Retain the existing `image-build/cloudbuild*.yaml` files, but make them parameterizable for any GCP project:

**Changes to `image-build/scripts/trigger-cloudbuild.sh`** (moved from `hack/`)**:**
- Accept `--project` and `--registry` flags instead of hardcoding a specific project.
- Default to `$GCLOUD_PROJECT` or `$(gcloud config get-value project)` if not specified.
- Default registry to `us-central1-docker.pkg.dev/${PROJECT}/fabric` (a conventional repo name).

**New: `image-build/scripts/setup-cloud-build.sh`:**
A one-time setup script that:
1. Creates the Artifact Registry repository if it doesn't exist.
2. Grants Cloud Build the necessary permissions.
3. Prints the registry path for the user to configure.

### 4. GitHub Actions Path

Add a reusable GitHub Actions workflow at `.github/workflows/build-images.yml`:

```yaml
name: Build Fabric Images
on:
  workflow_dispatch:
    inputs:
      target:
        description: 'Build target (common, all, core-base, harnesses)'
        default: 'common'
      registry:
        description: 'Container registry (e.g., ghcr.io/myorg)'
        required: true

jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      packages: write
      contents: read
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - name: Build and push images
        run: |
          image-build/scripts/build-images.sh \
            --registry "${{ inputs.registry }}" \
            --target "${{ inputs.target }}" \
            --platform all \
            --push
```

For users on GitHub, the onboarding experience is:
1. Fork the repo.
2. Go to Actions → "Build Fabric Images" → Run workflow.
3. Enter `ghcr.io/<your-username>` as the registry.
4. Run `fabric config set image_registry ghcr.io/<your-username>`.

### 5. CLI Integration: `fabric config set`

The existing `fabric config` command should support setting `image_registry`:

```bash
fabric config set image_registry ghcr.io/myorg
```

This writes to `~/.fabric/settings.yaml` (global) or `.fabric/settings.yaml` (grove-level with `--grove`).

---

## Implementation Plan

### Phase 1: `image_registry` Setting (Core) ✅ DONE

1. **Add `ImageRegistry` field to `VersionedSettings`** and `V1ProfileConfig`.
   - File: `pkg/config/settings_v1.go`
   - Add `ImageRegistry string` with appropriate tags.

2. **Implement image rewrite logic.**
   - File: `pkg/config/settings_v1.go` (in `ResolveHarnessConfig`)
   - When `image_registry` is set and the resolved image has no explicit override, rewrite the registry prefix.
   - Helper: `RewriteImageRegistry(fullImage, newRegistry) string` — extracts basename+tag, prepends new registry.

3. **Wire into provisioning.**
   - File: `pkg/agent/provision.go`
   - After harness-config image is resolved, apply `image_registry` rewrite if no higher-priority override exists.

4. **Add `fabric config set` support** for the new field.
   - File: `cmd/config.go` (or wherever config set lives)

5. **Add tests.**
   - Unit tests for `RewriteImageRegistry`.
   - Integration test for image resolution with `image_registry` set.

### Phase 2: Build Scripts ✅ DONE

6. **Create `image-build/scripts/build-images.sh`.** ✅
   - Unified local build script using `docker buildx`.
   - Parameterized by `--registry`, `--target`, `--push`, `--platform`, `--tag`.

7. **Move and update `hack/trigger-cloudbuild.sh` → `image-build/scripts/trigger-cloudbuild.sh`.** ✅
   - Accept `--project` and `--registry` flags.
   - Default to environment/gcloud for project.

7b. **Move `hack/pull-containers.sh` → `image-build/scripts/pull-containers.sh`.** ✅
   - Consolidates all image-related scripts in one place.
   - Parameterized with `--registry` and `--tag` flags.

8. **Create `image-build/scripts/setup-cloud-build.sh`.** ✅
   - One-time GCP Artifact Registry setup (APIs, repo, IAM).

### Phase 3: GitHub Actions ✅ DONE

9. **Add `.github/workflows/build-images.yml`.** ✅
   - Reusable workflow for GHCR builds.
   - `workflow_dispatch` for manual triggers.
   - `workflow_call` for use as a reusable workflow in forks.

### Phase 4: Documentation ✅ DONE

10. **Add onboarding guide** to project docs/README covering both paths. ✅
    - Added `docs-site/src/content/docs/guides/custom-images.md` covering Docker, GitHub Actions, and Cloud Build paths.
    - Added `image_registry` to orchestrator-settings reference (top-level and profile-level).
    - Registered in sidebar under Developer Guide > How To.

---

## Alternatives Considered

### A. Per-harness image overrides only (no `image_registry`)

Users would set each harness image individually in settings:

```yaml
profiles:
  local:
    harness_overrides:
      claude:
        image: ghcr.io/myorg/fabric-claude:latest
      gemini:
        image: ghcr.io/myorg/fabric-gemini:latest
```

**Rejected because:** Tedious, error-prone (easy to miss one), and doesn't scale as harnesses are added. The `image_registry` approach is strictly better for the common case.

### B. Registry in harness-config `config.yaml` as a variable

Replace the hardcoded registry in embedded `config.yaml` with a `${FABRIC_IMAGE_REGISTRY}` variable:

```yaml
image: ${FABRIC_IMAGE_REGISTRY}/fabric-claude:latest
```

**Rejected because:** Introduces variable expansion into YAML config parsing, adds complexity, and is less discoverable than a first-class setting. Also breaks if the env var isn't set.

### C. `fabric images build` subcommand

A dedicated fabric CLI command to build images.

**Deferred:** Could be added later, but a shell script is simpler to start with, easier to debug, and doesn't couple image building to the fabric binary. The script approach also works for CI without installing fabric.

---

## Edge Cases

- **Custom images**: If a harness-config or template specifies a non-fabric image (e.g., a fully custom Docker image), `image_registry` should NOT rewrite it. Detection: only rewrite images whose basename starts with `fabric-`.
- **Tag preservation**: `image_registry` only replaces the registry prefix. Tags are preserved from the original image reference.
- **Empty `image_registry`**: No rewrite occurs; the embedded default images are used as-is (current behavior).
- **Hub/broker scenarios**: The `image_registry` setting on the broker's settings should be respected when the broker resolves images for hub-dispatched agents.
