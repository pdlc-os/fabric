# Agent Detail Page — Tabbed Layout Redesign

## Overview

Redesign the agent detail page (`/agents/:agentId`) from a flat card layout into a **Header + Two-Tab** layout. The header provides at-a-glance identity and actions. The two tabs — **Status** and **Configuration** — separate dynamic runtime state from static creation-time settings.

## Data Sources

Information comes from two backend structures, but is **not** mapped 1:1 to tabs:

| Backend Source | Examples on Status Tab | Examples on Config Tab |
|---|---|---|
| `store.Agent` (AgentInfo) | phase, activity, toolName, lastSeen, taskSummary, connectionState | created, name, id, slug, template |
| `FabricConfig` (via `AppliedConfig.InlineConfig`) | (derived: time remaining, turns progress) | maxTurns, maxModelCalls, maxDuration, model, image, branch, task |

### New Fields Required

These fields do **not** currently exist on the API response and must be added:

| Field | Source | Notes |
|---|---|---|
| `currentTurns` | Broker → Hub status update | Mirrors `LimitsState.TurnCount` from `agent-limits.json` |
| `currentModelCalls` | Broker → Hub status update | Mirrors `LimitsState.ModelCallCount` |
| `startedAt` | Hub `store.Agent` | Timestamp when agent entered `running` phase (needed for time-remaining calc) |
| `appliedConfig` | Hub `store.Agent` | Already exists on the model but is not included in the frontend `Agent` type |

### Time Remaining Calculation

Must mirror the enforcement in `cmd/fabrictool/commands/init.go`:

```
timeRemaining = maxDuration - (now - startedAt)
```

Where `maxDuration` is parsed from the `FabricConfig.MaxDuration` string (e.g. `"2h30m"`) via Go's `time.ParseDuration`. The frontend should parse the same format and compute the countdown client-side, refreshing on the existing 15-second re-render interval.

---

## Page Layout

### Back Navigation (unchanged)
```
< To {Grove Name}
```

### Header Section (always visible, above tabs)

The header provides the 3-4 most critical pieces of information plus actions.

```
+-----------------------------------------------------------------------+
|  [cpu icon]  {Agent Name}   [status-badge: phase/activity]            |
|                                                                       |
|  [code-square] {template}   [folder] {grove}   [hdd-rack] {broker}   |
|                                                                       |
|                              [Terminal] [Start/Stop] [Configure] [x] |
+-----------------------------------------------------------------------+
```

**Contents:**
- **Agent name** — `h1` with cpu icon
- **Status badge** — shows `activity` when running, otherwise `phase` (existing `getAgentDisplayStatus` logic)
- **Template badge** — template name with icon
- **Grove link** — clickable link to grove detail
- **Runtime Broker link** — clickable link to broker detail
- **Action buttons** — Open Terminal, Start/Stop, Configure (if phase=created), Delete (capability-gated)

**Error banner** — Rendered below header, above tabs, when `phase === 'error'` (unchanged behavior).

---

### Tab Bar

Uses existing Shoelace `sl-tab-group` / `sl-tab` / `sl-tab-panel` (already imported in `main.ts`).

```
[ Status ]  [ Configuration ]
```

Default active tab: **Status** (the dynamic view is what users check most often).

---

### Status Tab

Everything on this tab is dynamic — values that change during the agent's lifetime, updated in real-time via SSE.

#### Layout

```
+--[ Current State ]----------------------------------------------------+
|                                                                        |
|  Phase          Activity          Tool              Detail             |
|  [badge]        [badge]           {toolName}        {message}          |
|                                                                        |
+------------------------------------------------------------------------+

+--[ Current Task ]-----------------------------------------------------+
|                                                                        |
|  {taskSummary text, pre-wrapped}                                       |
|                                                                        |
+------------------------------------------------------------------------+

+--[ Limits & Usage ]---------------------------------------------------+
|                                                                        |
|  Turns                    Model Calls              Time Remaining      |
|  {current} / {max}        {current} / {max}        {hh:mm:ss}         |
|  [progress bar]           [progress bar]           [progress bar]      |
|                                                                        |
|  (section hidden entirely if no limits are configured)                 |
+------------------------------------------------------------------------+

+--[ Connectivity ]-----------------------------------------------------+
|                                                                        |
|  Last Seen                 Connection State                            |
|  {relative time}           {connected/disconnected}                    |
|                                                                        |
+------------------------------------------------------------------------+

+--[ Notifications ]----------------------------------------------------+
|                                                                        |
|  Your Notifications           Agent Notifications                      |
|  (list)                       (list)                                   |
|                                                                        |
+------------------------------------------------------------------------+
```

#### Section Details

**Current State** (card, info-grid)
| Field | Source | Realtime | Notes |
|---|---|---|---|
| Phase | `agent.phase` | Yes | Status badge with icon/color |
| Activity | `agent.activity` | Yes | Status badge; dash when not running |
| Tool | `agent.detail.toolName` | Yes | Mono font; shown only when present |
| Detail | `agent.detail.message` | Yes | Hidden when phase=error (shown in banner instead) |

