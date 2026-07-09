# Grove Mount Protection: Agent Isolation from `.fabric` Directory

**Status**: Accepted
**Date**: 2026-03-08
**Updated**: 2026-03-09

## Problem Statement

Agents running in containers can access the `.fabric` directory of the grove they belong to. This directory contains other agents' home folders, which hold sensitive material:

- `.codex/auth.json` — raw API keys (written with `0600` perms)
- `.fabric/secrets.json` — hub-projected variable secrets
- `.config/gcloud/application_default_credentials.json` — cloud credentials
- `.gemini/oauth_creds.json` — OAuth tokens
- Various harness-specific config files with auth fingerprints

Since all agents run with the same host UID (container user UID is synchronized to the host), `0600` file permissions provide **no isolation** between agents — any agent that can reach another agent's home directory can read its secrets.

### How Exposure Occurs

| Scenario | Mechanism | Severity |
|---|---|---|
| **Non-git workspace** | Project directory mounted at `/workspace`; `.fabric/` is physically present | High — full access to all agent homes |
| **Full-repo fallback mount** (`common.go:169`) | When workspace is outside repo root, entire repo root mounted at `/repo-root` | High — `.fabric/` accessible at `/repo-root/.fabric/` |
| **Git worktree workspace** | Worktree excludes gitignored dirs; `.fabric/` not materialized | Protected (incidentally, not by design) |

The git-worktree case is only protected because `.fabric` is in `.gitignore` — this is an incidental side effect, not a security control.

### Current Mount Architecture

From `pkg/runtime/common.go`, `buildCommonRunArgs()`:

```
Agent home:  .fabric/agents/<name>/home  →  /home/<user>     (bind mount, rw)
Workspace:   <worktree-path>            →  /repo-root/...   (bind mount, rw)
.git:        <repo>/.git                →  /repo-root/.git  (bind mount, rw)
```

When the workspace is outside the repo root (external worktrees, explicit `--workspace`):
```
Repo root:   <repo>/                    →  /repo-root       (bind mount, rw)  ← EXPOSES .fabric/
Workspace:   <path>                     →  /workspace       (bind mount, rw)
```

---

## Decisions

### Storage Architecture

**Non-git groves**: Externalize grove data entirely (Approach 1).

**Git groves**: Split storage — worktrees stay in-repo, agent homes move external (Approach 2). This has the advantage that `.fabric/templates/` with custom project templates can be committed to git.

### Directory Layout

All `.fabric` configuration folders (other than the special global `~/.fabric/`) are stored externally:

```
~/.fabric/grove-configs/<grove-slug>__<short-uuid>/.fabric/    # All grove configs
~/.fabric/groves/<grove-name>/                                 # Hub-managed workspaces only (pure workspace holder)
```

This separation means `~/.fabric/groves/` is a "pure" workspaces directory, and all configs are isolated from workspace content — whether those workspaces are hub-managed (created via hub→broker) or linked by a user from the broker filesystem to a hub.

### Grove Path Naming

Use `<grove-slug>__<short-uuid>` format for grove-config paths. The short-uuid must be sufficient to route from a marker file to the correct location in `~/.fabric/grove-configs/` space.

### Marker File Format

For non-git groves, `.fabric` becomes a YAML file (without extension) containing:

```yaml
grove-id: <uuid>
grove-name: my-project
grove-slug: my-project
```

- `grove-name`: Based on the current working directory name
- `grove-slug`: Slugified version of the name
- `grove-id`: Hub/global UUID (consistent across brokers for linked groves)

### Settings File

The `settings.yaml` file in `~/.fabric/grove-configs/<grove-slug>__<short-uuid>/.fabric/` must include a `workspace-path` field. This is the path stored in the broker provider record so that a broker server knows what workspace to mount for a linked grove.

### UUID Source

The UUID is a hub/global UUID. Linked groves on different brokers all share the same UUID. A grove-broker combo is constrained to be 1:1 with a hub for now.

### Migration Strategy

This is a hard breaking change. If an older-style folder grove is detected, bail and print an error directing the user to reinitialize. Mark migration-check code with a TODO for removal at a later release date.

### Orphan Cleanup

Use the existing `fabric grove` command group:
- `fabric grove prune` — clean up orphaned grove configs
- `fabric grove reconnect` — re-establish the workspace path in settings.yaml if a linked grove was moved

### Hub API Consolidation

Audit and ensure that all in-container CLI operations use the Hub API (via `FABRIC_HUB_ENDPOINT`), eliminating filesystem access to grove data. This makes mount-level protections defense-in-depth rather than the primary control. **This is a separate phase of work.**

