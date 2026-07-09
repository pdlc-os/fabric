# Kubernetes Runtime: Launch-Ready Improvements

## Purpose
This document reviews the **current Kubernetes runtime implementation in code** and proposes a staged plan to mature it toward launch readiness, with priority on parity with local runtimes (Docker, Apple container, Podman).

Scope of review is based on current implementation in:
- `pkg/runtime/k8s_runtime.go`
- `pkg/runtime/factory.go`
- `pkg/runtime/common.go`
- `pkg/runtime/docker.go`
- `pkg/runtime/podman.go`
- `pkg/runtime/apple_container.go`
- `pkg/agent/run.go`
- Kubernetes runtime tests in `pkg/runtime/k8s_*_test.go`

## Executive Summary
The Kubernetes runtime is functional for core agent lifecycle operations (run/list/logs/attach/sync/delete), has meaningful secret support (native Secret and GKE Secret Store CSI paths), and includes GCS volume support for GKE CSI.

However, compared to local runtimes, there are major parity and production-readiness gaps:
1. **Config parity gaps**: Kubernetes-specific config fields are partially ignored (`context`, `runtimeClassName`, and effectively `kubernetes.namespace` at agent level).
2. **Env/auth parity gaps**: Kubernetes does not reuse `buildCommonRunArgs`, so it misses harness env/telemetry env behavior and has divergent auth handling.
3. **Volume parity gaps**: non-GCS volume mounts from config are not implemented on Kubernetes.
4. **Namespace correctness gaps**: operations like delete/logs/attach/exec default to one namespace and can miss agents started elsewhere.
5. **Security/portability issues**: `ResolvedAuth` files use `hostPath` mounts, which are not portable to remote clusters and often disallowed.
6. **Operational maturity gaps**: limited retry/error classification, no structured diagnostics per pod lifecycle phase, and minimal e2e verification coverage.

## What Is Implemented Today

### Runtime selection and base config
- Runtime factory supports `kubernetes`/`k8s`, sets defaults (`DefaultNamespace`, `SyncMode=tar`), and reads runtime-level namespace/sync/GKE mode from settings.
- `context` is recognized in settings but not actually applied to client selection.

### Pod launch flow
- `Run()` creates pod, waits for ready state with status polling, and then performs initial home/workspace sync.
- Workspace and home paths are persisted in pod annotations for later sync operations.
- Tmux command wrapping is implemented in pod command construction.

### Secrets
- Fallback path: creates native Kubernetes Secret and injects environment/file/variable secrets.
- GKE path: creates SecretProviderClass and mounts CSI volume, with env refs from synced K8s Secret.
- Secret cleanup on delete is implemented (Secrets + SecretProviderClass when enabled).

### Sync modes
- `tar` snapshot sync to/from pod is implemented over `pods/exec` streaming.
- GCS volume sync metadata (`fabric.gcs_volumes`) is supported for `sync to/from`.
### Runtime operations
- `List`, `GetLogs`, `Attach`, `Exec`, `Delete`, `Stop`, `GetWorkspacePath` exist and are wired.
- Attach includes terminal raw mode and resize support.

### Kubernetes resources
- Supports generic resource requests/limits from `ResourceSpec` and overlays Kubernetes-specific extended resources.
- Supports `ServiceAccountName`.

## Parity Comparison vs Local Runtimes

## 1) Environment and harness parity: **Partial / Divergent**
Local runtimes rely on `buildCommonRunArgs()` which applies:
- harness env (`Harness.GetEnv`)
- telemetry env (`Harness.GetTelemetryEnv` when enabled)
- resolved auth env/files
- common mount/env conventions (including optional local gcloud config)

Kubernetes runtime builds pod env independently and currently only ingests:
- `config.Env`
- secret-derived env
- some auth env (only in `ResolvedAuth` branch)
- host UID/GID env

Result: runtime behavior differs by backend for env/auth composition.