**Current Task** (card, conditional)
| Field | Source | Realtime | Notes |
|---|---|---|---|
| Task Summary | `agent.detail.taskSummary \|\| agent.taskSummary` | Yes | Pre-wrapped text block. Card hidden if empty. |

**Limits & Usage** (card, conditional — hidden if no limits configured)

This section shows progress toward configured limits. Each item shows a fraction and a visual progress bar.

| Field | Source | Realtime | Notes |
|---|---|---|---|
| Turns | `agent.currentTurns` / `appliedConfig.inlineConfig.max_turns` | Yes | Progress bar, e.g. "42 / 100". **New field.** |
| Model Calls | `agent.currentModelCalls` / `appliedConfig.inlineConfig.max_model_calls` | Yes | Progress bar. **New field.** |
| Time Remaining | Computed: `maxDuration - (now - startedAt)` | Yes (15s tick) | Countdown display "1h 23m 45s". Shows progress bar based on elapsed/total. Red when < 10% remaining. |

Display rules:
- Each limit item is only shown if its max value is configured (> 0 or non-empty).
- If no limits are configured at all, the entire card is hidden.
- Progress bars use `--fabric-success` color normally, `--fabric-warning` at >75%, `--fabric-danger` at >90%.

**Connectivity** (card, info-grid)
| Field | Source | Realtime | Notes |
|---|---|---|---|
| Last Seen | `agent.lastSeen` | Yes | Relative time format ("2 minutes ago"). Tooltip shows absolute. |
| Connection State | `agent.connectionState` | Yes | Badge: connected=success, disconnected=danger. **New on frontend.** |

**Notifications** (card, unchanged from current)
- Two sub-sections: "Your Notifications" and "Agent Notifications"
- Mark-read capability on user notifications
- Truncation detection with tooltip

---

### Configuration Tab

Everything on this tab is static — values set at agent creation time that do not change during the agent's lifetime. This tab does NOT need real-time updates.

#### Layout

```
+--[ Identity ]----------------------------------------------------------+
|                                                                         |
|  Agent ID              Name                  Slug                       |
|  {uuid, mono}          {display name}        {slug, mono}               |
|                                                                         |
|  Created               Created By            Visibility                 |
|  {formatted date}      {email/name}          {private/team/public}      |
|                                                                         |
+-------------------------------------------------------------------------+

+--[ Harness & Model ]---------------------------------------------------+
|                                                                         |
|  Template              Harness               Auth Method                |
|  {template name}       {harnessConfig}       {harnessAuth}              |
|                                                                         |
|  Model                                                                  |
|  {model name}                                                           |
|                                                                         |
+-------------------------------------------------------------------------+

+--[ Runtime Environment ]-----------------------------------------------+
|                                                                         |
|  Runtime Broker        Runtime Type           Image                     |
|  {broker name, link}   {docker/k8s/apple}     {image name, mono}       |
|                                                                         |
|  Branch                                                                 |
|  {branch name}                                                          |
|                                                                         |
+-------------------------------------------------------------------------+

+--[ Limits ]------------------------------------------------------------+
|                                                                         |
|  Max Turns             Max Model Calls        Max Duration              |
|  {number or "None"}    {number or "None"}     {duration or "None"}      |
|                                                                         |
|  (section hidden if no limits configured)                               |
+-------------------------------------------------------------------------+

+--[ Initial Task ]------------------------------------------------------+
|                                                                         |
|  {task text, pre-wrapped}                                               |
|                                                                         |
|  (section hidden if no task was set at creation)                        |
+-------------------------------------------------------------------------+
```

#### Section Details

**Identity** (card, info-grid)
| Field | Source | Notes |
|---|---|---|
| Agent ID | `agent.id` | Monospace, copy-on-click |
| Name | `agent.name` | Display name |
| Slug | `agent.slug` | Monospace. **New on frontend.** |
| Created | `agent.createdAt` | Absolute date format |
| Created By | `agent.appliedConfig.creatorName` | Email or agent name. **New on frontend.** |
| Visibility | `agent.visibility` | Badge: private/team/public. **New on frontend.** |

**Harness & Model** (card, info-grid)
| Field | Source | Notes |
|---|---|---|
| Template | `agent.template` | Template name |
| Harness | `agent.harnessConfig` | e.g. "claude", "gemini" |
| Auth Method | `agent.harnessAuth` | e.g. "api-key", "vertex-ai" |
| Model | `agent.appliedConfig.inlineConfig.model` | e.g. "claude-sonnet-4-20250514". **New on frontend.** |

**Runtime Environment** (card, info-grid)
| Field | Source | Notes |
|---|---|---|
| Runtime Broker | `agent.runtimeBrokerName` | Link to `/brokers/{id}` |
| Runtime Type | `agent.runtime` | docker/kubernetes/apple. **New on frontend.** |
| Image | `agent.image` | Monospace. **New on frontend.** |
| Branch | `agent.appliedConfig.inlineConfig.branch` | Git branch. **New on frontend.** |
**Limits** (card, info-grid, conditional)
| Field | Source | Notes |
|---|---|---|
| Max Turns | `appliedConfig.inlineConfig.max_turns` | Number or "None" |
| Max Model Calls | `appliedConfig.inlineConfig.max_model_calls` | Number or "None" |
| Max Duration | `appliedConfig.inlineConfig.max_duration` | Human-readable string or "None" |

