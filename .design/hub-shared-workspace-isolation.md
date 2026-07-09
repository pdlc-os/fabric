# Per-Agent State Isolation in Hub Shared-Workspace Groves

**Status**: Phases 1–3 complete (2026-04-18); Phase 4 deferred (no current need)
**Date**: 2026-04-18
**Builds on**: [`grove-mount-protection.md`](./grove-mount-protection.md) (Phases 1–5 complete)

## Problem Statement

In hub-hosted git groves with `sharedWorkspace = true`, every agent in the grove sees the per-agent state of every sibling agent at `/workspace/.fabric/agents/`.

This happens because the runtime mount logic for shared-workspace mode mounts the entire grove directory directly as `/workspace`:

```
~/.fabric/groves/<slug>/   →   /workspace
```

(`pkg/runtime/common.go:190-194`). The grove directory contains the in-grove agent parent dir at `<grove>/.fabric/agents/<name>/` (created in `pkg/agent/provision.go:271-275`), and that parent dir is therefore visible to every container that mounts the grove.

### Why other workspace modes do not have this leak

| Mode | Mount shape | Why `.fabric/agents/` is hidden |
|---|---|---|
| **Worktree (local git)** | `<repo>/.git → /repo-root/.git`, worktree → `/repo-root/<rel>` | The container only sees the worktree (a single branch checkout); `.fabric/agents/` is not part of it. The repo root itself is not mounted. |
| **Non-git (project mounted at `/workspace`)** | `<project>/ → /workspace` | Phase 2 of `grove-mount-protection.md` moved per-agent state out of the project dir entirely (lives in `~/.fabric/grove-configs/<slug>__<uuid>/`). |
| **Fallback (workspace outside repo root)** | `<repo>/ → /repo-root`, workspace → `/workspace` | Phase 1 of `grove-mount-protection.md` added a tmpfs shadow at `/repo-root/.fabric` (`pkg/runtime/common.go:335-336`). |
| **Shared-workspace (hub git grove)** | `~/.fabric/groves/<slug>/ → /workspace` | **No protection.** Tmpfs shadow only fires in the fallback branch (`fullRepoRootMounted == true`); the shared-workspace branch sets `/workspace` directly without it. |

### What is actually leaking

Per-agent state under `<grove>/.fabric/agents/<name>/` (broker side) currently contains:

| File / dir | Origin | Sensitivity |
|---|---|---|
| `prompt.md` | `provision.go:285-290`; user-supplied task text | Medium — may contain operational context the user did not intend siblings to see |
| `fabric-agent.json` | `provision.go:710+`; persisted `FabricConfig` (image, auth type, env keys, harness) | Medium — reveals harness/auth setup of siblings |
| `workspace/` | `provision.go:275`; intended as worktree mount in worktree mode | Empty / unused in shared-workspace mode (no per-agent worktree exists) |
| `home/` | Already external for git groves with split storage (`grove_marker.go:249-253`) | Not leaking — already at `~/.fabric/grove-configs/<slug>__<uuid>/.fabric/agents/<name>/home/` |

Agent **homes** (the secret-bearing directories) were already moved external by Phase 3 of `grove-mount-protection.md`. The remaining residue is the in-grove `agents/<name>/` parent dir itself, which carries `prompt.md`, `fabric-agent.json`, and the never-used `workspace/` shell.

## Design Goals

1. **Sibling invisibility for shared-workspace agents.** A container in a shared-workspace grove must not be able to read or write any other agent's per-agent state.
2. **Re-use the existing externalization model.** Phase 3 already moved homes to `~/.fabric/grove-configs/<slug>__<uuid>/`. The same mechanism should host the rest of per-agent state for shared-workspace groves.
3. **No regression for worktree mode.** The worktree at `<grove>/.fabric/agents/<name>/workspace/` must keep working — git's worktree pointers depend on the relative offset between the worktree and the main repo's `.git` (preserved by the mount layout in `pkg/runtime/common.go:181-189`).
4. **Mount logic stays branchable on `sharedWorkspace`.** The flag is already plumbed end-to-end through dispatch and restart (`pkg/hub/httpdispatcher.go:1037`, fixed in commit `e8cd7fdd`). Path resolution should branch on it cleanly.
5. **Defense by absence, not by configuration.** Sibling state should not be mounted into the container at all — a `.gitignore` or a permission bit is not sufficient (containers run as the same UID).

---

## Proposed Solution

### Storage layout

For groves where `sharedWorkspace = true`, relocate the **entire** per-agent state directory (not just `home/`) to the existing grove-configs external path:

