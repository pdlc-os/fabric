# Container Builder Refactor

> **Status:** Draft for review. Open questions are flagged inline and consolidated at the end.

## Overview

The `image-build/` tooling currently has two parallel implementations of the same target DAG:

1. **`scripts/build-images.sh`** — orchestrates `docker buildx` invocations locally (and in GitHub Actions, which just shells out to this script).
2. **`cloudbuild*.yaml` + `scripts/trigger-cloudbuild.sh`** — encodes the same target DAG as Google Cloud Build steps, submitted with `gcloud builds submit`.

Both flows produce the same image hierarchy (`core-base → fabric-base → {claude, gemini, opencode, codex}`) and accept overlapping but inconsistent logical targets. The sequencing logic, build args, tagging conventions, and platform handling are duplicated across the bash script and the Cloud Build YAML files.

This proposal refactors the build tooling so that **target sequencing lives in one place** (the shell entry point) and the **execution backend** (local Docker, local Podman, Cloud Build, etc.) is selected via a `--builder` flag. Each builder is a small adapter responsible for "build one image with these inputs"; the script owns "which images to build, in what order, with which tags."

## Motivation

- **Local-first development.** Contributors on Linux/macOS without a GCP project should be able to `image-build/scripts/build-images.sh --builder local-docker` (or `local-podman`) without touching Cloud Build.
- **Single source of truth for target sequencing.** Today, adding a new harness or changing the build-arg wiring requires edits in two places (the bash script and ~3 cloudbuild YAML files) and they drift.
- **Pluggability for new backends.** Podman is already a first-class runtime in Fabric (see `.design/podman-runtime.md`); the build tooling should match. Future backends (Kaniko, buildah, GitHub Actions cache exporters, remote BuildKit) should be additive.
- **Same UX everywhere.** Whether running locally, in CI, or against Cloud Build, the same target names and flags apply.

## Non-Goals

- Replacing the Dockerfiles or the image hierarchy.
- Changing what gets published, where, or under what tags by default.
- Building a Go-based replacement for the shell script in this iteration (see Open Questions).
- Replacing `pull-containers.sh` or `setup-cloud-build.sh`; those serve different lifecycle stages.

## Current State (inventory)

```
image-build/
├── scripts/
│   ├── build-images.sh          # local docker buildx orchestration
│   ├── trigger-cloudbuild.sh    # GCB submission wrapper
│   ├── pull-containers.sh       # consumer-side image pull
│   └── setup-cloud-build.sh     # one-time GCP setup
├── cloudbuild.yaml              # GCB: full rebuild
├── cloudbuild-common.yaml       # GCB: fabric-base + harnesses
├── cloudbuild-core-base.yaml    # GCB: core-base only
├── cloudbuild-fabric-base.yaml   # GCB: fabric-base only
├── cloudbuild-harnesses.yaml    # GCB: harnesses only
├── core-base/Dockerfile
├── fabric-base/Dockerfile
├── claude/Dockerfile
├── gemini/Dockerfile
├── opencode/Dockerfile
└── codex/Dockerfile
```

Targets supported today: `common` (default), `all`, `core-base`, `harnesses` (both scripts); `fabric-base` (GCB only — missing from local script). No `hub` image exists yet.

`.github/workflows/build-images.yml` simply shells out to `build-images.sh` after `docker/setup-buildx-action`, so the GHA path already piggybacks on the local-docker builder. **GHA is a runner, not a builder.**

## Proposed Design

### Conceptual Model

```
┌─────────────────────────────────────────────────┐
│  build-images.sh                                │
│  ─────────────────                              │
│  1. Parse flags (--builder, --target, --tag,    │
│     --platform, --registry, --push)             │
│  2. Resolve target → ordered list of build      │
│     steps (core-base, fabric-base, fabric-*)      │
│  3. For each step, dispatch to selected         │
│     builder via a uniform contract              │
└─────────────────────────────────────────────────┘
                       │
       ┌───────────────┼─────────────────┐
       ▼               ▼                 ▼
  ┌─────────┐    ┌──────────┐      ┌────────────┐
  │ local-  │    │  local-  │      │   cloud-   │
  │ docker  │    │  podman  │      │   build    │
  └─────────┘    └──────────┘      └────────────┘
```

### Builder Contract