### Local Mode

Support behavioral differences in CLI when `hub.enabled` is true vs false. Local mode is supported but may have weaker isolation guarantees.

---

## Approach Details

### Non-Git Groves: Externalized Grove Data

Move grove state out of the project directory entirely.

**Mechanism:**

When `fabric init` runs in a non-git project:

1. Generate a UUID for the grove (hub-sourced when hub is available)
2. Create grove config directory at `~/.fabric/grove-configs/<grove-slug>__<short-uuid>/.fabric/` (agents, templates, settings)
3. Write `workspace-path` to `settings.yaml` in the grove config
4. Write a **marker file** at `<project>/.fabric` (not a directory) containing the YAML fields above
5. The container mounts only the project directory; the `.fabric` marker file is inert

**Code impact:**

The following functions in `pkg/config/paths.go` check `info.IsDir()` and would need to handle the file-marker case:
- `FindProjectRoot()` (line 41)
- `ResolveGrovePath()` (line 198)
- `RequireGrovePath()` (line 231)
- `GetEnclosingGrovePath()` (line 261)

These would need to: detect `.fabric` as a file, parse the grove-id/slug, and resolve to `~/.fabric/grove-configs/<grove-slug>__<short-uuid>/.fabric/`.

Additionally affected:
- `GetProjectAgentsDir()` / `GetProjectTemplatesDir()` — must follow the indirection
- `InitProject()` in `pkg/config/init.go` — rewrite to create file + external directory
- `GetGroveName()` — read from the marker file or derive from the project directory name

### Git Groves: Split Storage

For git-based groves, worktrees must remain inside the repo (they rely on `--relative-paths` for container mounting). The `.fabric` directory stays as a directory but agent homes move external.

**Mechanism:**

```
<repo>/.fabric/                      (gitignored, remains a directory)
├── config.yaml                     settings, grove config
├── settings.yaml
├── templates/                      custom templates (committable to git)
├── grove-id                        file with UUID for cross-referencing
└── agents/
    └── <name>/
        └── workspace/              git worktree (relative paths work)

~/.fabric/grove-configs/<grove-slug>__<short-uuid>/   (external, never mounted into containers)
└── agents/
    └── <name>/
        └── home/                   agent home with secrets
```

- **Worktree mechanics** stay in `<repo>/.fabric/agents/<name>/workspace/` (because git relative paths require this)
- **Agent homes with secrets** move to `~/.fabric/grove-configs/<grove-slug>__<short-uuid>/agents/<name>/home/`
- The `config.HomeDir` in `RunConfig` already points to a specific path independent of workspace — changing it from `.fabric/agents/<name>/home` to the external path is straightforward

**Code impact:**

- `pkg/agent/provision.go`: Change `agentHome` derivation to use external path
- `pkg/agent/run.go`: `HomeDir` in `RunConfig` already decoupled from workspace
- Agent deletion (`pkg/agent/delete.go`): Must clean up both locations
- `fabric list`: Must reconcile state from two directories

---

## Implementation Phases

### Phase 1: Mount-Level Quick Fix (Immediate) ✅ COMPLETE

Close the active vulnerability with minimal code changes while the structural work is planned and implemented.

**Scope:**
- Modify `buildCommonRunArgs()` in `pkg/runtime/common.go` to add a tmpfs shadow mount over `.fabric/` when the full repo root is mounted:
  ```
  --mount type=tmpfs,destination=/repo-root/.fabric
  ```
- Enforce `.fabric` in `.gitignore` during `fabric init` for git repos
- Add a startup warning if `.fabric` is not gitignored when starting agents

**Deliverables:**
- Updated `pkg/runtime/common.go` with tmpfs shadow mount
- Updated `pkg/config/init.go` with gitignore enforcement
- Tests for mount argument generation

### Phase 2: Externalize Non-Git Groves ✅ COMPLETE

Implement the marker-file approach for non-git groves, creating the `grove-configs` external directory structure.

**Scope:**
- Implement marker file creation in `InitProject()` for non-git groves
- Update path resolution functions in `pkg/config/paths.go` to handle file-marker indirection
- Create `~/.fabric/grove-configs/<grove-slug>__<short-uuid>/.fabric/` directory structure
- Add `workspace-path` to `settings.yaml` schema
- Add old-style grove detection with hard error and TODO for removal
- Update agent provisioning to use external home paths for non-git groves

**Deliverables:**
- Updated `pkg/config/paths.go` with marker file resolution
- Updated `pkg/config/init.go` with external grove creation
- Updated `pkg/agent/provision.go` for external home paths
- Updated settings schema with `workspace-path`
- Migration error detection
- Tests for marker file resolution and external grove lifecycle