## 2) Auth parity: **Incomplete and risky**
- Kubernetes has `if len(ResolvedSecrets) > 0 { ... } else if ResolvedAuth != nil { ... }` behavior, so `ResolvedAuth` file/env injection is skipped whenever secrets are present.
- `ResolvedAuth.Files` use `hostPath` mounts. That is not a reliable mechanism for remote brokers/clusters and is often blocked by policy.

## 3) Volume parity: **Missing for non-GCS volumes**
- Local runtimes support generic local volume mounts.
- Kubernetes runtime only processes GCS volumes; non-GCS configured volumes are effectively ignored.

## 4) Namespace parity/correctness: **Weak**
- Namespace can be selected at run time from labels/settings, but several operations (`Delete`, `GetLogs`, `Attach`, `Exec`) default to `DefaultNamespace` without robust per-agent namespace resolution.
- `List` is single-namespace only.

This can cause lifecycle failures for agents created outside default namespace.

## 5) Kubernetes config parity: **Partial**
- `serviceAccountName` and resource overlays are used.
- `runtimeClassName` exists in API but is not applied to pod spec.
- runtime `context` is read but not applied to k8s client.
- agent-level `kubernetes.namespace` is not clearly honored in runtime namespace resolution path.

## 6) Local workspace parity: **Different model, acceptable but underdeveloped**
- Local runtimes use direct bind mounts for live local filesystem semantics.
- Kubernetes uses `EmptyDir` + tar sync, which is expected for remote clusters.
- Current sync model lacks stronger resumability/incrementality/observability expected for production parity.

## 7) Operational hardening parity: **Below local runtimes**
- Image handling is effectively a stub (`ImageExists=true`, `PullImage=nil`).
- Limited structured error taxonomy around scheduling/image/permission/network failures.
- BuildPod uses `resource.MustParse` which can panic on invalid values instead of returning user-friendly errors.

## 8) Test maturity: **Good unit coverage in key areas, limited integration/e2e**
Current tests cover:
- pod env basics
- tmux command wrapping
- annotation behavior
- secret creation/injection paths including GKE SPC

Gaps:
- namespace behavior across lifecycle operations
- auth file handling portability/security
- non-GCS volume behavior
- runtimeClassName/context wiring
- sync failure/retry edge cases
- multi-namespace listing/management

## Launch Risks (Current)
1. Agents in non-default namespaces can become hard to manage (`logs/attach/delete/exec` mismatch).
2. Remote cluster deployments may fail due to `hostPath` auth file assumptions.
3. Users can configure Kubernetes fields that are silently ignored, causing drift between config intent and runtime behavior.
4. Divergent env/auth semantics across runtimes create hard-to-debug behavior differences.
5. Invalid resource strings can crash runtime path due to `MustParse`.

## Staged Improvement Plan

## Stage 0: Correctness and Safety Baseline (Immediate)
Goal: remove sharp edges and stop silent misconfiguration.

Deliverables:
1. Namespace resolution hardening
- Persist namespace annotation on pod at create time (source of truth).
- Resolve namespace per-agent in `Delete`, `GetLogs`, `Attach`, `Exec`, `Sync`.
- Optional: allow `namespace/pod` ID format consistently across APIs.

2. Config wiring fixes
- Apply `kubernetes.runtimeClassName` to `pod.Spec.RuntimeClassName`.
- Implement runtime `context` selection in client construction (or fail clearly if unsupported).
- Honor agent-level `kubernetes.namespace` with clear precedence order:
  1) agent config `kubernetes.namespace`
  2) explicit runtime label override
  3) runtime default namespace from settings

3. Safe resource parsing
- Replace `resource.MustParse` with parse+validation returning actionable errors.

4. Auth correctness
- Remove `ResolvedSecrets` vs `ResolvedAuth` mutual exclusion; both pipelines should compose.

5. Compatibility guardrails
- If unsupported features are configured (until implemented), fail fast with explicit error rather than silently ignoring.

Acceptance checks:
- Unit tests for namespace-aware lifecycle operations.
- Tests validating runtimeClassName and context behavior.
- Tests ensuring invalid resource strings return errors (no panic).

## Stage 1: Runtime Parity Foundation (Near Term) — COMPLETED

Goal: align Kubernetes behavior with local runtimes where functionally equivalent.