Each builder is a shell script under `image-build/scripts/builders/<name>.sh` that defines a fixed set of functions (sourced by the entry script). The orchestrator holds a hard-coded allow-list of builder names (`local-docker`, `local-podman`, `cloud-build`); `--builder <name>` validates against the list, then sources the corresponding file. Adding a new builder requires both a deliberate edit to the allow-list and a new file in `builders/`. The contract surface is intentionally small:

| Function | Responsibility |
|---|---|
| `builder_check` | Verify the backend is available (binary present, daemon reachable, project configured). Print actionable error, exit nonzero on failure. |
| `builder_prepare` | One-time setup before the first image (e.g. `docker buildx create`, `gcloud auth configure-docker`). May be a no-op. |
| `builder_build` | Build (and optionally push) **one image** given a normalized set of inputs. |
| `builder_finalize` | Optional cleanup or summary. May be a no-op. |

`builder_build` receives a uniform argument set:

```
builder_build \
  --image-name      <e.g. core-base>
  --context-dir     <absolute path>
  --dockerfile      <absolute path>
  --tags            <comma-separated, e.g. "registry/core-base:latest,registry/core-base:abc1234">
  --platforms       <comma-separated, e.g. "linux/amd64,linux/arm64">
  --build-arg       <KEY=VALUE>   # repeatable
  --push            <true|false>
  --load            <true|false>  # mutually exclusive with --push; the orchestrator never sets both true
```

The orchestrator in `build-images.sh` is responsible for computing all of these per step (resolving registry path, picking SHA-based tags, choosing the right context dir, threading `BASE_IMAGE` between layers). Builders never know the target DAG — they only know how to execute one build.

When the user requests a multi-platform build (`--platform` resolves to more than one architecture) without `--push`, the orchestrator auto-promotes to `--push` and prints a warning, matching today's `build-images.sh` behavior. Builders therefore never receive `--push false` together with a multi-arch `--platforms` value.

Some builders (e.g. `cloud-build`) handle an entire target in a single external submission rather than iterating per image. These builders declare `BUILDER_MODE=target` (vs. the default `BUILDER_MODE=per-image`). When the orchestrator detects `target` mode, it calls `builder_run_target <target> <registry> <tag> <push>` instead of looping through `builder_build` per step. Per-image builders do not implement `builder_run_target`.

### Target Resolution

A new (private) function `resolve_targets <target>` returns an ordered list of step IDs. Step IDs are the published image names — they appear in build output, dry-run plans, and registry inspection, so keeping them identical to the image they produce avoids a layer of indirection.

| Target | Steps | Notes |
|---|---|---|
| `core-base` | `core-base` | |
| `fabric-base` | `fabric-base` | Was missing from local script — now parity with GCB |
| `harnesses` | `fabric-claude`, `fabric-gemini`, `fabric-opencode`, `fabric-codex` | |
| `hub` | `fabric-hub` | New image; extends `fabric-base` |
| `common` | `fabric-base`, all harnesses, `fabric-hub` | Default rebuild |
| `all` | `core-base`, `fabric-base`, all harnesses, `fabric-hub` | Full rebuild |

Each step ID maps to a step descriptor (image name, dockerfile path, context dir, build-args function). This descriptor table replaces the per-target inline functions (`build_core_base`, `build_fabric_base`, `build_harness`) in today's script.

### Step Descriptors and Inter-step Dependencies

A step descriptor is a small record per image:

| Field | Example (for `fabric-claude`) |
|---|---|
| `image_name` | `fabric-claude` (also the step ID) |
| `dockerfile` | `image-build/claude/Dockerfile` |
| `context_dir` | `image-build/claude` |
| `build_args_fn` | `step_build_args_fabric_claude` |

`build_args_fn` is a shell function that emits one `KEY=VALUE` line per build-arg on stdout. It receives the orchestrator's resolved state via environment (`REGISTRY`, `TAG`, `SHORT_SHA`, `BASE_TAG`) and is responsible for producing per-image args like `BASE_IMAGE=...` and `GIT_COMMIT=...`. The orchestrator collects the lines and threads each one through `builder_build --build-arg`.