Hidden if all three are unset/zero.

**Initial Task** (card, conditional)
| Field | Source | Notes |
|---|---|---|
| Task | `agent.appliedConfig.task` | Pre-wrapped text. Hidden if empty. |

---

## Quick Actions

**Decision:** Remove the quick-action cards entirely. "Open Terminal" is promoted to a header action button (next to Start/Stop) since it's the most frequently used action. The disabled "View Logs" and "Settings" placeholder buttons are dropped until they are functional.

---

## Frontend Type Changes

The `Agent` interface in `web/src/shared/types.ts` needs new fields:

```typescript
interface Agent {
  // ... existing fields ...

  // New fields for Configuration tab
  slug?: string;
  image?: string;
  runtime?: string;
  visibility?: string;
  createdBy?: string;
  appliedConfig?: AgentAppliedConfig;

  // New fields for Status tab (limits tracking)
  currentTurns?: number;
  currentModelCalls?: number;
  startedAt?: string;
  connectionState?: string;
}

interface AgentAppliedConfig {
  image?: string;
  harnessConfig?: string;
  harnessAuth?: string;
  model?: string;
  profile?: string;
  task?: string;
  attach?: boolean;
  workspace?: string;
  creatorName?: string;
  inlineConfig?: AgentInlineConfig;
}

interface AgentInlineConfig {
  max_turns?: number;
  max_model_calls?: number;
  max_duration?: string;
  model?: string;
  branch?: string;
  task?: string;
  image?: string;
}
```

---

## Backend Changes Required

### 1. Add `currentTurns` and `currentModelCalls` to Hub status updates

The broker already receives these via `agent-limits.json` (updated on every `agent-end` and `model-end` hook). The fabrictool status handler needs to include these in `StatusUpdate` messages sent to the Hub.

**Files to modify:**
- `pkg/fabrictool/hub/client.go` — Add `CurrentTurns` and `CurrentModelCalls` to `StatusUpdate`
- `pkg/fabrictool/hooks/handlers/limits.go` — After incrementing, report to hub client
- `pkg/store/models.go` — Add `CurrentTurns`, `CurrentModelCalls` fields to `store.Agent`
- `pkg/hub/handlers.go` — Handle the new fields in status update endpoint
- Hub database migration — Add columns for current_turns, current_model_calls

### 2. Expose `appliedConfig` to frontend

`store.Agent.AppliedConfig` already exists and is persisted. It just needs to be included in the API JSON response (it already has `json:"appliedConfig,omitempty"` tags). The frontend needs to parse it.

**Sensitive data filtering:** The `AppliedConfig.Env` map may contain secrets. The Hub handler should strip `Env` from the response before sending to the frontend, or the frontend type should simply not include it.

### 3. Add `startedAt` timestamp

Track when the agent entered `running` phase. This can be derived from existing data (the `Updated` timestamp when phase transitions to `running`), or stored as a dedicated field.

**Simplest approach:** Add a `StartedAt` field to `store.Agent`, set it when phase transitions to `running` in the Hub's status update handler.

### 4. Expose additional fields in API response

Fields already on `store.Agent` that need to be mapped into the API response / frontend type:
- `slug` — already on store.Agent
- `image` — already on store.Agent
- `runtime` — already on store.Agent
- `visibility` — already on store.Agent
- `createdBy` — already on store.Agent
- `connectionState` — already on store.Agent

These just need corresponding fields added to the frontend `Agent` interface.

---

## Real-Time Update Scope

| Value | Update Mechanism | Frequency |
|---|---|---|
| Phase, Activity, Tool, Detail | SSE delta via agent status update | On every state change |
| Task Summary | SSE delta via agent status update | On task summary change |
| Current Turns, Model Calls | SSE delta via limits report | On every turn/model-call end |
| Last Seen | SSE delta via heartbeat | Every 30 seconds |
| Connection State | SSE delta via Hub connectivity monitor | On connect/disconnect |
| Time Remaining | Client-side computation | Every 15 seconds (existing interval) |
| Notifications | SSE delta via notification events | On notification create/ack |
| All Configuration tab values | Initial load only | Never (static) |

---

## Implementation Order

1. **Frontend layout** — Restructure `agent-detail.ts` with tabs, using existing data only. Status tab gets current state + task + connectivity + notifications. Configuration tab gets identity + harness + runtime info from existing fields.
2. **Expose existing backend fields** — Add `appliedConfig`, `slug`, `image`, `runtime`, `visibility`, `connectionState`, `createdBy` to frontend type and populate Config tab sections.
3. **Add limits tracking** — Backend: pipe `currentTurns`/`currentModelCalls` from fabrictool through broker to Hub. Frontend: add Limits & Usage section with progress bars.
4. **Add time remaining** — Backend: add `startedAt`. Frontend: compute countdown and render progress bar.
