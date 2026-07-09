# Hub Messaging: CLI Integration

## Status
**Proposed** | February 2026

## Problem

The `fabric message` CLI command (`cmd/message.go`) always sends messages directly via the local runtime, bypassing the Hub entirely. Every other lifecycle command (`start`, `stop`, `delete`, `list`) already routes through the Hub when Hub integration is enabled. Messaging should follow the same pattern.

The Hub API (`POST /api/v1/agents/{id}/message`) and the hubclient `SendMessage` method already exist and are functional. The runtime broker handler (`POST /api/v1/agents/{id}/message`) is also wired up. The only missing piece is the CLI dispatch logic.

## Scope

This change is limited to `cmd/message.go`. No new Hub, broker, or hubclient code is required -- the backend path is already complete.

## Design

### Hub Detection

Follow the established CLI pattern used by `stop.go`, `delete.go`, etc.:

1. Call `CheckHubAvailability` / `CheckHubAvailabilityForAgent` at the start of `RunE`.
2. If `hubCtx != nil`, delegate to a new `sendMessageViaHub` function.
3. Otherwise, fall through to the existing local runtime path.

### Single-Agent Message (default)

When neither `--broadcast` nor `--all` is set, the command targets a single named agent:

```
hubCtx, err := CheckHubAvailabilityForAgent(grovePath, agentName, false)
```

The hub function resolves the grove ID via `GetGroveID(hubCtx)`, gets a grove-scoped agent service via `hubCtx.Client.GroveAgents(groveID)`, and calls `agentSvc.SendMessage(ctx, agentName, message, interrupt)`.

This mirrors `stopAgentViaHub` exactly.

### Broadcast (`--broadcast`)

Broadcast sends the message to all running agents in the current grove. In hub mode:

1. Use `CheckHubAvailability(grovePath)` (no specific agent to exclude from sync).
2. Resolve the grove ID.
3. List agents via `hubCtx.Client.GroveAgents(groveID).List(ctx, opts)` with a status filter for running agents.
4. Iterate over results and call `SendMessage` for each.
5. On per-agent failure, print a warning and continue (matching existing local broadcast behavior).

### All (`--all`)

Sends the message to all running agents across all groves. In hub mode:

1. Use `CheckHubAvailabilityWithOptions(grovePath, true)` (skip sync -- cross-grove operation).
2. List agents via `hubCtx.Client.Agents().List(ctx, opts)` (no grove scope) with a status filter for running agents.
3. Iterate and call `SendMessage` for each, using the global (non-grove-scoped) agent service since agents span multiple groves.
4. On per-agent failure, print a warning and continue.

### Hub Availability Check Placement

The hub availability check must happen **before** argument parsing branches for broadcast/single. The current code parses `agentName` from args conditionally based on `--broadcast`/`--all`, so the hub check placement depends on which mode is active:

- For single-agent: `agentName` is known, use `CheckHubAvailabilityForAgent`.
- For broadcast/all: no specific agent, use `CheckHubAvailability` or `CheckHubAvailabilityWithOptions`.

### Agent Status Filtering

The Hub `List` API supports a `Status` filter. Use `status=running` to filter server-side rather than listing all agents and filtering client-side. This replaces the current local approach of checking `ContainerStatus` strings like "Up" or "running".

### Timeout

Use a 30-second timeout for per-message calls, consistent with other Hub operations. For broadcast to many agents, each call gets its own 30-second timeout.

### Error Handling

- Wrap hub errors with `wrapHubError()` for consistent guidance messaging.
- For broadcast/all: warn on individual failures, don't abort the loop.
- For single-agent: return the error directly (wrapped).

### Output

- Print `Using hub: <endpoint>` via `PrintUsingHub` (skip in JSON output mode).
- Print `Sending message to agent '<name>'...` for each target (existing behavior).
- Append `via Hub` to success/failure messages for clarity.

## Implementation Steps

1. **Restructure `RunE` in `cmd/message.go`**: Move hub detection to the top of the function, after argument parsing.
2. **Add `sendMessageViaHub` function**: Handles single-agent, broadcast, and all modes. Takes `hubCtx`, targets/mode info, message, and interrupt flag.
3. **Add `listRunningAgentsViaHub` helper** (private to the function or inline): Calls `List` with status filter, returns agent names. Used by broadcast and all modes.
4. **Update imports**: Add `time` and `hubclient` packages.
5. **Test**: Verify all three modes (single, broadcast, all) route through Hub when enabled, and that `--no-hub` falls back to local.

## Function Signature

```go
func sendMessageViaHub(hubCtx *HubContext, agentName string, message string, interrupt bool, broadcast bool, all bool) error
```

This keeps the hub path self-contained in a single function, mirroring how `stopAgentViaHub` and `listAgentsViaHub` work.

## Out of Scope

- Hub API changes (already complete).
- Runtime broker changes (already complete).
- Hubclient changes (`SendMessage` already exists on `AgentService`).
- New tests for Hub/broker message handling (already covered).