Deliverables:
1. Common env/auth composition parity — **Done**
- `buildPod` now calls `Harness.GetEnv()` and `Harness.GetTelemetryEnv()` matching `buildCommonRunArgs` behavior.
- `ResolvedAuth` and `ResolvedSecrets` now compose independently (removed `else if` mutual exclusion).
- Auth files use K8s Secret volumes instead of `hostPath` for portability and security.

2. Non-GCS volume strategy — **Done**
- Local/bind-mount volumes emit structured `slog.Warn` instead of being silently ignored.
- GCS CSI path remains first-class.

3. Git/workspace parity behavior — **Done**
- Verified `WorkingDir` is consistently `/workspace` for all modes including `gitClone`.
- Workspace path annotations and sync initialization are consistent.

4. Observability parity — **Done**
- Structured `slog` events for each launch phase: `pod-create`, `wait-schedule`, `image-pull` (on failure), `home-sync`, `workspace-sync`, `complete`.
- Events include agent name, namespace, and phase identifiers.

Acceptance checks:
- Tests for harness env, telemetry env, and auth composition parity (`k8s_parity_test.go`).
- Tests for ResolvedAuth + ResolvedSecrets composition (no mutual exclusion).
- Tests for auth files using K8s Secret (no hostPath).
- Tests for local-volume behavior (skipped with warning).
- Tests for GCS volume handling (unchanged).
- Tests for workspace/gitClone parity.
- Tests for auth file secret creation.

## Stage 2: Production Hardening (Mid Term) — COMPLETED

Goal: make Kubernetes runtime resilient under real cluster conditions.

Deliverables:
1. Robust sync engine behavior — **Done**
- `syncWithRetry` wraps all sync operations with exponential backoff (1s, 2s, 4s, up to 3 retries).
- `isSyncTransientError` classifies connection resets, broken pipes, timeouts, EOF, and TLS errors as retryable.
- Non-transient errors (permission denied, not found) fail immediately without retry.
- Tar snapshot sync model documented as current approach; incremental sync is a future optimization.

2. Pod spec hardening — **Done**
- `PodSecurityContext.FSGroup` set from host GID for volume permission alignment.
- `NodeSelector` map support added to `KubernetesConfig` and applied to pod spec.
- `Tolerations` support added via `K8sToleration` struct, mapped to `corev1.Toleration`.
- `RuntimeClassName` now applied to pod spec (was previously read but not wired).
- `ResourceSpec.Disk` now sets both requests AND limits for ephemeral-storage.
- `resource.MustParse` replaced with `parseResourceSafe` — returns actionable errors instead of panicking.

3. Image handling policy — **Done**
- `ImagePullPolicy` field added to `KubernetesConfig` (Always, IfNotPresent, Never) with validation.
- `waitForPodReady` enhanced with structured error classification:
  - Image pull failures with remediation hints.
  - `CreateContainerConfigError` with config guidance.
  - `CrashLoopBackOff` with log check suggestion.
  - `Unschedulable` pods with scheduling guidance.
  - Pod-level scheduling condition checking.
- `ImageExists` validates image name format (empty, whitespace).

4. Multi-namespace operations — **Done**
- `ListAllNamespaces` field on `KubernetesRuntime` (configured via `V1RuntimeConfig.ListAllNamespaces`).
- `List()` queries all namespaces when `ListAllNamespaces` is true (empty namespace = all).
- `resolveNamespace()` helper looks up pod annotations for namespace, with cross-namespace search fallback.
- `fabric.namespace` annotation persisted at pod creation for lifecycle operations.
- `Delete`, `GetLogs`, `Attach`, `Exec`, `GetWorkspacePath` all support `namespace/pod` ID format.
- `Delete`, `GetLogs`, `Attach`, `Exec`, `GetWorkspacePath` all use `resolveNamespace` for annotation-based lookup.
- `Sync` already had namespace resolution via agent annotations.
- `AgentK8sMetadata` populated in List responses with namespace and pod name.