```
~/.fabric/grove-configs/<slug>__<uuid>/.fabric/agents/<name>/
├── home/              # already external (Phase 3)
├── prompt.md          # NEW: moved out of <grove>/.fabric/agents/<name>/
├── fabric-agent.json   # NEW: moved out of <grove>/.fabric/agents/<name>/
└── (no workspace/ — see below)
```

`<grove>/.fabric/agents/` becomes empty for shared-workspace groves (and may be omitted entirely if no agent ever populated it under the old layout). Worktree-mode groves are unaffected; their `<grove>/.fabric/agents/<name>/workspace/` continues to live in-grove because git relative paths require it.

### Path resolution

Introduce a single helper that returns the broker-side per-agent directory, branching on `sharedWorkspace`:

```go
// GetAgentDir returns the broker-side directory for per-agent state files
// (prompt.md, fabric-agent.json, and — in worktree mode — the workspace
// subdir). For shared-workspace groves this is the external grove-configs
// path so siblings cannot see it via the shared mount. Otherwise it stays
// in-grove so worktree-relative-path mechanics keep working.
func GetAgentDir(projectDir, agentName string, sharedWorkspace bool) string
```

This is the symmetric counterpart to the existing `GetAgentHomePath` (`grove_marker.go:249-253`). All call sites that currently compute `filepath.Join(projectDir, "agents", agentName)` get rewritten through this helper.

### Container-side mount

For shared-workspace agents, the container mount shape stays as it is today (one bind mount, grove → `/workspace`). The relocation is purely host-side: because `prompt.md` and `fabric-agent.json` move off the grove tree, they are no longer present at `/workspace/.fabric/agents/<name>/`, and there is nothing to leak.

If any in-container code path turns out to need the agent's own `prompt.md` or `fabric-agent.json` (it does not today — these are read broker-side; see "Container-side reads" below), we add a single per-agent bind mount:

```
~/.fabric/grove-configs/<slug>__<uuid>/.fabric/agents/<name>/   →   /agent-state   (rw)
```

Mounted only for the owning agent, so siblings remain invisible. The home mount continues to use its current `$HOME` target.

### Container-side reads (audit)

The investigation found no in-container reader of `<workspace>/.fabric/agents/<self>/`:

- The harness reads its config out of `$HOME` and `agentHome` (mounted from the external home dir) — `pkg/harness/claude_code.go:170-235`.
- `fabrictool` inside the container does not import `pkg/config` (verified by the import-isolation canary added in Phase 5 of `grove-mount-protection.md`), so it cannot resolve grove paths and does not read `prompt.md`/`fabric-agent.json` from the workspace.
- No env var passed to the container points at `.fabric/agents/` (`pkg/runtimebroker/start_context.go:220-290`).

So the relocation is invisible to container code; only broker-side code needs to learn the new path.

### Hub coordination

The hub already passes `sharedWorkspace` through both create and start dispatch paths (`pkg/hub/httpdispatcher.go:1037`, after `e8cd7fdd`). No new wire changes required — the broker resolves the path locally based on the already-received flag.

---

## Alternatives Considered

### A. Bind-mount overlay over `.fabric/agents/`

Mount an empty tmpfs (or per-agent dir) over `/workspace/.fabric/agents/` to hide siblings without changing where state is stored on the host.

- **Pros:** No host-side path migration; leaves `<grove>/.fabric/agents/` untouched; works as a defense-in-depth layer regardless of where state lives.
- **Cons:** Trivial in Docker; awkward in K8s (needs an init container to assemble subPath mounts or an emptyDir + symlink dance); diverges across runtimes. State still exists on the broker filesystem in a shared spot — the protection is only at the container boundary, parallel to the existing tmpfs shadow at `/repo-root/.fabric`. If a future code path mounts the grove differently, the leak resurfaces.
- **Verdict:** Worth keeping as a defense-in-depth backstop (see "Phase 3" below), but not the primary fix. Defense-by-absence is what the rest of the grove-mount-protection design chose.

### B. Subpath / selective mount of grove contents

Mount only the git-tracked tree as `/workspace`, omitting `.fabric/`. Either via a curated mount list or overlayfs/union setup.

- **Pros:** Clean separation between code and broker state.
- **Cons:** Curated mount lists are fragile (every new top-level dir in the repo must be re-listed). overlayfs adds runtime complexity and may not be supported in K8s. Doesn't help if agents need to read project-level `.fabric/templates/` (committable per Phase 3).
- **Verdict:** Strictly worse than relocating state — adds complexity without making `.fabric/templates/` accessible for the legitimate use case.

### C. Tmpfs shadow over `/workspace/.fabric/agents` only (mirror Phase 1)

Add `--mount type=tmpfs,destination=/workspace/.fabric/agents` in the shared-workspace branch of `buildCommonRunArgs`.