**BASE_IMAGE threading.** When step N depends on an image produced by step N-1 in the same run, the orchestrator references it by `:<short-sha>` (matching today's GCB behavior — reproducible, immune to concurrent overwrites of `:latest`). When SHA is unavailable, it falls back to `:<tag>`.

**Standalone-target base resolution.** When a step's base image is not built in the current run (e.g. `--target fabric-base` or `--target harnesses`), `BASE_TAG` resolves to `<tag>` (default `latest`). Concretely:

| Target | Step | BASE_IMAGE resolves to |
|---|---|---|
| `fabric-base` (standalone) | `fabric-base` | `<registry>/core-base:<tag>` |
| `harnesses` (standalone) | `fabric-*` | `<registry>/fabric-base:<tag>` |
| `hub` (standalone) | `fabric-hub` | `<registry>/fabric-base:<tag>` |
| `common`, `all` | (chained) | `<registry>/<base>:<short-sha>` (built earlier in same run) |

### Builder Implementations

#### `local-docker`

Wraps `docker buildx build`. Equivalent to today's `build-images.sh` body. Handles the `--load` vs `--push` mutual exclusion, ensures a `fabric-builder` buildx instance exists, bootstraps it.

#### `local-podman`

Wraps `podman build`. Builds native arch only by default. Rejects multi-arch `--platform` values with an actionable error (QEMU binfmt setup required, explicit opt-in needed). Single-platform cross-arch (e.g. `--platform linux/amd64` on an arm64 host) is allowed — Podman handles this via QEMU if available, and fails naturally if not.

#### `cloud-build`

Operates in `BUILDER_MODE=target`. `builder_run_target` maps the target name to the existing static `cloudbuild-*.yaml` file and delegates to `gcloud builds submit`. The static YAML files are **retained** as part of the `cloud-build` builder's implementation — they are not generated at runtime.

The builder forwards the orchestrator's `--registry`, `--tag`, and SHA into the submission as substitutions: `_REGISTRY`, `_TAG`, `_SHORT_SHA`, `_COMMIT_SHA`. The existing YAMLs hardcode `:latest` and must be updated to reference `$_TAG` so `--tag` is honored uniformly across builders. `_TAG` defaults to `latest` in each YAML's `substitutions:` block, preserving today's behavior when the flag is omitted.

`--platform` and `--push` are ignored by `cloud-build`: the YAMLs hardcode `linux/amd64,linux/arm64` and always push (these are server-side concerns that the Cloud Build steps own — see also Q4 on per-builder caching). Passing either flag with `--builder cloud-build` is a no-op; the `--help` output notes this.

Target → config mapping:

| Target | Config file | Notes |
|---|---|---|
| `common` | `cloudbuild-common.yaml` | Updated: include hub step; reference `$_TAG` |
| `all` | `cloudbuild.yaml` | Updated: include hub step; reference `$_TAG` |
| `core-base` | `cloudbuild-core-base.yaml` | Updated: reference `$_TAG` |
| `fabric-base` | `cloudbuild-fabric-base.yaml` | Updated: reference `$_TAG` |
| `harnesses` | `cloudbuild-harnesses.yaml` | Updated: reference `$_TAG` |
| `hub` | `cloudbuild-hub.yaml` | New file |

`trigger-cloudbuild.sh` is replaced by a thin deprecation shim that forwards to `build-images.sh --builder cloud-build`.

### Per-Builder Flag Behavior

| Flag | `local-docker` | `local-podman` | `cloud-build` |
|---|---|---|---|
| `--registry` | honored | honored | forwarded as `_REGISTRY` substitution |
| `--target` | honored | honored | honored (selects YAML file) |
| `--tag` | honored (mutable tag) | honored (mutable tag) | forwarded as `_TAG` substitution |
| `--platform` | honored | honored, single-arch only by default (errors on multi-arch unless explicitly opted in) | **ignored** (YAMLs hardcode amd64+arm64) |
| `--push` | honored; auto-promoted to `--push` for multi-arch builds | honored | **ignored** (YAMLs always push) |
| `--dry-run` | honored | honored | honored (prints `gcloud builds submit` command + YAML path) |

### Files Added

```
image-build/scripts/
├── build-images.sh                  # rewritten orchestrator (keeps name)
├── builders/
│   ├── local-docker.sh
│   ├── local-podman.sh
│   └── cloud-build.sh
└── lib/
    └── targets.sh                   # target → step list, step descriptors

image-build/
├── hub/
│   └── Dockerfile                   # produces image `fabric-hub` (mirrors harness naming)
└── cloudbuild-hub.yaml              # new GCB config for hub target
```

`trigger-cloudbuild.sh` is removed (its job is now `build-images.sh --builder cloud-build`). A tiny shim that prints a deprecation notice and forwards may be retained for one release.

### Backward-Compatible CLI

```bash
# Old (still works, defaults to local-docker):
image-build/scripts/build-images.sh --registry ghcr.io/myorg --target common --push

# New explicit builder selection:
image-build/scripts/build-images.sh --builder local-podman --registry quay.io/myorg --target common --push
image-build/scripts/build-images.sh --builder cloud-build --registry us-central1-docker.pkg.dev/myproj/fabric --target all

# Dry-run: see what would be built/submitted without executing:
image-build/scripts/build-images.sh --builder cloud-build --registry us-central1-docker.pkg.dev/myproj/fabric --target all --dry-run
```

`--builder` defaults to `local-docker` to preserve current behavior. The GitHub Actions workflow keeps calling `build-images.sh` unchanged.

### Tagging

The orchestrator always emits two tags per image: `:<tag>` (controlled by `--tag`, defaults to `latest`) and `:<short-sha>` (computed once from `git rev-parse --short HEAD`). If the working directory is not a git repo or the SHA is unavailable, only `:<tag>` is emitted. `--tag` controls the mutable tag only; the SHA tag is always added when available and cannot be suppressed.

This is a **behavior change for the local-docker path** — today's `build-images.sh` emits only `:<tag>`. The new policy aligns local with what GCB already does (today's cloudbuild YAMLs emit both `:latest` and `:_SHORT_SHA`).