Acceptance checks:
- Unit tests for sync retry behavior (success, transient retry, permanent fail, max retries, context cancellation).
- Unit tests for transient error classification.
- Unit tests for security context (FSGroup), node selectors, tolerations, runtimeClassName.
- Unit tests for ephemeral-storage in both requests and limits.
- Unit tests for safe resource parsing (invalid values return errors, no panics).
- Unit tests for image pull policy configuration and validation.
- Unit tests for image name validation.
- Unit tests for multi-namespace list (all namespaces vs single namespace).
- Unit tests for namespace resolution from annotations and defaults.
- Unit tests for namespace/pod delete format.
- Unit tests for namespace annotation persistence.
- Integration test for full config with all Stage 2 features applied.

## Stage 3: Launch Readiness and UX (Final) — COMPLETED

Goal: ship a predictable, documented, supportable Kubernetes runtime.

Deliverables:
1. CLI/user experience polish — **Done**
- `fabric doctor` command implemented with runtime-aware diagnostic checks.
- Kubernetes diagnostics verify: cluster connectivity, namespace access, pod CRUD/exec permissions, secret permissions.
- GKE-mode diagnostics verify: SecretProviderClass CRD, Secrets Store CSI driver, GCS FUSE CSI driver.
- Error messages throughout the runtime include direct remediation hints (image pull, scheduling, config, resource parsing).
- `fabric doctor --format json` for machine-readable output.

2. Context support wiring — **Done**
- `NewClientWithContext()` added to k8s client, supporting explicit context selection.
- `V1RuntimeConfig.Context` now wired through factory to k8s client construction.
- Backwards compatible: empty context uses current-context from kubeconfig.

3. Documentation and policy — **Done**
- Published explicit support matrix in `docs-site/src/content/docs/hub-admin/kubernetes.md`:
  - Supported volume types per deployment mode (EmptyDir, GCS FUSE, local/PVC status).
  - Secret modes (native K8s Secret, GKE Secret Store CSI, ResolvedAuth).
  - Sync modes and tradeoffs (tar snapshot, GCS volume sync).
  - Required RBAC permissions with example ClusterRole.
  - Pod spec features matrix (resources, tolerations, nodeSelector, runtimeClassName, etc.).
  - Namespace management features.
  - Error handling reference table.
  - Diagnostics usage guide.

4. Release gates — **Done**
- Defined as acceptance checks below. SLO targets are documented for CI integration:
  - Agent start: pod creation + ready within 10-minute timeout, structured failure classification.
  - Attach: tmux session connectivity with terminal resize support.
  - Sync: tar snapshot with retry (3 attempts, exponential backoff) and transient error classification.
  - Cleanup: pod + secret + SecretProviderClass deletion with namespace resolution.

Acceptance checks:
- Unit tests for `fabric doctor` diagnostic checks (connectivity, namespace, permissions, GKE CRDs).
- Unit tests for context wiring (`NewClientWithContext` with specific context, fallback, invalid context).
- Unit tests for error message quality (actionable field names, valid options listed).
- Unit tests for full Stage 3 config integration (all features applied to pod spec).
- Documentation matches implementation with no known silent no-op fields.
- `kubernetes.context` is now wired (was previously a no-op).

## Prioritized Backlog (Suggested)
1. Fix namespace resolution across all lifecycle methods.
2. Wire `runtimeClassName`, context support, and namespace precedence.
3. Remove `MustParse` panics.
4. Unify env/auth composition with shared helper logic.
5. Replace/contain `hostPath` auth file strategy.
6. Implement non-GCS volume behavior (or explicit unsupported errors by mode).
7. Add integration test lanes and failure-mode tests.
8. Add observability and doctor checks.

## Definition of “Parity Enough” for First Launch
Kubernetes runtime is launch-ready when:
1. Core lifecycle (`start/list/logs/attach/sync/exec/delete`) is namespace-correct and deterministic.
2. Env/auth behavior matches local runtimes for equivalent configs.
3. Unsupported Kubernetes constraints are explicit errors, not silent ignores.
4. Sync behavior is reliable with clear failure recovery paths.
5. CI includes real-cluster integration coverage for critical paths.