- **Pros:** ~5-line fix; perfectly mirrors the existing Phase-1 protection pattern.
- **Cons:** Tmpfs shadow hides the parent dir but means *the agent itself* cannot find its own state in `/workspace/.fabric/agents/<self>/` either. That is fine today (no in-container reader) but is a hidden footgun if a future harness or template tries to write there: writes would land in tmpfs and vanish on container stop.
- **Verdict:** Cheap immediate mitigation but accumulates technical debt. Reasonable as Phase 0 if we want a stop-gap before the structural fix lands.

### D. Per-agent UID / permission isolation

Run each agent container as a different UID and chmod 0700 sibling dirs.

- **Verdict:** Rejected. The grove-mount-protection doc already documents that container UIDs are synchronized to the host UID, so file permissions provide no isolation between agents. No change here.

### E. New external path namespace `~/.fabric/groves/<slug>-state/<name>/`

The original brainstorm proposed a fresh top-level dir.

- **Pros:** Visually distinct from the workspace at `~/.fabric/groves/<slug>/`; avoids any mental conflation with grove-configs.
- **Cons:** Forks the externalization story. Phase 3 already chose `~/.fabric/grove-configs/<slug>__<uuid>/.fabric/agents/<name>/` as *the* external per-agent path; introducing a parallel namespace doubles the resolution logic, the cleanup paths, and the `fabric grove prune` surface area. The grove-id / UUID convention exists precisely so brokers can route by marker file.
- **Verdict:** Use the existing grove-configs path. Documented as an open question below in case there's a reason to revisit.

---

## Decisions (resolved from open questions)

1. **Path namespace** → Reuse the existing `~/.fabric/grove-configs/<slug>__<uuid>/.fabric/agents/<name>/` path. No new top-level namespace.

2. **Empty `workspace/` shell** → Skip creation entirely when `sharedWorkspace` is set. No unused dir.

3. **Defense-in-depth tmpfs** → Rely on structural absence; do not add a tmpfs shadow. Add a code comment in `buildCommonRunArgs()` cross-referencing the Phase-1 `/repo-root/.fabric` tmpfs pattern as a potential future addition if the threat model changes.

4. **Migration of in-place groves** → Detect-and-migrate. On provision/start, if `sharedWorkspace` is true and `<grove>/.fabric/agents/<name>/fabric-agent.json` (or `prompt.md`) exists at the old in-grove path, move it to the external path and remove the in-grove copy.

5. **`.gitignore` enforcement** → Keep enforcing `.fabric/agents/` in `.gitignore` for all git groves, including shared-workspace ones. Guards against future regressions.

6. **Hub-side visibility** → Confirmed: no hub admin tool or debug endpoint walks the grove directory for per-agent files. No changes needed.

7. **CLI on broker host** → Not a concern for this scope. This work is limited to hub-managed groves; `fabric` commands in that context use the hub API as the source of truth for agent records, not filesystem walking.

8. **`fabric-agent.json` portability** → Grep all hard-coded `agents/<name>/fabric-agent.json` path references as part of the implementation and update them through `GetAgentDir`.

---

## Implementation Plan

### Phase 0 (optional stop-gap, ~½ day)

If a fix is needed before the structural change can land, mirror the Phase-1 tmpfs shadow into the shared-workspace branch:

- `pkg/runtime/common.go:190-194`: add `--mount type=tmpfs,destination=/workspace/.fabric/agents` when entering the shared-workspace branch.
- New test in `pkg/runtime/common_test.go` mirroring the existing `/repo-root/.fabric` test.
- Document as a known limitation that this *also* shadows the agent's own state dir — but since nothing reads from there in-container, it's a no-op for correctness today.

Skip this phase if Phase 1–2 below can land within the same release.

### Phase 1: Path resolver and broker-side relocation (~1–2 days) — **DONE**

Goal: move `prompt.md`, `fabric-agent.json`, and the `workspace/` shell to the external path for shared-workspace groves. No mount changes yet.

- Add `config.GetAgentDir(projectDir, agentName string, sharedWorkspace bool) string` in `pkg/config/grove_marker.go` (next to `GetAgentHomePath`). Returns external path when `sharedWorkspace` and a grove-id marker exist; otherwise returns `<projectDir>/agents/<name>` (current behavior).
- Update `pkg/agent/provision.go`:
  - Line 271-275: replace `agentDir := filepath.Join(agentsDir, agentName)` with the new helper, threading `sharedWorkspace` from `api.IsSharedWorkspaceFromContext(ctx)`.
  - Line 285-290 (`prompt.md` write), Line 710+ (`fabric-agent.json` write): no source changes if they all go through `agentDir`, just verify.
  - Line 277 (`os.MkdirAll(agentDir, 0755)`): now creates external dir for shared-workspace.