For `cloud-build`, the orchestrator forwards `--tag` into the submission as the `_TAG` substitution; the YAMLs reference `$_TAG` for the mutable tag and `$_SHORT_SHA` for the SHA tag. See §Builder Implementations → `cloud-build`.

### Authentication

Builders assume the caller is already authenticated to the target registry and any required cloud APIs. No login steps are performed by the script. Prerequisites are documented in the README per builder.

## Migration Plan

1. Land the new orchestrator + `local-docker` builder behind `--builder local-docker` (default). Behavior matches today's `build-images.sh` for all existing flag combinations, with the addition of the SHA tag (see §Tagging).
2. Add `local-podman` builder. Hand-test on Linux + macOS (via `podman machine`).
3. Add `cloud-build` builder. Update existing `cloudbuild-*.yaml` files to reference `$_TAG` (default `latest`) for the mutable tag, so `--tag` flows through. Cut over `trigger-cloudbuild.sh` to a thin deprecation shim. Validate against the existing GCP project that produces `us-central1-docker.pkg.dev/.../public-docker`.
4. Add `image-build/hub/Dockerfile` (producing image `fabric-hub`, mirroring the harness naming pattern) and `cloudbuild-hub.yaml`. Wire `fabric-hub` and `fabric-base` as standalone targets across all builders. Update `common` and `all` cloudbuild configs to include the hub step.
5. Update `image-build/README.md`.

Each step is independently shippable.

## Risks

- **Shell complexity.** Stitching together a strategy pattern in bash gets unwieldy past 3–4 builders. If the spec grows much beyond what's described here, consider Open Question 9 (graduate to Go).
- **Cloud Build config drift.** The static `cloudbuild-*.yaml` files and the orchestrator's target table must stay in sync as images are added. The orchestrator's target table is the source of truth; the `cloud-build` builder's target→file map fails loudly if a target listed there has no corresponding YAML. Adding a new image therefore requires touching both, and missing one will surface immediately rather than silently.
- **Podman multi-arch parity.** If Podman's qemu emulation has practical issues building our images, `local-podman` may be effectively single-arch. Document the limitation.

## Open Questions

