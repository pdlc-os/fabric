# Design: Multi-Node Broker Dispatch over LISTEN/NOTIFY

**Branch:** `postgres/wave-b-integration`
**Date:** 2026-06-02
**Author:** broker-architect agent
**Status:** Approach approved by @ptone (2026-06-02). Scope: **message + agent
lifecycle dispatch only**; model is **"DB as state machine, NOTIFY as the
communications channel."** PTY, logs, and exec are out of scope (§10).
**Reviewers:** @ptone
**Implements:** the agreed "DB-state-machine + NOTIFY-signaled dispatch" approach.

Inputs: `RESEARCH-MESSAGE-DISPATCH.md`, `RESEARCH-BROKER-ROUTING.md`,
`pkg/hub/controlchannel.go`, `pkg/hub/controlchannel_client.go`,
`pkg/hub/events_postgres.go`, `pkg/hub/server.go`, `.design/postgres-strategy.md`.

---

## 1. Problem statement

A runtime broker opens **one** outbound WebSocket "control channel" to **one** hub
replica. That replica holds the live socket in an in-memory map
(`ControlChannelManager.connections`). Dispatch (`start`/`stop`/`message`/`exec`/…)
decides reachability purely from that local map
(`HybridBrokerClient.useControlChannel` → `manager.IsConnected`).

Behind a load balancer with N replicas, an API call lands on an arbitrary replica.
If the broker's socket is on Hub A but the call lands on Hub B:

- `IsConnected(brokerID)` is **false** on Hub B → falls back to HTTP at
  `broker.Endpoint`.
- For NAT'd / control-channel-only brokers (`Endpoint == ""` — the entire reason the
  control channel exists) the HTTP fallback **fails**, and worse, for messages the
  store row + SSE event were already written, so the UI shows "sent" while the agent
  never receives it (silent split-brain). Probability of failure ≈ (N−1)/N.

Two further defects compound this:

- **No broker→hub affinity** exists in the DB. A replica cannot even discover which
  peer owns a socket. (`runtime_brokers` has `status`/`connection_state`/
  `last_heartbeat` but no owning-replica column.)
- **`onDisconnect` status race** (`server.go:691`): the callback unconditionally
  stamps the broker `offline`. When a broker flaps A→B, Hub A's delayed disconnect
  can clobber Hub B's freshly-written `online` (last-writer-wins on
  `runtime_brokers.status`).

## 2. Design goals & non-goals

### 2.0 Hard constraint (maintainer-confirmed, 2026-06-02)

> **There is no hub-to-hub HTTP addressability.** A node generally cannot reach
> another node directly. A broker's reverse tunnel lands on an **arbitrary** node and
> stays sticky there. Therefore **Postgres LISTEN/NOTIFY is the only inter-node
> transport**, and dispatch must reach the socket-holding node *without any node
> addressing another*.

### 2.0.1 Model: DB as state machine, NOTIFY as the signal (maintainer-directed)

> **The DB holds the durable state/intent; NOTIFY is only the wakeup signal.** A
> dispatch is *not* "send a command over NOTIFY and hope a node is listening." It is
> "write the intent to the DB (durable), then NOTIFY so the socket-holding node wakes
> and **reconciles** DB intent → broker." If the NOTIFY is missed, or the owning node
> is down, the intent persists and is reconciled when a node next owns the socket
> (on (re)connect). This gives durability and at-least-once delivery **for free**, and
> makes the NOTIFY payload a tiny signal rather than the source of truth.

This reframes the response pattern too: the originator observes **DB state changes** (the
agent's 3-layer `phase`/`activity`/`detail`, or the message's `dispatch_state`) via the
events that already publish those transitions cross-node — not a bespoke RPC reply. A
**rolling timeout resets on each such change** (§6.4), so liveness, not a fixed clock,
bounds the wait.

Consequences baked into this design:
- Intent is **persisted to the DB** first; a NOTIFY on the global channel signals
  "reconcile broker X". The node holding the socket **self-selects** and reconciles.
  No node ever addresses a peer.
- Responses are **DB-state transitions observed via existing events** (`agent.<id>.status`
  phase changes; `agent.deleted`; message `dispatch_state`). No hub-to-hub reply path.
- The `connected_hub_id` affinity column is **not load-bearing for routing** —
  ownership is decided by who physically holds the socket. Affinity exists only to
  (a) fast-fail when *no* node owns the broker and (b) fix the `onDisconnect` race.
- **PTY, logs, and exec are out of scope.** PTY/log streams cannot ride NOTIFY and
  cannot be hub-to-hub reverse-proxied (no addressability); exec is an interactive
  request/response that does not fit the state-machine model. The only path for these
  is LB sticky-routing the client to the owning node — a separate problem (§10).

**Goals**
- A dispatch arriving at *any* node reaches the broker's socket, wherever it lives —
  with no node addressing another.
- Reuse the existing `PostgresEventPublisher` (LISTEN/NOTIFY, payload-offload,
  reconnect) — no new transport.
- **Durable + at-least-once** for in-scope dispatch: intent persists in the DB and is
  reconciled on (re)connect, so a missed NOTIFY or a down owner does not lose the
  command (§2.0.1).
- Fix the `onDisconnect` clobber race as a side effect of affinity tracking.
- Preserve today's fast path (local socket → tunnel) unchanged and at zero added
  latency.
- Preserve today's API semantics (start/stop "done" == broker accepted the command;
  see §6).
- **Support long, multi-step provisioning** (GKE pod cold-start, future runtime
  providers): reuse the existing 3-layer agent state (phase/activity/detail) for interim
  feedback and a **rolling timeout** that resets on each update, so duration is bounded by
  broker liveness, not a fixed clock (§6.4).

