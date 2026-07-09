# Fabric FAQ

> Practical Q&A about what Fabric is, how it runs agents, and how to integrate an
> existing Claude Code–based harness with it.
>
> **Provenance:** distilled from the captured [Code Wiki](./README.md) plus direct
> reads of the source at commit `b11448f8f01fdcf12deab96204417d87f8d46ab3`.
> Items tagged **[verified in source]** were confirmed against the actual code;
> items tagged **[design/general]** reflect design intent or general platform
> knowledge and should be re-checked against current source before relying on them
> for a production decision.

---

## Table of Contents

- [1. What agentic runtime does Fabric use?](#1-what-agentic-runtime-does-fabric-use)
- [2. What is the intent of Fabric? How is it better than a harness built on AgentCore?](#2-what-is-the-intent-of-fabric-how-is-it-better-than-a-harness-built-on-agentcore)
- [3. Does it run on GCP, or can I run it on AWS / Lyzr AI?](#3-does-it-run-on-gcp-or-can-i-run-it-on-aws--lyzr-ai)
- [4. I have multiple agents (own LLMs/tools/skills) running inside Claude Code. How does Fabric wrap around this?](#4-i-have-multiple-agents-own-llmstoolsskills-running-inside-claude-code-how-does-fabric-wrap-around-this)
- [5. Does Fabric offer a web UI to run agents? How do I interact with them?](#5-does-fabric-offer-a-web-ui-to-run-agents-how-do-i-interact-with-them)
- [6. In my harness, users interact with Claude. How do I do that inside Fabric?](#6-in-my-harness-users-interact-with-claude-how-do-i-do-that-inside-fabric)
- [7. Can multiple users share one live attached Claude Code session?](#7-can-multiple-users-share-one-live-attached-claude-code-session)
- [8. How do I set up "multiple users collaborate with one shared Claude"?](#8-how-do-i-set-up-multiple-users-collaborate-with-one-shared-claude)
- [9. Can I run Claude Code with `--dangerously-skip-permissions` and attach via the web terminal?](#9-can-i-run-claude-code-with---dangerously-skip-permissions-and-attach-via-the-web-terminal)
- [10. I have a `jwn-claude` wrapper script (symlinked as `claude`). Can I keep it in Fabric?](#10-i-have-a-jwn-claude-wrapper-script-symlinked-as-claude-can-i-keep-it-in-fabric)
- [11. What does each layer of the concentric harness stack (Fabric → Claude Code → PDLC) do?](#11-what-does-each-layer-of-the-concentric-harness-stack-fabric--claude-code--pdlc-do)
- [12. Why keep the layers separate? Benefits and overlaps to manage](#12-why-keep-the-layers-separate-benefits-and-overlaps-to-manage)

---

## 1. What agentic runtime does Fabric use?

"Agentic runtime" maps to **two different layers** in Fabric — clarify which you mean.

### a) Agent harnesses (the actual LLM coding agents) — **[verified in source]**

Fabric is deliberately **harness-agnostic**; there is no single hardcoded agent. LLM
agents are integrated as opt-in **harness bundles** under `harnesses/<name>/`:

- **Gemini CLI** (`harnesses/gemini-cli`, first-class base image `image-build/gemini`)
- **Claude Code** (`harnesses/claude`)
- **Codex** (`harnesses/codex`)
- **GitHub Copilot** (`harnesses/copilot`)
- **Antigravity** (`harnesses/antigravity`)
- **OpenCode** (`harnesses/opencode`)
- **Amp** (example: `examples/amp` — script-only harness, no core changes)
- **ADK** (Agent Development Kit) (`examples/adk_fabric_agent`)
- **Hermes** (referenced in MCP translation)

Each harness encapsulates that agent's env vars, command construction, auth, and MCP
translation via `fabric_harness.py`. The harness type is resolved at runtime
(ContainerScript / declarative-generic / generic). Agent + model are selected per
agent via **template + harness-config**.

### b) Container runtime (what Fabric internally calls "runtime") — **[verified in source]**

The execution substrate for the agent container, abstracted in `pkg/runtime`:

- **Docker**
- **Podman**
- **Apple Container** (`apple_container.go`, macOS-native virtualization)
- **Kubernetes** (`k8s_runtime.go`, experimental → evolving to the agent-sandbox CRD standard)

---

## 2. What is the intent of Fabric? How is it better than a harness built on AgentCore?

### Fabric's intent — **[design/general]**

Orchestrate **many concurrent LLM *coding* agents**, each isolated in its own
container with a dedicated git worktree, managing their full lifecycle across local
or distributed compute. Five stated goals: **parallelism, isolation
(identity/creds/config), context management (git worktrees), specialization (role
templates), and interactivity (detach/attach with human-in-the-loop).**

Load-bearing design choices, all serving *code agents at team scale*:
- Git worktree per agent (`../.fabric_worktrees/...` + feature branch) → N agents edit one repo without conflict.
- Harness-agnostic (Gemini/Claude/Codex/Copilot/… behind one `Harness` interface).
- Hub + Runtime Broker split → centralized state, distributed execution across machines/clusters.
- Inter-agent messaging + orchestrator patterns (fan-out, sequential, coordinator-relay); a "lead agent" spawns sub-agents within its project.
- Container-runtime-agnostic.

**One line:** Fabric is a manager for a fleet of collaborating coding agents working on real git repos.

### Fabric vs. an AgentCore-built harness — **[general — AgentCore specifics not verified]**

They are **not the same category**, which is the crux of the decision.

**Amazon Bedrock AgentCore** = a **managed, general-purpose** cloud platform of agent
building blocks (serverless Runtime with session isolation, managed Memory, Gateway
for MCP tools, Identity, Observability, sandboxed Code Interpreter/Browser). Framework-
agnostic; not opinionated about coding agents or git.

| Dimension | Fabric | Harness on AgentCore |
|---|---|---|
| Sweet spot | Concurrent **coding** agents on git repos | Any agent workload; you define the shape |
| Git/worktree isolation | First-class, built-in | You build it yourself |
| Multi-harness (Claude/Gemini/Codex/…) | Built-in abstraction | You wire up each yourself |
| Infra ops | You self-host Hub/Brokers (or K8s) | AWS-managed, scale-to-zero |
| Vendor lock-in | None (self-hosted, OSS) | AWS |
| Human-in-the-loop attach | `fabric attach` into live tmux session | Build it |
| Maturity | Pre-release / alpha | GA managed service |

- **Fabric wins** when your problem *is* the coding-fleet problem: many agents, real
  repos, worktree isolation, attach-to-debug, mixing agent vendors, self-hosted / no
  lock-in. It gives you that plumbing out of the box.
- **AgentCore wins** for a **general** agent where you want someone else to run the
  infra (managed memory, autoscaling, identity/observability, native browser/code-
  interpreter tools, AWS integration).
- Not mutually exclusive — you could even run a harness's agent process inside
  AgentCore Runtime, since Fabric is harness-agnostic.

---

## 3. Does it run on GCP, or can I run it on AWS / Lyzr AI?

### Core is cloud-agnostic — **[design/general]**

Fabric is a self-hosted Go binary + containers. Nothing forces GCP:
- **Runtimes:** Docker, Podman, Apple Container, Kubernetes (any cluster — EKS, GKE, on-prem).
- **Hub state:** SQLite locally (Postgres for production). Not a GCP-only datastore.
- **Deployment:** Hub + Runtime Brokers are just processes (wiki shows them under `systemd` on a plain VM — that VM can be EC2, GCE, or a laptop).

**→ Yes, you can run it on AWS** (Hub on EC2/EKS, register Runtime Brokers).

### But GCP is the "paved road" — **[design/general]**

Optional integrations assume Google Cloud; on AWS you replace or skip these:

| GCP-coupled feature | AWS reality |
|---|---|
| Cloud Build image backend | Use `local-docker`/`local-podman` backends, or your own CI |
| Artifact Registry (image push) | Point at ECR; setup scripts target Artifact Registry |
| GCE metadata server emulation | Only if agents call GCP services; skippable on AWS |
| Vertex AI skill registry (`gcp-skill://`) | Use GitHub (`gh://`) or Hub skill resolvers |
| Google Cloud Logging (agent-viz) | agent-viz expects GCP log export format |
| Cloud Run / GCE deploy scripts, GCP Secret Manager | Substitute AWS equivalents manually |

None are load-bearing for core orchestration — they're conveniences for GCP shops.

> **Caveat:** not yet verified that the Runtime Broker *startup path* has zero hard
> GCP assumption. Worth a code check before committing to AWS.

### Lyzr AI — different category, won't work as a host — **[general — Lyzr specifics not verified]**

Lyzr is an agent-**framework**/SaaS, **not** container/compute infrastructure. Fabric
must own the container lifecycle (spawn containers, mount git worktrees, run
`fabrictool` as PID 1, exec/attach). Lyzr doesn't provide that primitive.

- **You cannot host Fabric on Lyzr** the way you can on AWS/GCP/K8s.
- Only viable relationship is *integration* (wrap a Lyzr agent as a Fabric harness, or
  have a Fabric agent call a Lyzr API) — custom glue, not a supported path.

---

## 4. I have multiple agents (own LLMs/tools/skills) running inside Claude Code. How does Fabric wrap around this?

### The key mismatch: two orchestration layers — **[design/general]**

Fabric's unit of orchestration is **1 Fabric agent = 1 container = 1 Claude Code
process** (via the `claude` harness). If your agents run *inside* one Claude Code
process (Task-tool subagents / in-process SDK orchestration), Fabric **cannot see or
decompose them** — your whole harness looks like *one* agent to Fabric. Two layers:

- **Fabric's layer** = multi-*container* (isolated, own worktree/creds, cross-container messaging).
- **Your layer** = multi-*agent-in-one-process* (shared context/workspace, in-process handoff).

This split gives two paths:

### Path A — Wrap the whole harness as ONE Fabric agent (little/no refactor)
- Package your harness into a Claude Code container (`FROM fabric-base`), one template.
- Your multi-agent logic runs unchanged inside it.
- **Fabric adds:** container isolation, git worktree at `/workspace`, lifecycle
  (start/stop/resume/delete), `fabric attach`, Hub visibility, resource limits,
  secret/auth injection.
- **Fabric does NOT** orchestrate/visualize/message your *internal* agents individually.
- **Use when** agents share context/workspace and in-process orchestration already works.

### Path B — Make each agent a Fabric agent (moderate refactor)
- Express each agent as a template + wire orchestration to Fabric primitives.
- **Fabric adds:** true parallelism, worktree-per-agent isolation, the orchestrator
  pattern (lead agent spawns/manages workers via CLI with
  `ScopeAgentCreate`/`ScopeAgentLifecycle`), inter-agent `fabric message`, per-agent
  observability.
- **Use when** agents are meant to be independent, parallel units on the same repo
  (Fabric's sweet spot).

### What maps cleanly (either path — you don't rewrite agent logic)

| Your artifact | Fabric mapping | Effort |
|---|---|---|
| Per-agent LLM/model | Template `model` field + harness-config (alias tiers / project defaults / CLI flag) | Low |
| Tools (MCP servers) | Universal MCP config; `fabric_harness.py` (`apply_mcp_servers_simple`) translates to Claude's native format | Low |
| Skills | Keep as Claude Code skills in-container, OR convert to Fabric `SKILL.md` (agentskills.io, auto-injected) | Low–Med |
| System prompts / instructions | `system-prompt.md` (persona) + `agents.md` (behavior); projected into Claude's native file, preserving content outside Fabric-managed blocks | Low |
| Agent identity/role | Template dir per role, inheriting built-in `default` | Low |
| In-process orchestration | Path A: unchanged. Path B: → `fabric message` + orchestrator pattern | **The only real refactor** |

The Claude harness also handles Claude-specific plumbing (OAuth token capture from
tmux scrollback, iptables egress control, MCP translation) — you don't build that.

**Bottom line:** the refactor is **not significant for your agent logic** (prompts,
tools, skills, per-agent LLM configs carry over). The only thing that changes is
*who owns orchestration*. Deciding question: **are your agents isolated/parallel
units (→ Path B) or in-process collaborators sharing one context (→ Path A)?**
A hybrid (nested: a Fabric `claude` agent internally runs Claude Code subagents) is
also legitimate.

---

## 5. Does Fabric offer a web UI to run agents? How do I interact with them?

### The Web UI — **[verified in source / design]**

Fabric ships a **Lit + Shoelace SPA** (`web/`), served by the Go `fabric` binary when
started with `--enable-web` (same binary that runs the Hub — no separate service).

It lets you:
- Manage projects, agents, brokers; create/start agents from templates.
- Watch **real-time state** (`StateManager` + `SSEClient` on `/events`, delta merging, no reloads).
- **Agent detail page:** Header + two tabs — **Status** (live: Phase, Activity, Tool,
  Detail, Current Task, Limits & Usage, Connectivity, Notifications) and
  **Configuration** (static: Identity, Harness & Model, Runtime Environment, Limits,
  Initial Task).
- Use an **in-browser terminal** (WebSocket PTY to the container).
- View logs/messages/files (`FabricAgentLogViewer`, `FabricAgentMessageViewer`, JSON
  browser, file browser/editor with syntax highlighting + Markdown).
- Admin: groups, integrations, skill registries; light/dark theming.

### How you interact with agents (all channels)

| Channel | How |
|---|---|
| Web UI | Create/start/stop, watch live status via SSE, in-browser terminal, view logs/messages/files |
| CLI — attach | `fabric attach <agent>` → live interactive session (local via runtime; Hub-managed via WebSocket into persistent tmux) |
| CLI — messaging | `fabric message` to `agent:<name>`, `user:<email>`, or a group; flags `--interrupt`, `--wake`, `--raw`, `--attach <file>`, scheduled `--in`/`--at` |
| CLI — inspect | `fabric look <agent>` (recent terminal/UI without attaching); `fabric list` |
| CLI — lifecycle | `create` / `start` / `stop` / `resume` / `delete` / `cdw` |
| Status signals (agent → you) | `ask_user`, `blocked`, `task_completed` via `fabrictool status` → surface as notifications/prompts |
| Chat bridges | Google Chat, Slack, Discord, Telegram — bidirectional agent↔chat + slash commands |
| A2A / programmatic | `fabric-a2a-bridge` (A2A endpoints); Hub REST API + WebSocket for custom |

### Caveats — **[design/general]**
1. **Alpha maturity** — the UI is real but rough vs. a polished product, and evolving.
2. The web UI orchestrates **Fabric agents (containers)**, not your in-process sub-agents.
   Path A → UI shows *one* agent; Path B → UI shows/controls each individually.
3. **You host it** — served by your Hub via `--enable-web`; no hosted SaaS dashboard.

---

## 6. In my harness, users interact with Claude. How do I do that inside Fabric?

### Mechanism — **[verified in source]**

With the `claude` harness, Claude Code runs as the live process inside a **tmux
session** in the agent container. Users connect to that session:

| Surface | Experience | Nature |
|---|---|---|
| `fabric attach <agent>` | Drops into Claude Code's live TUI — real prompt, like your harness today | **Synchronous direct chat** ✅ closest to your model |
| Web UI terminal | WebSocket PTY into the same session — Claude TUI in the browser | Synchronous direct chat, in browser |
| `fabric message <agent> "..."` | Injected via tmux `send-keys`; `--raw` = literal keystrokes, `--interrupt` interrupts | Async / scripted |
| Chat bridges | User chats from Slack/etc.; routed to agent, replies back | Async conversational |
| `ask_user` signal | Claude signals it needs input → user notified → replies via message/attach → continues | Turn-based HITL |

The direct-chat experience your users expect = **`fabric attach` or the web terminal**.
Running inside Fabric does not strip Claude Code of its conversational UI.

### The tension — **[design/general]**
Your harness assumes a user *drives* Claude turn-by-turn; Fabric's model is
"give an agent a task, it runs, humans drop in when needed." Gaps:
1. `fabric attach` is a **terminal/PTY session**, not a branded chat widget.
2. Agents typically start *with* a task; a pure empty "waiting to chat" session is
   more the attach model (start the agent, then attach).
3. `fabric message` is **injection**, not a request/response chat protocol.

### Which path — **[design/general]**
- Users chat with *one* Claude → wrap harness as one Fabric agent; interact via
  `fabric attach` / web terminal (near drop-in).
- Users chat via your *own* front-end → build it on the **Hub REST API + WebSocket
  PTY** (what the web terminal uses) or route through a chat bridge; Fabric is the backend.
- Users pick which of N agents to talk to → Path B (each agent visible individually).

> **Caveat:** WebSocket PTY / Hub API endpoints are from the wiki snapshot; verify
> against `pkg/hub` + `pkg/brokerclient` before building a front-end on them.

---

## 7. Can multiple users share one live attached Claude Code session?

**Yes.** — **[verified in source: `pkg/runtimebroker/pty_handlers.go`]**

Every attach (CLI or web) runs `tmux attach-session -t fabric` inside the container
(`pty_handlers.go:50, :544, :929`). tmux natively multiplexes many clients into one
session. Fabric spins up an **independent PTY per WebSocket connection** with **no
single-client lock**, so:

- Two users open the web terminal for the same agent → both share the same live
  Claude session; both see output; both can type.
- Mix CLI `fabric attach` + multiple browser terminals freely.
- It's a genuine **shared collaborative terminal** (tmux shared-session model over WebSocket).

### Caveats (real) — **[verified in source]**
1. **Unarbitrated input** — all users type into the *same* prompt; simultaneous typing
   interleaves/garbles. No built-in floor control / "who's driving."
2. **Terminal-size contention** — clients with different sizes fight over sizing; depends
   on tmux `window-size`. Shipped `.tmux.conf` sets none (see Q8).
3. **No per-user access scoping in the PTY path** — auth is per-connection ("Auth is
   handled separately", `pty_handlers.go:212`), but once in, everyone gets equal
   read/write. No read-only/observer mode in this path (the `ObserverOnly` flag exists
   only on the *messaging* layer, not PTY).
4. **One Claude = one shared context** — users co-drive one conversation, not separate ones.

**Fits:** "multiple users collaborate with one shared Claude" ✅ out of the box.
**Does not fit:** per-user separate conversations (→ one agent per user), or
controlled driver/observer roles (→ you'd add it).

---

## 8. How do I set up "multiple users collaborate with one shared Claude"?

This is the **built-in default** — point N users at the same agent. — **[verified in source]**

### How users connect
- **Web terminal (easiest):** start Hub with `--enable-web`; users open the agent's
  detail page → terminal tab. Each browser = authenticated WebSocket → own PTY → same
  tmux session. The attach syncs a toolbar via an OSC escape (`activeWindowOSC`).
- **CLI:** `fabric attach <agent>` (routes via Hub `attachViaHub` when configured, else
  local). Mix freely with web users.

**Model:** one shared "collab" agent per session/room; share the URL, everyone joins.

### The one thing you MUST tune: tmux window sizing — **[verified in source]**

The shipped `.tmux.conf`
(`pkg/config/embeds/templates/default/home/.tmux.conf`) sets **no `window-size` /
`aggressive-resize`**. tmux's default `window-size` is `latest` → the whole session
resizes to whichever client resized/attached most recently → jumpy display, truncated
output for smaller clients. **This is the #1 thing that makes naive shared tmux feel broken.**

**Fix:** add to that file (edit the *source embed*, not `.fabric/`, per CLAUDE.md):
```tmux
set -g window-size smallest
set -g aggressive-resize on
```
Sizes the session to the smallest attached client → consistent, fully-visible view.

### Out of the box vs. add-later
- **Out of the box (after the tmux tweak):** shared live session, real-time shared
  output, any user can type — a genuine collaborative Claude console. ✅
- **Add if needed:**
  - Read-only observer mode → pass tmux `-r` for observer connections. The attach
    command is currently hardcoded (`pty_handlers.go:544/929`), so this is a small
    code change, not a config toggle.
  - Turn-taking / designated driver → a front-end layer over the WebSocket, or social convention.

### Recommended path
1. One shared agent = one collaboration room; users join via web terminal URL.
2. Set `window-size smallest` (+ `aggressive-resize on`) in the template's `.tmux.conf`.
3. Ship as-is if coordinated pairing is acceptable (everyone can type; teams self-coordinate).
4. Add read-only observer mode / driver floor-control later if needed (bounded additions).

This is squarely in Fabric's wheelhouse — the attach/web-terminal design was built for
HITL on a persistent tmux session; sharing across users is a natural extension.

---

## 9. Can I run Claude Code with `--dangerously-skip-permissions` and attach via the web terminal?

**Yes — it's already ON by default.** — **[verified in source]**

The Claude harness (`harnesses/claude/config.yaml:53`) launches:
```yaml
command:
  base: ["claude", "--no-chrome", "--dangerously-skip-permissions"]
```
Confirmed in `cmd/fabrictool/commands/init.go:1826` and asserted in
`init_test.go:665`. It's a deliberate, consistent pattern:
- **Codex** → `--dangerously-bypass-approvals-and-sandbox --sandbox danger-full-access` (`harnesses/codex/config.yaml:45`)
- **Antigravity** → `exec agy --dangerously-skip-permissions` (`provision.py:353`)

Autonomous, non-interactive operation in an isolated container is the point, so
permission prompts are bypassed by design. Start a Claude agent and attach via the web
terminal → you join the live tmux session where the flag is already in effect. Nothing
to toggle.

### Notes / caveats — **[verified in source / design]**
1. **Risk containment is isolation, not prompts:** own container, own git worktree at
   `/workspace`, plus the Claude harness's **iptables network egress control**. In a
   **shared multi-user session (Q7/Q8)**, *any* attached user typing inherits full
   skip-permissions power, and there's no per-user PTY scoping — for a collaborative
   console that's expected, but everyone in the room is effectively an operator.
2. **If you ever wanted it OFF:** override `command.base` in a custom harness-config
   (edit source `harnesses/claude/config.yaml`, not `.fabric/`). Note Claude Code only
   allows the flag under conditions (non-root, sandboxed); Fabric runs the agent as the
   `fabric` user (`user: fabric`), which is why it works here.

---

## 10. I have a `jwn-claude` wrapper script (symlinked as `claude`). Can I keep it in Fabric?

**Yes — it's easy.** — **[verified in source]**

### How Fabric launches Claude (relevant facts)
- Command base is a **bare name**: `["claude", "--no-chrome", "--dangerously-skip-permissions"]`
  (`harnesses/claude/config.yaml:53`), resolved via **PATH inside the container**.
- `claude` is installed via `npm install -g @anthropic-ai/claude-code` in
  `harnesses/claude/Dockerfile` (lands on the npm global bin in PATH).
- Launched inside the container's tmux session.

Because it's a PATH lookup of a bare name, your symlink-shadowing pattern works exactly
as it does on your machine.

### Two ways to integrate `jwn-claude`
- **Option A — shadow `claude` with your wrapper (recommended, closest to today):**
  Extend `harnesses/claude/Dockerfile` to copy in `jwn-claude` and put a `claude`
  symlink → `jwn-claude` **earlier in PATH** than the npm global bin. Wrapper does its
  env/telemetry/housekeeping, then `exec`s the real Claude. Fabric still calls bare
  `claude`, unaware. **No Fabric config changes.**
- **Option B — point the harness command at your wrapper:** set `command.base` to
  `["jwn-claude", ...]` in a custom harness-config and install `jwn-claude` on PATH.
  Edit source `harnesses/claude/config.yaml` (not `.fabric/`).

### ⚠️ Gotcha: don't break agent-process detection — **[verified in source]**
Fabric's in-container supervisor detects the agent process by **inspecting the tmux
command line and matching on `claude`** (`cmd/fabrictool/commands/init.go:1826` + tests).
The exit-detection hook in `.tmux.conf` keys off the `agent` window's command exiting
to tear down the session.

- **Option A is safer** — the process/command line still contains `claude`, so detection keeps working.
- With **Option B** (renamed launched command), verify agent-liveness/exit detection
  still fires. Safest: have your wrapper **`exec`** the real claude (final process still
  named `claude`) rather than run it as a child.

> Not yet fully traced: the exact match logic at `init.go:1826`. Confirm before relying
> on Option B with a renamed binary. Option A is the safe default today.

### You may be duplicating Fabric — **[design/general]**
Your wrapper "sets env vars + housekeeping + telemetry before invoking claude." Fabric
already has first-class hooks for all three, so parts may be redundant or conflict:
- **Env vars** → harness `env:` block in `config.yaml` + auth/credential injection (`GatherAuth`).
- **Pre-launch housekeeping** → `provision.py` **`pre-start` lifecycle hook** (runs in-container before Claude starts).
- **Telemetry** → Fabric's OTel/hook pipeline (`fabrictool hook`, telemetry handlers) normalizes Claude events. Overlap risks double-counting.

Keep the wrapper for what's genuinely yours; move static env → `config.yaml` `env:`,
housekeeping → `provision.py` pre-start, and check telemetry against Fabric's OTel.

### Recommendation
- **Fastest, safest:** Option A — Dockerfile installs `jwn-claude`, `claude` symlink
  shadows it earlier in PATH, wrapper `exec`s real claude. Detection intact, no config changes.
- **Then rationalize** env/housekeeping/telemetry against Fabric's built-in hooks.

---

## 11. What does each layer of the concentric harness stack (Fabric → Claude Code → PDLC) do?

> **Provenance for this answer:** Fabric = **[verified in Fabric source]**; PDLC =
> **[verified in `../pdlc-os` source: README.md, agents/, skills/, hooks/]**; Claude
> Code = **[general — well-known product behavior]**. Confirmed against the actual
> PDLC fork at `/Users/xe4a/Projects/pdlc-os`.

When you run PDLC under Fabric, you get **three concentric harnesses**. The mental model:
each layer wraps the one inside it and makes a *net addition* — it does not replace or
duplicate the inner layer's job. From outermost to innermost:

```
┌──────────────────────────────────────────────────────────────────────────┐
│  FABRIC  — fleet orchestration & isolation (the "meta-harness")            │
│  • container + git-worktree per agent    • lifecycle (start/stop/resume)   │
│  • Hub state + Runtime Brokers           • attach / web terminal / SSE UI  │
│  • inter-agent messaging, multi-user     • auth/secret injection, limits,  │
│  • templates + harness-config              telemetry (OTel), GitHub App    │
│                                                                            │
│   ┌──────────────────────────────────────────────────────────────────┐   │
│   │  CLAUDE CODE  — the agent runtime / REPL (PDLC is a plugin OF this)│   │
│   │  • model + token/context management   • tool execution loop        │   │
│   │  • skills / plugins / MCP clients      • permissions model         │   │
│   │  • interactive REPL / turn flow        • Stop/Pre/PostToolUse hooks │   │
│   │  • sub-agents / Agent Teams mode       • file & shell tooling      │   │
│   │                                                                    │   │
│   │    ┌────────────────────────────────────────────────────────────┐ │   │
│   │    │  PDLC  — Product Development Lifecycle (Claude Code plugin)  │ │   │
│   │    │  • phases: Init→Inception(Brainstorm)→Construction(Build)→  │ │   │
│   │    │    Operation(Ship); hotfix / night-shift / rollback         │ │   │
│   │    │  • 10 named specialist agents + Party Mode meetings         │ │   │
│   │    │  • Beads(+Dolt) task graph & file-based memory (docs/pdlc/) │ │   │
│   │    │  • Decision Registry, 5-layer security, hook guardrails     │ │   │
│   │    │            ← your methodology / "what & how to build"       │ │   │
│   │    └────────────────────────────────────────────────────────────┘ │   │
│   └──────────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────────┘
```

Read it as: **PDLC decides *what/how* a product gets built (and by which specialist
role); Claude Code is the engine that *thinks and acts*; Fabric runs *many such engines*
safely, in parallel, on real repos, with humans able to watch and join.**

> **Key correction from the PDLC source:** PDLC is **itself a Claude Code plugin**, not
> a separate process that merely "runs on" Claude Code. It installs slash commands,
> **hooks** (SessionStart, PreToolUse, PostToolUse, Stop, statusLine), and specialist
> **sub-agents** *into* Claude Code. So the boundary between the inner two layers is a
> plugin boundary, not a process boundary — which matters for the overlap analysis in Q12.

### Layer 1 (innermost) — PDLC: the product-development methodology

**[verified in `../pdlc-os`]**

PDLC is "a Claude Code plugin that guides small startup-style teams (2–5 engineers)
through the full arc of feature development — from raw idea to shipped, production
feature — using structured phases, a named specialist agent team, persistent memory,
and safety guardrails." Its unit of work is a **feature** moving through **phases**.
What it adds:

- **Phased lifecycle** (`skills/`): **Init** (`/setup`) → **Inception** (`/brainstorm`:
  Discover→Define→Design→Plan) → **Construction** (`/build`: TDD + multi-agent review +
  test) → **Operation** (`/ship`: merge, deploy, reflect). Plus `/hotfix` (compressed
  emergency build-ship with auto-pause/resume), `/night-shift` (autonomous Build→Ship),
  `/rollback`, `/abandon`, `/pause`+`/continue`, `/release`, `/decide`, `/whatif`,
  `/diagnose`, `/override`.
- **A named 10-agent specialist team** (`agents/`, model tier per role):
  **Atlas** (PM, opus), **Neo** (Architect, opus), **Bolt** (Backend, opus),
  **Friday** (Frontend, opus), **Pulse** (DevOps, opus), **Echo** (QA, sonnet),
  **Phantom** (Security, sonnet, `always_on`), **Muse** (UX, sonnet),
  **Jarvis** (Tech Writer, sonnet), **Sentinel** (Night-shift watcher, haiku — the
  `type:"agent"` Stop hook that drives autonomous runs). Custom agents drop into
  `.pdlc/agents/` and auto-join meetings when labels match.
- **Party Mode** — 12 structured multi-agent meeting types across phases (Decision
  Review, What-If, Threat Modeling, Deployment Review, Post-Mortem, Contract, …) that
  produce **minutes (MOM files)**; under `/night-shift` they auto-resolve via binding
  pitch+vote.
- **Persistent memory & task graph** — file-based memory bank in `docs/pdlc/`
  (CONSTITUTION, INTENT, ROADMAP, DECISIONS, STATE, OVERVIEW, METRICS, episodes, PRDs,
  design docs, MOMs) committed to git as the team's shared brain, backed by the
  **Beads** task manager on a **Dolt** SQL DB. This is *product/feature* state —
  orthogonal to Claude Code's conversation state and Fabric's agent/container state.
- **Governance & safety** — a **Decision Registry** (`DECISIONS.md`, every ADR: who/
  when/why/impact), a **5-layer security model** (config → lifecycle stops like the
  Threat-Modeling Party → always-on Phantom → the **`hooks/pdlc-guardrails.js`** deploy
  gate → lifecycle-of-findings), and `/override` (single/double-confirm bypass, logged).
- **Context-rot prevention** — context-usage estimation with warn/auto-checkpoint
  thresholds, on-demand markdown skills, per-task model tiering (Haiku/Sonnet/Opus),
  `distill`/`condense` compression of large artifacts.

**Net add:** *methodology, a specialist team, durable product memory, and process/
security discipline.* PDLC is the difference between "an agent that can edit code" and
"a whole product-dev team + method that records decisions, enforces safety gates, and
manages feature lifecycle." Its value is the methodology + memory, portable in principle
to any runtime that supports the plugin primitives it relies on.

### Layer 2 (middle) — Claude Code: the agent runtime / plugin host

**[general — well-known Claude Code behavior; plugin/hook/Agent-Teams facts corroborated by PDLC's install]**

Claude Code is the **execution engine and plugin host** PDLC installs *into*. It
provides exactly the primitives PDLC depends on:

- **Model & context/token management** — LLM calls, context window, prompt caching,
  compaction. (PDLC's context-monitor hook rides on top of this.)
- **The REPL / turn flow** — reason → tool call → result → response; interrupts.
- **Plugin system: skills, slash commands, hooks, sub-agents** — PDLC's commands, its
  `SessionStart`/`PreToolUse`/`PostToolUse`/`Stop`/`statusLine` hooks, and its 10
  agents are all registered *through* Claude Code's plugin + **Agent Teams** mechanism
  (`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`, set by `pdlc install`; Sentinel is a
  `type:"agent"` Stop hook that only spawns under Agent Teams).
- **MCP clients**, **tool execution** (file/shell/search), and the **permissions model**
  (`--permission-mode bypassPermissions` / `--dangerously-skip-permissions`).
- **Session state** — the conversation the agent reasons within.

**Net add:** *a capable, domain-agnostic coding-agent runtime + plugin host.* It turns
PDLC's methodology into real reasoning and actions. It doesn't know about "phases,"
"Party Mode," or "Beads" — PDLC supplies that meaning; Claude Code supplies the engine
and the extension points.

> **Nordstrom note:** `superclaude` (and the `jwn-claude` binary this fork wires in) is
> a thin wrapper = `claude --permission-mode bypassPermissions "$@"`. PDLC's
> `/night-shift` *requires* bypass mode. This is the same "skip permissions" posture
> Fabric's `claude` harness already launches with (Q9) — see the Q12 overlap note.

### Layer 3 (outermost) — Fabric: fleet orchestration & isolation (the meta-harness)

**[verified in Fabric source]**

Fabric wraps a *whole Claude-Code-running-PDLC process* as **one Fabric agent = one
container** (`harnesses/claude`, `command.base` launches `claude`). It adds everything
*around* a single agent that a single agent cannot give itself:

- **Isolation & context management** — each agent runs in its own container with a
  dedicated **git worktree** at `/workspace` on its own branch, so many
  PDLC-on-Claude-Code agents work the same repo concurrently without collision.
- **Fleet lifecycle** — `create`/`start`/`stop`/`resume`/`delete`, resource limits
  (`FABRIC_MAX_TURNS`/`_DURATION`/`_MODEL_CALLS`), auto-suspend/resume, crash handling
  — supervised by `fabrictool` (PID 1) in the container.
- **Distribution** — the **Hub** (central state: agents, projects, templates, secrets,
  policies) + **Runtime Brokers** (execute agents on local Docker/Podman/Apple/K8s),
  so the fleet spans machines/clusters.
- **Human-in-the-loop & multi-user** — `fabric attach` and the **web terminal** join the
  agent's live tmux session; **multiple users can share one live Claude Code/PDLC
  session** (Q7/Q8).
- **Inter-agent coordination** — `fabric message` + orchestrator patterns (fan-out /
  sequential / coordinator-relay); a lead agent can spawn/manage sub-agents in its project.
- **Cross-cutting platform services** — auth/secret injection (`GatherAuth`, GCE
  metadata emulation), MCP server config translation, OTel telemetry normalization,
  GitHub App token management (`gh` wrapper / credential helper), image build pipeline.
- **Observability & UI** — Hub web UI: agent list, real-time Status (Phase/Activity/…)
  via SSE, logs/messages, the in-browser terminal.

**Net add:** *scale, isolation, distribution, multi-user, and platform plumbing.*
Fabric is **methodology- and runtime-agnostic** — it doesn't care that the container is
running Claude Code, or that Claude Code has the PDLC plugin loaded. To Fabric it's "an
agent"; that opacity is exactly what lets it orchestrate *any* harness the same way.

### The clean separation of concerns (why "concentric" is the right word)

| Layer | Owns | Unit of state | Agnostic to |
|---|---|---|---|
| **Fabric** | Orchestration, isolation, distribution, multi-user, platform services | **Agent / container** state | *what* runtime or plugin runs inside |
| **Claude Code** | Model/tokens, REPL, tools, plugin+hook host, permissions, Agent Teams | **Conversation / session** state | *what domain* it's being used for |
| **PDLC** | Product-dev method, 10-agent team, Party Mode, Beads/Dolt memory, decisions, security | **Feature / product** state (in `docs/pdlc/` + Beads) | *which* engine executes it |

Each layer manages a **different kind of state** and is **agnostic to the layers on
either side of it**. That orthogonality is the whole point — see Q12 for why keeping
them separate is worth it (and the seams that need care because PDLC is a *plugin of*
Claude Code, not a separate process).

---

## 12. Why keep the layers separate? Benefits and overlaps to manage

> **[design/general reasoning; overlap mechanics verified in both Fabric and `../pdlc-os` source]**

### Why the concentric arrangement is worth keeping

1. **Separation of concerns / single responsibility.** Each layer owns one job and one
   kind of state (feature/memory vs. conversation vs. agent-container). You can reason
   about, test, and change one without destabilizing the others.
2. **Independent evolvability & substitutability.**
   - PDLC's methodology, agents, and Party Mode evolve via its own repo (`pdlc upgrade`)
     without touching Fabric or Claude Code. Its agent `model:` tiers (`opus`/`sonnet`/
     `haiku`) auto-track new Anthropic models without a PDLC release.
   - Claude Code can be upgraded (new model, new tools) under a stable PDLC.
   - Because Fabric is **harness-agnostic**, you could run PDLC on a different host, run
     Claude Code *without* PDLC, or run *other* harnesses beside PDLC in the same Fabric
     fleet. No layer is welded to another.
3. **You reuse, not rebuild.** Fabric gives isolation/lifecycle/multi-user/distribution
   for free; Claude Code gives the REPL/token/plugin/hook machinery for free; PDLC keeps
   being your differentiated IP (method + 10-agent team + memory bank). None of the
   three is worth reimplementing inside another.
4. **Net-additive capability, not duplication.** PDLC (method + team + memory) → +
   Claude Code (execution + plugin host) → + Fabric (fleet/isolation/multi-user). The
   outcome — *many isolated, observable, human-joinable agents, each running your full
   PDLC team-and-method on its own git branch* — is something **no single layer can
   produce alone**.
5. **Matches your goals from earlier questions.** Multi-user shared sessions (Q7/Q8),
   web UI + attach (Q5/Q6), skip-permissions autonomy (Q9), and keeping your wrapper
   (Q10) are all **Fabric-layer** concerns that drop in *without* changing PDLC or Claude
   Code — precisely because the layers are separate.
6. **Right layer for human-in-the-loop.** Humans join at the **Fabric** layer (attach /
   web terminal) into the **Claude Code** REPL, while **PDLC** governs what the work
   *is* (phases, Party Mode approvals) — oversight, execution, and methodology each have
   a natural home. Note PDLC's own approval gates (Contract Party, `/decide`, `/override`)
   assume an interactive human; see the multi-user seam below.

### Overlaps to manage (where layers can step on each other)

Concentric layering only stays clean if you avoid double-owning the same responsibility.
Because **PDLC is a plugin *inside* Claude Code** (not a separate process), the inner
two layers share an address space — so these seams need explicit ownership:

- **Permissions/autonomy (Claude Code ↔ PDLC gates) — REAL, verified both sides.**
  Fabric launches Claude Code with `--dangerously-skip-permissions`
  (`harnesses/claude/config.yaml:53`); PDLC's `/night-shift` *also requires* bypass mode
  (`superclaude`/`jwn-claude`). So Claude Code's per-tool permission prompts are OFF by
  design — which means **PDLC's own guardrails become the real safety layer**:
  `hooks/pdlc-guardrails.js` (deploy gate outside Operation phase), the 5-layer security
  model, Phantom `always_on`, `/override`. **Under `/night-shift` PDLC deliberately
  bypasses its own deploy gate** (`[night-shift bypass]` log) and relies on driver-agent
  discipline + contract abort_conditions + the three-layer production-deploy ban.
  Containment therefore = **Fabric's container/worktree/egress isolation + PDLC's
  guardrails**, not Claude Code prompts. Make sure that's the intended posture.
- **Hooks & sub-agents (PDLC ↔ Claude Code ↔ Fabric supervisor).** PDLC registers
  `SessionStart`/`PreToolUse`/`PostToolUse`/`Stop`/`statusLine` hooks and requires
  **Agent Teams** (`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`). In a Fabric container these
  must be present and correctly registered at provision time (the `claude` harness image
  must have PDLC installed + hooks wired + Agent Teams enabled). Verify this is done in
  the harness `Dockerfile`/`provision.py`, not assumed from a developer's laptop.
- **Env / housekeeping / telemetry (jwn-claude wrapper ↔ Fabric ↔ PDLC).** As in Q10,
  Fabric provides env injection (`config.yaml env:` + `GatherAuth`), a `provision.py`
  pre-start hook, and an OTel pipeline; PDLC has its own context-monitor + statusline +
  metrics. Decide one owner per concern (env, telemetry) to avoid drift/double-counting.
- **Three different "resume/continue" semantics — must not be conflated:** PDLC
  `/pause`+`/continue` (feature state in `docs/pdlc/` + Beads reclaim + rebase-on-main),
  Claude Code session/context, Fabric agent suspend/resume + limits. Keep them distinct
  in docs and UX; a Fabric resume is not a PDLC `/continue`.
- **Persistent state & multi-user (Beads/Dolt ↔ Fabric worktrees ↔ shared sessions).**
  PDLC's memory + Beads/Dolt task graph live **in the repo** (`docs/pdlc/`, `.beads/`).
  With **Fabric giving each agent its own git worktree/branch**, PDLC's multi-developer
  model (shared `docs/pdlc/` via git, per-dev local hooks, roadmap-claim reconciliation
  through Beads) maps onto *per-agent worktrees* — confirm how Beads/Dolt state is shared
  or isolated across concurrent Fabric agents so two agents don't fork the task graph.
  Separately, in a **shared multi-user Fabric session (Q7/Q8)** many humans drive one
  Claude Code REPL — but PDLC's approval gates assume *one* interactive human; decide who
  "the human" is for Party Mode approvals / `/override` in a shared session.
- **Orchestration altitude (Fabric fleet ↔ PDLC autonomous flows).** PDLC `/night-shift`
  (Sentinel Stop-hook loop) orchestrates a *feature* Build→Ship *inside one agent*;
  Fabric orchestrates *many agents*. Both are legitimate but different altitudes — be
  deliberate: one Fabric agent running PDLC night-shift, vs. a Fabric orchestrator fanning
  out several PDLC agents. Also note **PDLC never deploys to production** (three-layer
  ban) — a Fabric-level deploy path must respect that.

### Bottom line

Keep the layers concentric and orthogonal. **PDLC = the method + team + memory (what/how
to build, by which role, with what recorded), Claude Code = the engine + plugin host
(reason + act + tokens/REPL/hooks/Agent-Teams), Fabric = the platform (isolate, scale,
distribute, let humans watch/join).** Each is net-additive and runtime/domain-agnostic
to its neighbors — which is what makes the stack upgradable, testable, and reusable. The
one discipline required: **assign each cross-cutting concern (permissions, hooks/Agent-
Teams, env, telemetry, resume-semantics, Beads/Dolt state, deploy authority) to a single
owning layer** so the seams stay clean — paying special attention to the fact that PDLC
lives *inside* Claude Code and its guardrails (not Claude Code prompts) are the real
safety net once Fabric runs it in skip-permissions mode.

---

*Generated from the Fabric working session on 2026-07-08. For deep architecture, see
[`README.md`](./README.md) (the full Code Wiki) and the diagrams in [`diagrams/`](./diagrams/).*