### Phase 3: Split Storage for Git Groves ✅ COMPLETE

Move agent homes out of the repo `.fabric/` directory for git-based groves while keeping worktrees in-repo.

**Scope:**
- Generate and store `grove-id` in `<repo>/.fabric/grove-id` during init
- Create `~/.fabric/grove-configs/<grove-slug>__<short-uuid>/` for git groves
- Move agent home provisioning to external path
- Update agent deletion to clean up both locations
- Update `fabric list` to reconcile split state
- Ensure templates remain in `<repo>/.fabric/templates/` (committable)

**Deliverables:**
- Updated `pkg/config/init.go` with grove-id generation for git groves
- Updated `pkg/agent/provision.go` for split home paths
- Updated `pkg/agent/delete.go` for dual-location cleanup
- Updated listing/reconciliation logic
- Tests for split storage lifecycle

### Phase 4: Grove Management Commands ✅ COMPLETE

Add CLI commands for grove lifecycle management.

**Scope:**
- `fabric grove prune` — detect and clean up orphaned grove configs in `~/.fabric/grove-configs/`
- `fabric grove reconnect` — update `workspace-path` in settings.yaml when a linked grove has moved
- `fabric grove list` — show all known groves with their status and paths

**Deliverables:**
- New commands in `cmd/`
- Grove registry/discovery logic
- Tests for prune and reconnect scenarios

### Phase 5: Hub API Consolidation ✅ COMPLETE

Audited all in-container CLI operations to verify they route through the Hub API when configured, and do not access filesystem-based grove data.

**Audit Finding:** fabrictool already does NOT access grove-level data from the filesystem. All filesystem access is agent-local (`$HOME/agent-info.json`, `$HOME/agent-limits.json`, `$HOME/.fabric/fabric-services.yaml`). Hub API communication already works for status updates, heartbeats, and token refresh. The remaining work was formalizing this with explicit mode awareness and codifying the guarantees in tests.

**Deliverables:**
- `OperatingMode()` helper in `pkg/fabrictool/hub/client.go` — consolidates mode detection (local/hub-connected/hosted)
- Mode-aware startup logging in `cmd/fabrictool/commands/init.go`
- Import isolation canary test — verifies fabrictool does NOT import `pkg/config` (grove path resolution)
- Operating mode tests — table-driven tests for all three modes
- Hub-enabled vs hub-disabled behavioral tests — verifies HubHandler is nil in local mode and active in hub mode, and StatusHandler always writes agent-info.json (defense-in-depth)

---

## Comparison Matrix

| Concern | Non-Git (Externalize) | Git (Split Storage) |
|---|---|---|
| Protection model | Strong — data not in project dir | Strong — homes not in repo mount |
| Worktree compatibility | N/A | Preserved (worktrees stay in-repo) |
| Code complexity | Medium (~10 resolution sites) | Medium (provisioning + cleanup) |
| Template committability | Templates in external config | Templates in `.fabric/templates/` (in git) |
| Hub model convergence | Good (unifies with hub-managed) | Partial |
| Defense model | Structural (data not present) | Structural (data not present) |

---

## Follow-up: Shared-Workspace Groves

Phase 3 of this design moved agent **homes** out of the in-grove `.fabric/agents/<name>/home/` path, but in groves where every agent shares a single workspace mount (`sharedWorkspace = true`, used by hub-hosted git groves), the rest of the per-agent state — `prompt.md` and `fabric-agent.json` — was still visible to sibling agents through the shared `/workspace/.fabric/agents/<name>/` view. That residual gap is closed in [`hub-shared-workspace-isolation.md`](./hub-shared-workspace-isolation.md), which extends the same external-path model to those files when the grove is in shared-workspace mode.

## Open Questions (Resolved)

| Question | Resolution |
|---|---|
| UUID vs slug for grove paths | Use `<grove-slug>__<short-uuid>` hybrid |
| Marker file format | YAML without extension; includes `grove-name`, `grove-slug`, `grove-id` |
| Migration path | Hard breaking change; bail with error on old-style groves; TODO for removal |
| Orphan cleanup | `fabric grove prune` and `fabric grove reconnect` |
| Multi-broker UUID divergence | Not an issue — UUID is hub/global, shared across brokers |
| Git history exposure | Not a concern — not using mount-only approach for git groves |
| Hub API consolidation | Yes, but in Phase 5 as a separate work session |
| Local mode | Support via `hub.enabled` behavioral differences |