1. **Cloud Build orchestration model.** ✅ *Resolved.* Retain the static `cloudbuild-*.yaml` files as part of the `cloud-build` builder implementation. `builder_run_target` selects and submits the right file. No YAML generation at runtime.
2. **Local-script `fabric-base` target.** ✅ *Resolved.* Confirmed gap — `fabric-base` is added as a standalone target across all builders.
3. **Podman multi-arch.** ✅ *Resolved.* `local-podman` builds native arch only by default. If `--platform` specifies multiple architectures, the builder exits with an error explaining that multi-arch Podman builds require manual QEMU binfmt setup and must be opted into explicitly (e.g. `--platform linux/amd64,linux/arm64`). Future improvement: downgrade to a warning and proceed (option B), once multi-arch Podman is validated on supported platforms.
4. **Caching strategy.** ✅ *Resolved.* Each builder owns its caching entirely. The orchestrator passes no cache-related hints. The cloud-build builder's split build/push trick for `core-base` stays baked into the static YAML files.
5. **Authentication scope.** ✅ *Resolved.* Builders assume the caller is already authenticated. No login steps in `builder_check`, `builder_prepare`, or anywhere in the script. Prerequisites are documented in the README.
6. **GitHub Actions as a builder?** ✅ *Resolved.* GHA remains a runner, not a builder — it calls `build-images.sh --builder local-docker` and shares all Dockerfiles. No `--builder github-actions` mode. The entrypoint script's `--help` output will mention `gh workflow run build-images.yml` as the path for triggering GHA-based builds remotely.
7. **Future builders.** ✅ *Resolved.* Deferred. Contract stays path-based. Remote-context builders (Kaniko, remote BuildKit) would require a contract extension; `BUILDER_MODE=remote` is the natural future extension point when there's a concrete need.
8. **Naming.** ✅ *Resolved.* `--builder`. The buildx instance name is an internal implementation detail of `local-docker`, not a UX concern.
9. **Shell vs Go.** ✅ *Resolved.* Stay in bash. A Go-based `fabric image build` subcommand remains a future option if the orchestration outgrows what bash handles well.
10. **Builder discovery.** ✅ *Resolved.* Hard-coded allow-list in the orchestrator; the named file under `builders/` is sourced when the name validates. Predictable `--help` output; adding a builder requires both an allow-list edit and a new file.
11. **Tag policy.** ✅ *Resolved.* Always emit both `:<tag>` (default `latest`) and `:<short-sha>` for every image. The orchestrator computes the short SHA once and threads it through all steps. Falls back to `latest`-only if the working directory is not a git repo or the SHA is unavailable. For `cloud-build`, `--tag` is forwarded as the `_TAG` substitution; YAMLs reference `$_TAG` and `$_SHORT_SHA`.
12. **Static cloudbuild YAMLs.** ✅ *Resolved.* Retained as part of the `cloud-build` builder — not generated, not deleted. They live alongside the builder script in `image-build/`. Updated to reference `$_TAG` so `--tag` flows through uniformly.
13. **`pull-containers.sh` and `setup-cloud-build.sh`.** ✅ *Resolved.* Left unchanged. They serve distinct lifecycle stages and have no overlap with the builder abstraction.
14. **Dry-run / plan mode.** ✅ *Resolved.* Include `--dry-run`. For per-image builders, prints the resolved step list and the exact `docker`/`podman` command the builder would execute for each step. For target-mode builders (`cloud-build`), prints the resolved target, the YAML file path, and the `gcloud builds submit` invocation (with substitutions). Implemented in the orchestrator before the execution loop; each builder script prints its command rather than running it when `DRY_RUN=true`.
15. **Concurrency.** ✅ *Resolved.* Deferred. Harnesses build serially. A `--jobs N` flag can be added later without contract changes if build times become a pain point.
16. **Hub image base.** ✅ *Resolved.* `hub/Dockerfile` (image `fabric-hub`) extends `fabric-base`, consistent with all harness images. A dedicated server base image is a future option but not in scope here.
17. **Hub in `common` target.** ✅ *Resolved.* `fabric-hub` is included in `common`. It is a first-class image alongside the harnesses and rebuilds with the default target.
18. **Hub build context and binary.** ✅ *Resolved.* Answered by Q16 — hub extends `fabric-base`, so build context follows the same pattern as all other images in the hierarchy. No special handling required.

## Appendix: Mapping Today → Tomorrow

| Today | Tomorrow |
|---|---|
| `build-images.sh --target common` | `build-images.sh --builder local-docker --target common` (default builder) |
| `trigger-cloudbuild.sh common` | `build-images.sh --builder cloud-build --target common` |
| `cloudbuild-common.yaml` | Retained, updated to include hub step and reference `$_TAG` |
| GHA `build-images.yml` calling `build-images.sh` | Unchanged |
| `pull-containers.sh` | Unchanged |
| `setup-cloud-build.sh` | Unchanged |
