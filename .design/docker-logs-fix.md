# Fix: Broker `getLogs` handler uses empty Slug for agent.log path

## Problem

When an agent calls `fabric logs <other-agent>` via the hub, the broker's `getLogs` handler fails to read `agent.log` from the filesystem and falls back to `docker logs`, which can also fail.

### Error observed

```
Failed to get logs: docker logs foo failed: exit status 1
```

### Root cause

The `getLogs` handler in `pkg/runtimebroker/handlers.go` (line ~1340) uses `found.Slug` to build the filesystem path to `agent.log`:

```go
agentLogPath := filepath.Join(config.GetAgentHomePath(
    filepath.Join(found.GrovePath, ".fabric"), found.Slug,
), "agent.log")
```

**No runtime implementation (Docker, Podman, K8s, Apple) ever populates the `Slug` field in `api.AgentInfo`.** The slugified agent name is stored in `found.Name`, populated from the `fabric.name` Docker label. With `Slug` empty, the path resolves to:

```
<GrovePath>/.fabric/agents//home/agent.log
→ <GrovePath>/.fabric/agents/home/agent.log   (wrong)
```

Instead of the correct:

```
<GrovePath>/.fabric/agents/foo/home/agent.log
```

The `ReadFile` always fails, falling through to the `docker logs` fallback. The fallback has a secondary issue: it passes the raw request slug (`id`) instead of `found.ContainerID` to `rt.GetLogs()`. When the Docker container name is grove-prefixed (e.g. `mygrove--foo`), `docker logs foo` fails because Docker can't find a container named `foo`.

### Why `fabric look` worked but `fabric logs` didn't

`fabric look` uses `rt.Exec()` which runs `docker exec --user fabric foo <cmd>`. In setups where the container name matches the slug (no grove prefix), Docker finds the container. The `look` command had a *different* bug (tmux socket discovery) that was fixed separately. `fabric logs` hits the path resolution bug before it even reaches Docker.

## Fix

### 1. Use `Name` as fallback when `Slug` is empty (primary fix)

In `pkg/runtimebroker/handlers.go`, `getLogs` handler:

```go
// Before the filesystem read:
agentSlug := found.Slug
if agentSlug == "" {
    agentSlug = found.Name
}

// Then use agentSlug instead of found.Slug:
agentLogPath := filepath.Join(config.GetAgentHomePath(
    filepath.Join(found.GrovePath, ".fabric"), agentSlug,
), "agent.log")
```

`found.Name` is populated by every runtime from the `fabric.name` label, which is already the slugified agent name (set as `api.Slugify(opts.Name)` during container creation at `pkg/agent/run.go:797`).

### 2. Use `ContainerID` in the `docker logs` fallback (secondary fix)

When the filesystem read fails and the handler falls back to container logs, use the actual container identifier instead of the request slug:

```go
containerID := found.ContainerID
if containerID == "" {
    containerID = id
}
logs, err := rt.GetLogs(ctx, containerID)
```

This handles the case where the Docker container name is grove-prefixed (`mygrove--foo`) but the request slug is just `foo`.

## Files changed

| File | Change |
|------|--------|
| `pkg/runtimebroker/handlers.go` | `getLogs`: resolve slug from `Name` when `Slug` is empty; use `ContainerID` in fallback |
| `pkg/runtimebroker/handlers_test.go` | Add `TestAgentLogsReadsFileWhenSlugEmpty` and `TestAgentLogsFallbackUsesContainerID` |

## Test plan

- `TestAgentLogsReadsFileWhenSlugEmpty` — creates a temp directory with `agent.log`, sets up a mock agent with empty `Slug` and populated `Name`/`GrovePath`, verifies the handler reads the file (and never calls `GetLogs` on the runtime).
- `TestAgentLogsFallbackUsesContainerID` — sets up a mock agent with `ContainerID` set to a grove-prefixed name, verifies the `GetLogs` fallback receives the container ID, not the slug.

## Related

- The `execCommand` handler has the same pattern (passes raw `id` to `rt.Exec`) but is not addressed here — it works in the common case where the container name matches the slug, and fixing it would require the same list-then-resolve approach that `getLogs` already does.