**In scope (commands):** `message` (incl. broadcast / `set[]`), and **agent
lifecycle**: `start`, `stop`, `restart`, `delete`, and create-time ops
(`create-with-gather`, `finalize-env`, `check-prompt`).

**Non-goals (this design)**
- **PTY / interactive streams** (`OpenStream`/`SendStreamData`), **logs**
  (`GetAgentLogs`), and **exec** (`ExecAgent`) — explicitly out of scope per maintainer
  (§10). They do not fit "DB as state machine" and/or cannot ride NOTIFY.
- Hub-to-hub HTTP of any kind (does not exist; §2.0).
- Replacing the HTTP-endpoint fast path for direct-mode brokers (kept as a fallback
  tier; rare under NAT'd deployments).

---

## 3. Architecture overview

```
                       shared Postgres (one DB, N hubs)
   ┌──────────────────────────────────────────────────────────────────────┐
   │  runtime_brokers (+ connected_hub_id, connected_session_id, …)         │
   │  fabric_event_payloads        (existing oversized-payload offload)      │
   │  LISTEN/NOTIFY channels:                                               │
   │     fabric_ev_global / fabric_ev_g_<grove>   (existing events)           │
   │     fabric_broker_cmd                        (NEW: dispatch commands)   │
   └──────────────────────────────────────────────────────────────────────┘
        ▲  ▲                         ▲  │                      ▲  │
        │  │ NOTIFY cmd              │  │ NOTIFY agent.status   │  │
        │  │                  LISTEN │  ▼                LISTEN │  ▼
   ┌────┴──┴─────┐            ┌──────┴────────┐           ┌─────┴───────┐
   │   Hub B     │            │     Hub A     │           │   Hub C     │
   │ (API entry) │            │ owns brokerX  │           │             │
   │             │            │ socket in-mem │           │             │
   │ instanceID= │            │ instanceID=   │           │ instanceID= │
   │   b2f1…     │            │   a9c3…       │           │   c7e0…     │
   └─────────────┘            └──────┬────────┘           └─────────────┘
                                     ║ WS control channel
                                ┌────╨─────┐
                                │ broker X │  (NAT'd; Endpoint == "")
                                │  agents  │
                                └──────────┘

Outbound dispatch (API on Hub B, socket on Hub A):
  1. Hub B handler → HybridBrokerClient.<Op>
  2. local IsConnected(X)? NO
  3. write DURABLE INTENT (broker_dispatch row / message.dispatch_state) + NOTIFY
     fabric_broker_cmd{broker_id:X}  — in ONE transaction (PublishTx)
  4. Hub A's signal-listener wakes, sees ownsLocally(X)==true, CAS-claims the intent,
     runs LOCAL tunnel <Op>, marks the intent done
  5. (for start/stop) Hub A sets phase + PublishAgentStatus  ── NOTIFY agent.status ──┐
  6. Hub B, which Subscribed to agent.<id>.status before step 3, wakes and returns ◄──┘
     to the API caller. (message = fire-and-forget: Hub B already returned 202 at step 3,
      durably. If NO node owns X, the intent persists and reconciles on X's reconnect.)
```

Two NOTIFY directions, both on infrastructure that already exists:

- **Command signal (NEW channel `fabric_broker_cmd`)** is a *tiny wakeup* — `{broker_id}`,
  no payload. The durable command lives in the DB. Every node receives the signal; only
  the socket-holder reconciles (ownership *self-selected*). Affinity (`connected_hub_id`)
  is a fast-fail hint, not the correctness gate; the reconnect-drain is the durability
  backstop.
- **Response (EXISTING channels `fabric_ev_*`)** is the already-published
  `AgentStatusEvent` (carries `Phase`) for lifecycle, or a slim `broker.dispatch.<id>`
  completion event for data-returning ops. The originating node subscribes and waits; the
  authoritative result is always the DB row.

---

## 4. Component 1 — Hub instance identity & broker affinity

### 4.1 Per-process instance ID (NEW — do **not** reuse `hubID`)

`hubID` (`config.ResolveHubID`) is **logical**: it is `HubID` from config if set,
else `sha256(hostname)[:12]`. It is used for **secret namespacing** and is explicitly
intended to be *stable* — operators may configure the *same* `HubID` across replicas
so they share a secret scope. Therefore `hubID` is **not safe** as an affinity key:
two replicas can legitimately share it.

Introduce a distinct **per-process instance ID**:

```go
// Server field, set once at construction.
instanceID string // e.g. uuid.NewString(); unique per hub process/boot
```

- Generated at boot (random UUID). Optionally seed from `POD_NAME`+boot-nonce in k8s
  for log readability, but uniqueness must not depend on hostname.
- Lives only in memory + the affinity column; never persisted to config.
- Exposed as `Server.InstanceID()`.

### 4.2 Schema change — `runtime_brokers`

Add three nullable columns (Ent schema `pkg/ent/schema/runtimebroker.go` + store model
`pkg/store/models.go` + migration):

| Column | Type | Meaning |
|---|---|---|
| `connected_hub_id` | `TEXT` null | instance ID of the replica currently holding the socket; `NULL` when no replica owns it |
| `connected_session_id` | `TEXT` null | the `BrokerConnection.sessionID` (uuid) of the owning socket — disambiguates reconnects |
| `connected_at` | `TIMESTAMPTZ` null | when the current owner registered the socket |

Reuse the existing `lock_version` optimistic-concurrency token (already on the row,
already CAS-looped by `UpdateRuntimeBrokerHeartbeat`).

> Dialect-neutral per `postgres-strategy.md` §6.4: `TEXT`/`TIMESTAMPTZ` work on both
> SQLite and Postgres. No Postgres-only types.

### 4.3 Affinity write paths (store methods)

Two new store methods, both modeled on the `UpdateRuntimeBrokerHeartbeat` CAS loop
(`project_store.go:755`):

```go
// ClaimRuntimeBrokerConnection sets affinity to this replica unconditionally
// (the newest connection wins — mirrors HandleUpgrade replacing an existing local
// socket). Bumps status->online + heartbeat in the same CAS write.
ClaimRuntimeBrokerConnection(ctx, brokerID, hubInstanceID, sessionID string) error

// ReleaseRuntimeBrokerConnection clears affinity ONLY IF it still names
// (hubInstanceID, sessionID) — compare-and-clear. Returns (cleared bool).
// If affinity already moved to another replica/session, it is a no-op and the
// caller MUST NOT stamp the broker offline (fixes the §1 race).
ReleaseRuntimeBrokerConnection(ctx, brokerID, hubInstanceID, sessionID string) (bool, error)
```

`ClaimRuntimeBrokerConnection` is called from `markBrokerOnline`
(`server.go:2456`) — pass the new `sessionID` out of `HandleUpgrade` (it already
generates one at `controlchannel.go:202`; thread it through the `onConnect` path).

### 4.4 The `onDisconnect` race fix (Component 5 in the brief)

Today (`server.go:691`):
```go
srv.controlChannel.SetOnDisconnect(func(brokerID string) {
    s.UpdateRuntimeBrokerHeartbeat(ctx, brokerID, store.BrokerStatusOffline) // UNCONDITIONAL
    ...
})
```

New: `SetOnDisconnect` must receive the **sessionID** of the connection that dropped
(extend the callback signature to `func(brokerID, sessionID string)` — `removeConnection`
already has the `*BrokerConnection`, so it can pass `hc.sessionID`). Then:

```go
srv.controlChannel.SetOnDisconnect(func(brokerID, sessionID string) {
    cleared, err := s.store.ReleaseRuntimeBrokerConnection(ctx, brokerID, s.instanceID, sessionID)
    if err != nil { /* log */ return }
    if !cleared {
        // Another replica (or a newer session on this replica) already owns the
        // socket. Do NOT mark offline — that would clobber the live owner.
        slog.Info("broker reconnected elsewhere; skipping offline stamp",
            "brokerID", brokerID, "staleSession", sessionID)
        return
    }
    // We were the owner and nobody replaced us: mark offline + publish.
    s.store.UpdateRuntimeBrokerHeartbeat(ctx, brokerID, store.BrokerStatusOffline)
    ... // provider status updates + PublishBrokerDisconnected (unchanged)
})
```

This is correct under A→B flap because the offline stamp is now gated on
"affinity still names *me* with *this* session". `HandleUpgrade` already closes+replaces
an existing **local** connection (`controlchannel.go:218`); the sessionID guard extends
that safety **across** replicas.

> Note `Shutdown()` (`controlchannel.go:544`) deliberately nils `onDisconnect` to avoid
> touching the DB during teardown. Keep that — on graceful shutdown we intentionally do
> **not** clear affinity (the broker will reconnect and re-claim; a brief stale-but-dead
> affinity row is handled by the liveness check in §5.3).

---

## 5. Component 2 & 3 — Command dispatch channel & command types

### 5.1 Channel choice

Single global channel **`fabric_broker_cmd`** (not per-broker). Rationale:

- Postgres channels have no wildcards; a per-broker channel
  (`fabric_broker_cmd_<id>`) would require every replica to `LISTEN` on the channel of
  every broker it *might* own — but a replica doesn't know which brokers will dial it
  next, so it would have to LISTEN on all of them anyway. A single channel is simpler
  and matches the `fabric_ev_global` precedent.
- Volume is low (dispatch is human-paced lifecycle/message ops, not data-plane traffic).
  One global channel is fine. Each node filters the signal by `ownsLocally(brokerID)`.

A dedicated signal-listener goroutine (mirroring `runListener` in `events_postgres.go`)
LISTENs on `fabric_broker_cmd`. On a signal for a broker it owns, it runs the reconcile
drain (§5.3). Implement as a sibling type **`PostgresCommandBus`** reusing the same
connect/reconnect/keepalive helpers (`connectListener`, `applyConnKeepalives`,
`nextBackoff`) — kept separate from `PostgresEventPublisher` so the event-fanout path and
the dispatch path are independently testable and pooled.

### 5.2 Intent lives in the DB; the NOTIFY is a tiny signal

Per the state-machine model (§2.0.1), the command **payload is not carried in the
NOTIFY**. The durable intent is written to the DB; the NOTIFY only says "broker X has
pending work, whoever owns it should reconcile."

**NOTIFY `fabric_broker_cmd` payload — a signal, not a command:**
```jsonc
{ "broker_id": "uuid", "kind": "dispatch" }   // optional "cmd_id" for log correlation
```
Tiny, never near the 8000-byte cap, never carries secrets. If the payload is ever lost
(LISTEN reconnect gap), correctness is unaffected — the intent is still in the DB and is
picked up by the next reconcile (NOTIFY-loss is just latency, not loss).

**Durable intent — two tables:**

1. **Messages reuse their existing row.** `store.Message` is already persisted *before*
   dispatch today. Add a `dispatch_state` (`pending|dispatched|failed`) +
   `dispatched_at`. No duplication; the message *is* the durable intent.

2. **Lifecycle uses a new `broker_dispatch` intent table:**

```sql
CREATE TABLE broker_dispatch (
  id          UUID PRIMARY KEY,
  broker_id   UUID NOT NULL,
  agent_id    UUID,                 -- null for project-scoped ops
  agent_slug  TEXT,
  project_id  UUID,
  op          TEXT NOT NULL,        -- start|stop|restart|delete|finalize_env|check_prompt|create
  args        TEXT,                 -- JSON; env/secrets/inlineConfig live here (see note)
  state       TEXT NOT NULL,        -- pending|in_progress|done|failed
  result      TEXT,                 -- JSON; for ops that return data (check_prompt, env-gather)
  claimed_by  TEXT,                 -- hub instanceID that reconciled it
  attempts    INT  NOT NULL DEFAULT 0,
  error       TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  deadline_at TIMESTAMPTZ
);
CREATE INDEX broker_dispatch_pending_idx ON broker_dispatch (broker_id, state);
```

Notes:
- `args` holds the bulky/secret-bearing fields (`resolvedEnv`, `resolvedSecrets`,
  `inlineConfig`, structured message bodies). They sit in a DB column, **not** in a
  NOTIFY payload — so secrets never appear in PG NOTIFY logs, and there is no 8000-byte
  limit to work around (the §6 oversized-offload concern for commands disappears
  entirely; offload remains only for the *event* path). On Postgres these can later
  become `JSONB` per strategy §6.4; `TEXT` keeps SQLite parity for now.
- `deadline_at` lets a late reconciler drop a command the caller already abandoned.
- Atomic publish: the intent row INSERT and the NOTIFY are issued in **one transaction**
  via `PublishTx` (events_postgres.go:236) — the signal is delivered only if the intent
  commits.

### 5.3 Routing decision — `HybridBrokerClient`

Because there is no hub-to-hub addressing (§2.0), routing is **not** "find the owner and
send to it". It is "run locally if I hold the socket, otherwise **broadcast** and let the
holder self-select". Affinity is consulted only to *fast-fail* — to avoid waiting out a
timeout when we can already tell nobody owns the broker.

```go
func (c *HybridBrokerClient) route(ctx, brokerID) routeDecision {
    if c.controlChannel.manager.IsConnected(brokerID) {
        return routeLocal          // I hold the socket → tunnel directly (unchanged)
    }
    // I don't hold it. Some OTHER node might. We cannot address that node, so:
    owner, alive := c.affinity.Lookup(ctx, brokerID) // reads runtime_brokers (hint only)
    switch {
    case owner != "" && alive:
        return routeForward        // NOTIFY-broadcast; the holder self-selects
    case brokerEndpointSet:        // direct-mode broker (hub→broker HTTP, not hub→hub)
        return routeHTTP           // existing fallback; rare under NAT'd deployments
    default:
        return routeUndeliverable  // no owner & no endpoint → typed retryable error
    }
}
```

Important: `routeForward` writes the **durable intent** (a `broker_dispatch` row, or a
`message.dispatch_state=pending`) and NOTIFYs the global `fabric_broker_cmd` channel in
one transaction; *every* node receives the signal but only the socket-holder reconciles.
The affinity lookup is a **hint** that *a* node owns the broker, so we should write intent
+ signal (and wait for the resulting state transition) rather than fast-fail. Even if the
hint is stale, correctness holds: a wrong "alive" costs one timeout (the intent stays
durable and reconciles later); a wrong "dead" is reaped by §7.1.

**Durability backstop — reconcile-on-connect.** Independent of any NOTIFY, when a broker
(re)connects to a node (`markBrokerOnline` / claim), that node runs a drain:
`SELECT … FROM broker_dispatch WHERE broker_id=$X AND state='pending'` plus pending
messages, and reconciles them. So even if *no* node owned the broker when the intent was
written (broker was down, or every NOTIFY was missed), the work executes the moment a
node next owns the socket. This is what makes the design durable + at-least-once without
a separate work queue (§2.0.1).

"alive" = the owning node is believed up. Since we can't ping it (no hub-to-hub HTTP),
liveness is inferred from `last_heartbeat` freshness on the broker row (the broker's own
HTTP heartbeat lands on any node and keeps the row fresh while the tunnel is up), backed
by the command timeout. This is OQ3: confirm `last_heartbeat`-freshness + timeout is
sufficient for v1 (recommended), given a dedicated `hub_instances` table buys us nothing
without hub-to-hub addressing.

### 5.4 Command types (in scope)

Each command is: **write durable intent → NOTIFY signal → owner reconciles → originator
observes the resulting DB-state transition.** The "observe" column is always an existing
or DB-backed state change, never a bespoke RPC reply.

| `op` | Durable intent | Owner reconcile (local tunnel) | Originator observes |
|---|---|---|---|
| `message` / broadcast / `set[]` | `message.dispatch_state=pending` (row already exists) | `MessageAgent`; set `dispatched_at` | fire-and-forget: return 202 once intent is durable (§6.1) |
| `start` | `broker_dispatch{op:start, args}` | `StartAgent`; set phase `Starting→Running` + `PublishAgentStatus` (today's behavior) | `agent.<id>.status` phase ∈ {running, error} |
| `stop` | `broker_dispatch{op:stop}` | `StopAgent`; set phase `Stopped` + publish | `agent.<id>.status` phase ∈ {stopped, error} |
| `restart` | `broker_dispatch{op:restart, args}` | `RestartAgent` | phase ∈ {running, error} |
| `delete` | `broker_dispatch{op:delete, args}` | `DeleteAgent` (idempotent; 404 ok); set state=done | `agent.deleted` event (already published cross-node) or dispatch state=done |
| `finalize_env` | `broker_dispatch{op:finalize_env, args}` | `FinalizeEnv`; write `result`; set phase | dispatch state→done (+ `result`) and/or phase |
| `check_prompt` / `create-with-gather` | `broker_dispatch{op:…, args}` | run tunnel; write `result` (bool / env-requirements) | dispatch state→done; originator reads `result` |

All in-scope ops map to a **DB-state transition** the originator can observe. The two
data-returning create-time ops (`check_prompt`, `create-with-gather`) write their result
into `broker_dispatch.result` — the result is *state*, consistent with the model, and is
durable/re-readable rather than a fire-once reply. **No separate `cmd-ack` RPC event and
no command-body offload are needed** (both removed from the earlier draft).

> **exec, logs, PTY are out of scope** (§10) — they are interactive request/response or
> streaming, not state transitions, and per maintainer are deferred.

---

## 6. Component 4 — Response = observing the DB-state transition

The originator never waits on a point-to-point reply. It **subscribes to the existing
event stream** (which already fans DB-state changes across nodes via
`PostgresEventPublisher`) *before* writing intent, then waits for the transition.

### 6.1 Fire-and-forget (message, broadcast, set[])

The message row is durable *before* dispatch (today, and still). So:
- Originator writes `dispatch_state=pending` + NOTIFY (one tx), returns **202** at once.
- Owner reconciles: tunnels the message, sets `dispatch_state=dispatched, dispatched_at`.
- **Loss visibility** (replaces today's silent split-brain): a sweep flags any row stuck
  `pending` beyond T. Because the row is durable, a broker that was offline gets its
  messages on reconnect-drain (§5.3) — they are delayed, never dropped. This is strictly
  better than today, where an undeliverable message surfaced as a *synchronous* error
  *after* the row + SSE were already written.

### 6.2 Lifecycle via the 3-layer agent state (start, stop, restart) — reuses existing events

The owner already calls `PublishAgentStatus` after dispatch
(handlers.go:1192/2345/2828), which NOTIFYs `agent.<id>.status` carrying the existing
**3-layer state** (events.go:97): `Phase` (top-level lifecycle), `Activity` (what the
agent is doing — "building", "pulling image", "waiting"), and `Detail` (untyped free-text
for broker/runtime-specific interim states). All three already cross nodes via the event
layer. The originating node watches **any change to the agent record**, not just the
terminal phase:

1. Originator `Subscribe("agent."+agentID+".status")` **before** writing intent.
2. Write `broker_dispatch` intent + NOTIFY (one tx).
3. Loop over `AgentStatusEvent`s:
   - **Any** change (phase/activity/detail) → forward progress: surface to the caller and
     **reset the rolling timeout** (§6.4).
   - **Terminal** phase (start/restart: {running, error}; stop: {stopped, error}) → done.
4. `error` phase → return the agent's `Message`. Rolling-timeout expiry (no update within
   the window) → dispatch **failed** (§6.4); the originator marks the outcome and returns.

> **Semantics preserved (OQ2 confirmed):** "done" == the owner accepted the command and
> published the resulting phase — *not* waiting for the harness to report truly-ready.
> The owner runs the local accept-and-publish sequence; the originator observes it.

### 6.3 Lifecycle/create ops without a phase (delete, finalize_env, check_prompt, gather)

Observe the **`broker_dispatch` row reaching a terminal state**. A slim completion event
`broker.dispatch.<id>` (subject → `fabric_ev_global`, reuses the existing publisher) is
emitted by the owner when it sets state `done|failed`; the originator subscribed to it
before writing intent and reads `result`/`error` from the row on wake. Because the
**authoritative result is the DB row**, a missed event is recoverable (bounded re-read at
timeout) — no point-to-point reply to lose. `delete` may instead observe the existing
`agent.deleted` event.

### 6.4 Rolling timeout on the 3-layer state (OQ2/OQ6 — resolved per maintainer + coordinator)

> Maintainer + coordinator: long providers (GKE pod cold-start; future runtimes) sit in
> schedule→image-pull→init for minutes, so a fixed wall-clock timeout is wrong. Instead:
> **a rolling timeout that resets on each interim state update.** Brokers are expected to
> update the sub-state (`activity`/`detail`) within the window; if a step needs longer the
> broker runs its own timer loop to keep emitting heartbeat-style `detail` updates. If no
> update arrives within the window, the dispatch is considered **failed**. Interim states
> are **untyped** (free-text `detail`) — no canonical sub-state set to define.

This is the whole model — it replaces the earlier inactivity-bound + absolute-cap +
provider-config machinery:

```
window := dispatchRollingTimeout            // single tunable; reset on ANY agent-record change
loop: select {
  case ev := <-sub:                          // ANY change to phase/activity/detail
      if terminal(ev.Phase) { return ev }    // running | stopped | error → done
      reset(window)                          // forward progress (incl. a detail heartbeat)
  case <-window:        return ErrDispatchFailed   // broker went silent → FAILED
  case <-ctx.Done():    return ctx.Err()
}
```

Properties:
- **Liveness-based, not duration-based.** A 10-minute GKE start succeeds as long as the
  broker keeps updating `detail`; a broker that dies mid-step fails fast (within one
  window), regardless of how long the step "should" take.
- **The broker owns the heartbeat.** A slow step (e.g. image pull) means the broker's own
  timer loop emits periodic `detail` updates ("pulling image… 40%"). This pushes the
  liveness contract to where the knowledge is, and needs no provider-specific config in
  the hub.
- **One knob.** `dispatchRollingTimeout` (a single default, e.g. 60–90s) rather than
  per-provider bounds. Providers express "I need longer" by *keeping the heartbeat going*,
  not by configuring a number.
- **Cross-node for free.** The waiting node (Hub B) watches the same `agent.<id>.status`
  events every node already receives; the owning node (Hub A) just keeps publishing the
  3-layer state as it does today.
- **Failure is authoritative.** On window expiry the originator marks the outcome
  (`broker_dispatch.state=failed` / agent `phase=error`) and returns failure. Because a
  well-behaved broker heartbeats while working, silence genuinely means stuck/dead.

> **Long-poll caveat (note, not a blocker):** a multi-minute synchronous request can
> exceed an L7 LB idle timeout *on the dispatch connection itself* (interim updates flow
> on the SSE/event stream, not on the blocked dispatch socket). If that bites, the op can
> return 202 once intent is durable + first update seen, and the client watches
> `agent.<id>.status` for the terminal phase — same state, just observed by the client
> instead of the hub. Flagged for implementation; not required by this design.

| Op | Wait model |
|---|---|
| message / broadcast | none — return 202 on durable intent |
| start / restart | rolling timeout on the agent record; terminal phase → done; window expiry → failed |
| stop / delete | rolling timeout (typically one window) — terminal phase / `agent.deleted` |
| finalize_env / check_prompt / gather | rolling timeout — `broker_dispatch` terminal state + `result` |

---

## 7. Error handling & edge cases

Because intent is **durable**, *messages* degrade to added latency (never lost), while
*lifecycle* ops follow the rolling-timeout contract (silence ⇒ failed + retryable).

| Case | Behavior |
|---|---|
| **Rolling-timeout expiry** (no agent-record update within the window — broker stuck/dead mid-step) | The in-flight dispatch is **failed**: originator marks `broker_dispatch.state=failed` / agent `phase=error` and returns 503. This is the §6.4 contract — a well-behaved broker heartbeats `detail` while working, so silence is genuine failure. |
| **No node owns the broker** (broker offline) | Intent is written `pending` and persists. **Message:** return 202 — delivered on reconnect-drain (§5.3), never dropped. **Lifecycle:** originator can see the broker is offline up front (affinity/heartbeat) and return retryable immediately rather than wait a full window; the `pending` intent may be reaped (§7.1) or left for reconnect-drive per op. |
| **Owner believed alive but is actually dead** (crashed without clearing affinity) | No status updates reach the originator → rolling window expires. **Message:** stays `pending`, reconciled when a node next owns the socket. **Lifecycle:** failed + retryable (above). Stale affinity reaped by §7.1. |
| **Owner alive but socket just dropped there** | Reconciler sees `ownsLocally==false` → ignores. Intent stays `pending`; broker re-dials and the new owner drains it (message) or the user retries (lifecycle). |
| **Two nodes both think they own it** (flap mid-signal) | The `broker_dispatch` claim is a CAS (`state pending→in_progress WHERE state='pending'`), so exactly one node executes a given intent. Messages: dedupe on `dispatch_state` CAS likewise. No double-execution. |
| **NOTIFY/intent write fails** (pool saturated) | It's one transaction (`PublishTx`): either both the intent row and the signal commit, or neither. On failure the handler returns 503 retryable with **no partial state**. Bounded by `publishTimeout` (5s). |
| **Large args** (env/secrets/inlineConfig) | Live in the `broker_dispatch.args` DB column — no NOTIFY size limit, no payload-offload table, no secrets in PG logs. |
| **Completion/phase event missed** (subscriber buffer overflow or originator crash) | The authoritative result is the **DB row** (phase / `dispatch_state` / `broker_dispatch.state`+`result`). On timeout the originator may re-read it; the command itself already ran. At-least-once; all in-scope ops are idempotent (start/stop/restart/delete are broker-idempotent — `DeleteAgent` allows 404; message dedupes on `dispatch_state`). |
| **Reconcile runs after the caller gave up** (`deadline_at` passed) | Reconciler may skip (lifecycle) or still deliver (message — better late than never). Correctness relies on the originator's own timeout, not the reconciler's clock. |
| **Completion event lands on a non-originating node** | Harmless — the event is broadcast to all nodes via `fabric_ev_global`; only the node with a live `Subscribe` for that agent/dispatch matches; others ignore it. |

### 7.1 Stale-affinity reaping

A recurring **singleton** job (reuse `RegisterRecurringSingleton` /
`pg_try_advisory_lock`, precedent `server.go:1858`) clears `connected_hub_id` for
brokers whose `last_heartbeat` is older than `2 × heartbeatInterval` AND whose
`connected_hub_id` is non-NULL. This bounds how long a crashed owner's affinity misleads
`route` into `routeForward` (after which `route` falls to `routeUndeliverable`, i.e. a
durable `pending` intent + retryable status). The same job can mark `broker_dispatch`
rows stuck `in_progress` past `deadline_at` back to `pending` (re-drive) or `failed`.

### 7.2 Routing order (summary)

```
local socket (tunnel)                    ── fastest, unchanged
  └─ else a node owns it → write durable intent + NOTIFY signal → owner reconciles
       └─ else broker.Endpoint set → HTTP (direct-mode brokers; existing; rare under NAT)
            └─ else → write durable intent (pending) + retryable status
                      → reconciled on broker reconnect-drain (never silent)
```

The HTTP tier is retained for direct-mode brokers (`Endpoint` set, reachable hub→broker —
distinct from the nonexistent hub→hub path). Whether any production broker uses it is OQ1;
under pure NAT it is never taken.

---

## 8. Data model & migration summary

```sql
-- 1. Broker affinity (fixes the disconnect race; hint for routing).
ALTER TABLE runtime_brokers
  ADD COLUMN connected_hub_id     TEXT,
  ADD COLUMN connected_session_id TEXT,
  ADD COLUMN connected_at         TIMESTAMPTZ;
-- lock_version already present and CAS-looped.

-- 2. Durable lifecycle/create intent (the state machine).
CREATE TABLE broker_dispatch ( … );   -- see §5.2

-- 3. Message delivery state (messages are already durable rows).
ALTER TABLE messages
  ADD COLUMN dispatch_state TEXT NOT NULL DEFAULT 'pending',  -- pending|dispatched|failed
  ADD COLUMN dispatched_at  TIMESTAMPTZ;
```

- Ent: add the affinity fields to `pkg/ent/schema/runtimebroker.go`, a new
  `BrokerDispatch` schema, and the message fields — all dialect-neutral (`TEXT`/
  `TIMESTAMPTZ`, no Postgres-only annotations) per strategy §6.4.
- Store model: add fields to `store.RuntimeBroker` (`models.go:281`) and `store.Message`;
  add `BrokerDispatch` model + store methods (insert/claim-CAS/complete/drain).
- New NOTIFY channel `fabric_broker_cmd` (no DDL; channels are ephemeral). The existing
  `fabric_event_payloads` table is **not** needed by dispatch (args live in
  `broker_dispatch.args`); it stays in use only by the event path.
- New in-memory `Server.instanceID`.

No SQLite-path behavior changes: single-process SQLite always takes the local fast path
(`IsConnected==true`), so the intent tables are written-through but routing never forwards
and the reconcile-drain simply runs locally. The affinity columns still fix the
disconnect race harmlessly.

---

## 9. Sequence diagrams

### 9.1 `message` (durable, fire-and-forget) — socket on Hub A, API on Hub B

```
User→LB→Hub B: POST /agents/{id}/message
Hub B: BEGIN tx: persist Message (dispatch_state=pending)
                 + PublishUserMessage (SSE)               [unchanged, cross-node]
                 + NOTIFY fabric_broker_cmd{broker_id:X}   [signal only]  COMMIT
Hub B: 202 Accepted  ───────────────────────────────────────► User (immediate, durable)
Hub A: signal-listener wakes; ownsLocally(X)=yes
Hub A: drain: SELECT messages WHERE broker_id=X AND dispatch_state='pending'
Hub A: CAS dispatch_state pending→dispatched; MessageAgent (local tunnel) → broker → agent
   (broker offline at notify time? → no owner acts; row stays pending;
    delivered when X reconnects and its new owner runs the same drain — never lost)
```

### 9.2 `start` — observe phase, with intermediate sub-states (long provider, e.g. GKE)

```
User→LB→Hub B: POST /agents/{id}/start
Hub B: Subscribe("agent.{id}.status")                     [BEFORE writing intent]
Hub B: BEGIN tx: INSERT broker_dispatch{op:start, args, state=pending}
                 + NOTIFY fabric_broker_cmd{broker_id:X}   COMMIT
Hub A: signal-listener; ownsLocally=yes; CAS-claim dispatch row pending→in_progress
Hub A: StartAgent local tunnel → broker accepts; mark dispatch done
Hub A: broker/provider advances; each step → PublishAgentStatus ── NOTIFY agent.{id}.status ─┐
       phase=starting; activity="pulling image"; detail="… 40%" (broker heartbeats detail)   │
Hub B: <-sub: ANY change (phase/activity/detail) → surface to caller + RESET rolling window ◄─┤
Hub B: <-sub: phase==running (terminal) → 200 OK  ◄──────────────────────────────────────────┘
   phase==error → 502 + agent.Message
   no update within rolling window → dispatch FAILED: mark phase=error / dispatch.state=failed, 503
   (broker is expected to keep emitting detail while working; silence ⇒ stuck/dead)
```

### 9.3 `check_prompt` / env-gather (data result, no phase)

```
Hub B: Subscribe("broker.dispatch.{id}")                  [BEFORE writing intent]
Hub B: BEGIN tx: INSERT broker_dispatch{op:check_prompt, state=pending}
                 + NOTIFY fabric_broker_cmd{broker_id:X}   COMMIT
Hub A: ownsLocally=yes; CAS-claim; run local tunnel → result (bool / env-requirements)
Hub A: UPDATE broker_dispatch SET state=done, result=… ; Publish broker.dispatch.{id}
Hub B: <-sub ; read result from the dispatch row → return to caller
        (event missed? re-read row at timeout — result is authoritative DB state)
```

### 9.4 Broker flap A→B (disconnect race fix)

```
t0  broker X socket on Hub A: connected_hub_id=a9c3, session=s1
t1  LB reshuffle; X re-dials, lands on Hub B
t2  Hub B HandleUpgrade(session=s2); ClaimRuntimeBrokerConnection(X, b2f1, s2)
       → row now (b2f1, s2), status=online
t3  Hub A's old socket finally errors; onDisconnect(X, s1)
       → ReleaseRuntimeBrokerConnection(X, a9c3, s1): affinity is (b2f1,s2) ≠ (a9c3,s1)
       → cleared=false → SKIP offline stamp  ✅ (today this clobbered B's online)
```

---

## 10. Out of scope (maintainer-confirmed): PTY, logs, exec

These are **not** part of this work item. They do not fit "DB as state machine":

- **PTY / interactive streams** (`OpenStream`/`SendStreamData`/`ResizeStream`/
  `CloseStream`) — high-frequency, ordered, back-pressured bytes. NOTIFY is wrong for
  this (8000B cap, no flow control, fan-out to all nodes).
- **Logs** (`GetAgentLogs`) and **exec** (`ExecAgent`) — request/response bodies /
  streaming, not state transitions.

Why they can't simply reuse this design: the response/stream must originate from the
*owning* node, and **there is no hub-to-hub transport** (§2.0) to relay it. So the only
viable future approach is **sticky client routing** — terminate the user's stream on the
owning node directly:

- **LB session affinity** keyed so the PTY/logs/exec client lands on the node that owns
  the broker (e.g. cookie/route keyed to broker or agent), **or**
- introduce **hub addressability** (a `hub_instances(instance_id, endpoint, last_seen)`
  table + reachable internal endpoints) so an entry node can reverse-proxy the
  upgrade/stream to the owner. This is a larger change explicitly deferred.

Until one of those lands, PTY/logs/exec work only when the client happens to hit the
owning node. **Document as a known multi-node limitation; gate "full multi-node GA" on a
separate streaming design.**

---

## 11. Open questions for the maintainer (@ptone)

Asked one at a time per protocol; answers folded back into this doc as received.

- **OQ1 — RESOLVED (2026-06-02).** Maintainer reframed: there is **no hub-to-hub HTTP**;
  broker tunnels are sticky to an arbitrary node; **NOTIFY is the only inter-node
  transport**. Folded into §2.0. (Whether direct-mode `broker.Endpoint` brokers exist at
  all is a minor optimization; the design does not depend on it.)

- **Scope — RESOLVED (2026-06-02).** Maintainer: **message + agent lifecycle only**;
  **PTY, logs, exec out of scope**; model is **"DB as state machine, NOTIFY as the
  communications channel."** Folded into §2.0.1, §5, §6, §10.

- **OQ4 (durability) — RESOLVED by the state-machine model.** Intent is durable in the
  DB and reconciled on broker reconnect-drain (§5.3), so an owner being down delays but
  never loses a command. No separate ephemeral queue. *Confirm this is the intended
  durability bar* (vs. also persisting a per-attempt audit log).

- **OQ5 (loss visibility) — RESOLVED by the state-machine model.** Messages carry
  `dispatch_state` + `dispatched_at` on the existing row; lifecycle carries
  `broker_dispatch.state`. Sweep in §7.1. *Confirm column placement is acceptable.*

- **OQ2 — RESOLVED (2026-06-02).** Contract confirmed (owner accepts + publishes phase
  = done; not harness-ready). Timeouts: long providers (GKE, future runtimes) need more
  time + interim feedback, handled by a **rolling timeout on the existing 3-layer agent
  state** that resets on each interim update (see OQ6 below for the full resolution).
  Folded into §6.2, §6.4, §9.2.

- **OQ6 — RESOLVED (2026-06-02, maintainer + coordinator).** Reuse the existing 3-layer
  state (phase/activity/**detail**); interim states are **untyped** free-text in `detail`
  (no canonical set to define). Timeout is a **rolling window that resets on each interim
  update**; brokers heartbeat `detail` (own timer loop) while a step runs; no update within
  the window ⇒ dispatch **failed**. Folded into §6.2, §6.4, §9.2. (Async-202 noted only as
  an LB-idle escape hatch, §6.4.)

Still genuinely open (lower stakes; sensible default proposed — non-blocking):

- **OQ3 (liveness signal):** Is `last_heartbeat`-freshness + the rolling dispatch timeout
  sufficient to decide "a node owns this broker" for v1 (recommended), or introduce a
  `hub_instances` heartbeat table now? (Note: `hub_instances` buys nothing for *this*
  scope without hub-to-hub HTTP; it only pays off for the deferred PTY/logs/exec work in
  §10, so deferring it with that work is the natural call.)

---

## 12. Implementation sequencing (suggested)

1. **Phase 1 — affinity + race fix (independently shippable).** Per-process
   `instanceID`; affinity columns on `runtime_brokers`; `ClaimRuntimeBrokerConnection` /
   `ReleaseRuntimeBrokerConnection` (CAS compare-and-clear); thread `sessionID` through
   `markBrokerOnline` and the `onDisconnect(brokerID, sessionID)` callback. **Fixes the
   disconnect-race correctness bug today**, with no dependency on dispatch.
2. **Phase 2 — state-machine substrate.** `broker_dispatch` table + store methods
   (insert / CAS-claim / complete / drain) and `messages.dispatch_state`/`dispatched_at`.
   `PostgresCommandBus`: a listener on `fabric_broker_cmd` reusing the events_postgres
   connect/keepalive/reconnect helpers; the **reconcile-on-connect drain** wired into
   `markBrokerOnline`.
3. **Phase 3 — message dispatch.** `route` in `HybridBrokerClient`; transactional
   intent+signal for `message`/broadcast/`set[]`; owner drain → tunnel → mark dispatched.
   (Fixes the originally-reported message split-brain.)
4. **Phase 4 — lifecycle dispatch.** `start`/`stop`/`restart`/`delete` via
   `broker_dispatch` + phase/`agent.deleted` observation; then the create-time data ops
   (`finalize_env`, `check_prompt`, `create-with-gather`) via `broker_dispatch.result` +
   the `broker.dispatch.<id>` completion event.
5. **Phase 5 — hardening.** Stale-affinity / stuck-`in_progress` reaper singleton;
   `pending`-message sweep + metrics.
6. **Deferred — PTY / logs / exec.** Separate streaming design (§10). Out of scope.

Phase 1 is independently shippable and fixes a real correctness bug today. Phases 2–3
deliver the originally-reported message-dispatch fix; Phase 4 completes lifecycle.
