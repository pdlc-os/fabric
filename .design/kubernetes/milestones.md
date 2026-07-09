# Kubernetes Support Milestones

*Updated: 2026-02-18*

This document outlines the incremental milestones required to evolve the Fabric Kubernetes runtime to a fully functional, production-ready environment.

Each milestone is designed to be independently testable ("QAable") and builds upon the previous one.

> **Implementation note:** The K8s runtime (`pkg/runtime/k8s_runtime.go`) has been substantially
> implemented (1183 lines, 17 methods). Many items originally scoped in M1-M4 are now complete.
> Status annotations below reflect the current code state as of 2026-02-18.

## Milestone 1: Basic Runtime Configuration & Connectivity — ✅ Complete

**Goal:** Ensure the Kubernetes runtime honors the basic `run` configuration provided by the CLI.

See detailed design in `m1-design.md`.

**Tasks:**
1.  ✅ **Environment Propagation:** `KubernetesRuntime.Run` serializes `runCfg.Env` into the Pod spec. Manages Pods directly (not SandboxClaim CRD).
2.  ✅ **Command Propagation:** Harness command is executed by the container, overriding the default entrypoint.
3.  ✅ **Basic Auth Injection:** API keys injected as environment variables into the Pod spec.

---

## Milestone 2: Identity & Context Projection (The "Snapshot" Fix) — ✅ Complete

**Goal:** Enable standard Harnesses (Gemini/Claude) to function by ensuring their required configuration files and home directory context are present in the remote container.

**Tasks:**
1.  ✅ **Home Directory Sync:** Tar-based upload of agent home directory to container. Mutagen live sync also available for home directory.
2.  ✅ **Wait-for-Init Logic:** Container starts with `tail -f /dev/null`, context sync is performed, then harness command is exec'd.
3.  ✅ **Workspace Path Configuration:** Uses configured workspace path, not hardcoded.

---

## Milestone 3: SCM Integration (Git Clone on Start)

**Goal:** Transition from "uploading local workspace" to "cloning from source" for the project code.

**Status:** Partially addressed. Workspace sync (tar snapshot + Mutagen live sync) is implemented, but git-clone-based init container approach is NOT yet implemented.

**Tasks:**
1.  [ ] **Repository Detection:** Detect Git remote URL of the current grove for clone-based bootstrap.
2.  [ ] **Init Container Injection:** Configure init container to `git clone` into workspace volume.
3.  [ ] **Credential Management (Basic):** Copy local Git credentials into a Kubernetes Secret for init container.

**Current workaround:** Workspace tar snapshot sync (`syncWorkspace`) uploads the local workspace to the container. Mutagen-based live sync keeps it up to date during the session.

**Verification (Manual QA):**
- Run: `fabric start --runtime kubernetes` in a git repo.
- Success: Pod starts, `kubectl exec ls /workspace` shows the git repository content.

---

## Milestone 4: Interactive Synchronization — ✅ Complete

**Goal:** Restore the "local development" feel by allowing users to push/pull changes between their local machine and the remote agent.

**Tasks:**
1.  ✅ **Sync-To:** Tar-based workspace sync pushes local changes to remote `/workspace`.
2.  ✅ **Sync-From:** Tar-based sync pulls remote workspace changes to local directory.
3.  ✅ **Live Sync:** Mutagen-based bidirectional live sync for both workspace and home directory.

---

## Milestone 5: Production Hardening

**Goal:** Move from "Dev/Test" quality to "Production" quality, securing secrets and handling lifecycle events robustly.

**Tasks:**
1.  [ ] **Secret Management:** Replace environment variable injection with proper Kubernetes Secrets for API keys and auth tokens.
2.  [ ] **Status Reconciliation:** Update `fabric list` to accurately reflect K8s Pod status (Pending, Running, CrashLoopBackOff) rather than just local state.
3.  [ ] **Cleanup:** Ensure `fabric delete` removes Secrets, ConfigMaps, and PVCs associated with the agent.
4.  [ ] **SecurityContext:** Set `FSGroup` and other pod security context fields for proper file permissions.
5.  [ ] **Resource Requests/Limits:** CPU and memory requests/limits are implemented but need validation and tuning guidance.

**Verification (Manual QA):**
- Inspect Pod: `kubectl get pod <agent> -o yaml`. Verify no API keys are visible in `spec.containers.env`.
- Run: `fabric delete <agent>`. Verify all related K8s resources are gone.