- Update `pkg/agent/provision.go:35-140` (`DeleteAgentFiles`): the existing code already handles an `externalAgentDir` cleanup path; extend it to remove the new external `prompt.md`/`fabric-agent.json`/`workspace` entries, not just `home/`.
- Update any other broker-side reader of `<projectDir>/agents/<name>/` to use the helper. Suspected sites from the investigation:
  - `cmd/sync.go:571-573` (worktree path discovery — worktree mode only, should not change behavior).
  - `pkg/runtimebroker/workspace_handlers.go:322-328` (same, worktree path).
  - Any `fabric list` / status reader.
- Tests: extend `pkg/agent/provision_test.go` and `pkg/agent/delete_test.go` with shared-workspace fixtures asserting state lands in `~/.fabric/grove-configs/.../agents/<name>/` and not in the grove tree.

### Phase 2: Mount-layer cleanup and assertions (~½ day) — **DONE**

Goal: codify the "no agent state visible in `/workspace`" invariant.

- `pkg/runtime/common.go:190-194`: in the shared-workspace branch, optionally add the defense-in-depth tmpfs from Phase 0 (deferred to open question 3). At minimum, add a comment cross-linking to this design doc.
- New test in `pkg/runtime/common_test.go`: build mount args for a shared-workspace agent and assert that no host path under `<grove>/.fabric/agents/` is referenced.
- Smoke test (manual or scripted): start two agents in a shared-workspace grove; from inside agent A's container, attempt to `ls /workspace/.fabric/agents/agent-b/` and confirm absence.

### Phase 3: Migration & docs (~½ day) — **DONE**

- Open question 4 (migration) — implemented as detect-and-migrate in `migrateLegacyAgentState` (`pkg/agent/provision.go`). On every provision call for a shared-workspace agent, any pre-existing `prompt.md` / `fabric-agent.json` at the legacy in-grove path (`<grove>/.fabric/agents/<name>/`) is moved to the external grove-configs path. Covered by `TestProvisionAgent_SharedWorkspaceMigratesLegacyState` (`pkg/agent/provision_test.go`). The legacy parent dir is removed when empty.
- `.design/grove-mount-protection.md`: closing note added pointing at this design as the follow-up that closed the shared-workspace residual gap. Kept as a separate doc rather than absorbed as a phase, since the threat shape (shared mount surface vs. dedicated worktree mount) is meaningfully different and benefits from independent framing.
- `docs-site/src/content/docs/concepts.md`: Resource Isolation section updated to note that for shared-workspace groves, per-agent state is relocated to the external grove-configs path so siblings cannot see it via the shared mount.
- `docs-site/src/content/docs/hub-user/runtime-broker.md`: Security & Isolation entry clarified — agents in shared-workspace groves share the workspace mount but their per-agent state (prompt, agent config) is held outside that mount.

### Phase 4 (optional): per-agent state mount inside container — **DEFERRED**

Mount the external per-agent dir at `/agent-state` (or similar) for the owning agent only, so in-container code can read its own `prompt.md` / `fabric-agent.json`.

Not implemented. Verified at the time of Phase 3 completion (2026-04-18) that no in-container code path reads either file from `/workspace/.fabric/agents/<self>/`:

- Harnesses read system prompts and configs from `$HOME` / agent-home (e.g. `pkg/harness/claude_code.go:332,455`, `pkg/harness/gemini_cli.go:105-113`), not from the workspace tree.
- `fabrictool` writes a hook-captured first prompt to `~/prompt.md` (`pkg/fabrictool/hooks/handlers/prompt.go:34`); it does not read the broker-side `prompt.md`.
- `fabrictool` does not import `pkg/config` (canary test from Phase 5 of grove-mount-protection), so it cannot resolve grove paths to find `.fabric/agents/<name>/fabric-agent.json`.

Re-evaluate if a future harness or in-container tool grows a need to read the broker-side `prompt.md` or `fabric-agent.json` directly. Implementation is straightforward when needed: register a single per-agent bind mount in the shared-workspace branch of `buildCommonRunArgs` (`pkg/runtime/common.go`) — `~/.fabric/grove-configs/<slug>__<uuid>/.fabric/agents/<name>/` → `/agent-state` (rw), mounted only for the owning container so siblings remain invisible.

---

## Out of Scope

- **Worktree-mode visibility.** Already protected; no changes proposed.
- **Project-templates visibility.** `.fabric/templates/` is intentionally committable and shared (Phase 3 of grove-mount-protection); siblings seeing it is by design.
- **Cross-grove isolation.** Each container only mounts one grove; cross-grove leakage is not a concern here.
- **Hub-side state visibility.** Hub never reads broker-local agent files; out of scope.
