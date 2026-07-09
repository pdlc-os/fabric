# Design: FabricTool Architecture

## 1. Executive Summary

`fabrictool` is a unified Go binary designed to run *inside* Fabric agent containers. It serves as the container's specialized init process (PID 1), lifecycle manager, and telemetry forwarder.

Currently, agent containers rely on a mix of shell scripts, Python hooks, and direct entrypoints. `fabrictool` consolidates these responsibilities into a single, robust, and testable binary that ensures consistent behavior across different runtimes (Docker, Kubernetes, Apple).

**Key Benefits:**
- Single source of truth for container initialization logic
- Proper zombie process reaping (critical for PID 1)
- Standardized lifecycle hooks across agent types (Gemini, Claude)
- Built-in observability via OTel forwarding
- Testable Go code replacing fragile shell scripts

## 2. Architecture & Components

`fabrictool` is built as a modular CLI application using Cobra, with each major capability isolated in its own package.

```mermaid
graph TD
    Entry[Container Entrypoint] --> FabricTool[fabrictool init]

    subgraph "FabricTool (PID 1)"
        Reaper[Process Reaper]
        Setup[Setup Logic]
        Supervisor[Process Supervisor]

        Reaper --> Supervisor
        Setup --> Supervisor
    end

    Supervisor --> Agent[Agent Process (Gemini/Claude)]
    Supervisor --> OTel[OTel Collector]
    Supervisor --> TTY[WebTTY Server]

    Agent -->|Hooks| FabricToolHooks[fabrictool hook]
    Agent -->|Logs/Traces| OTel

    OTel -->|Forward| External[Hub / Remote OTel]
```

### 2.1. Project Structure

```
cmd/fabrictool/
├── main.go           # Entry point
├── root.go           # Root Cobra command
├── version.go        # Version command
├── init.go           # Init/PID 1 command
├── hook.go           # Hook command (harness hooks entry point)
├── daemon.go         # Hub daemon command (hosted mode)
└── otel.go           # OTel subcommands (future)

pkg/fabrictool/
├── supervisor/       # Process management & signal handling
├── hooks/            # Hook system
│   ├── lifecycle.go  # Fabric lifecycle hooks (pre-start, post-start, session-end)
│   ├── harness.go    # Harness hook dispatcher
│   ├── handlers/     # Shared hook handler implementations
│   └── dialects/     # Harness-specific event parsers
│       ├── claude.go # Claude Code event format
│       └── gemini.go # Gemini CLI event format
├── hub/              # Hub communication (hosted mode)
│   ├── client.go     # Hub API client
│   ├── heartbeat.go  # Liveness reporting loop
│   └── status.go     # Agent status management
├── telemetry/        # OTel collector/forwarder
└── setup/            # Container setup tasks
```

### 2.2. Core Capabilities

#### A. Container Entrypoint (PID 1)
- **Library:** `github.com/ramr/go-reaper`
- **Role:** Acts as the init process. Handles signal propagation (SIGTERM/SIGINT) to child processes and reaps zombie processes to prevent resource leaks.
- **Command:** `fabrictool init [--] <command> [args...]`
- **Behavior:**
  1. Initialize the reaper goroutine
  2. Run setup tasks (permissions, mounts)
  3. Spawn the child command (e.g., `gemini`, `tmux`)
  4. Forward signals to child processes
  5. Wait for child exit and propagate exit code

#### B. Setup & Provisioning
- **Role:** Executed before the main agent process starts.
- **Tasks:**
  - Mount FUSE filesystems (gcsfuse for GCS access)
  - Fix permissions on `/workspace` or home directories
  - Inject dynamic configuration files
  - Validate environment requirements

#### C. Hook System

The hook system has two distinct layers:

**C.1. Fabric Lifecycle Hooks**
- **Role:** Container-level lifecycle events managed directly by `fabrictool init`.
- **Trigger:** Invoked automatically by the supervisor during agent lifecycle transitions.
- **Events:**
  - `pre-start` — Before agent process begins (after setup, before spawn)
  - `post-start` — After agent process is confirmed running
  - `session-end` — On graceful shutdown (before child termination)
- **Use Cases:** Workspace validation, environment checks, cleanup tasks, metrics flush.

**C.2. Harness Hooks**
- **Role:** Replaces existing `fabric_hook.py` script. Receives events from agent harnesses (Claude Code, Gemini CLI) during their operation.
- **Command:** `fabrictool hook <event> [--dialect=claude|gemini] [--data=<json>]`
- **Trigger:** Called directly by the harness configuration (e.g., Claude Code hooks, Gemini CLI callbacks).
- **Events:**
  - `tool-start` — Before a tool/command execution
  - `tool-end` — After a tool/command execution (includes result status)
  - `prompt-submit` — User prompt submitted to agent
  - `response-complete` — Agent response finished
- **Dialects:** Each harness sends events in its native format. The `--dialect` flag tells fabrictool how to parse and normalize the incoming data into Fabric-standard events.

```
┌─────────────────────────────────────────────────────────────┐
│                     fabrictool init (PID 1)                  │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ Fabric Lifecycle Hooks                               │    │
│  │   pre-start → post-start → ... → session-end       │    │
│  └─────────────────────────────────────────────────────┘    │
│                           │                                 │
│                           ▼                                 │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ Agent Process (Claude Code / Gemini CLI)            │    │
│  │                                                     │    │
│  │   On tool use: exec `fabrictool hook tool-start`    │    │
│  │   On complete: exec `fabrictool hook tool-end`      │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

#### D. Telemetry & Observability
- **Role:** In-container OTel collector and forwarder.
- **Endpoint:** `localhost:4317` (OTLP gRPC)
- **Function:**
  - Receives OTLP data from the agent process
  - Filters and sanitizes data (removing PII if configured)
  - Batches and forwards metrics/traces to configured backend
- **Backends:** Fabric Hub, Google Cloud Trace, or external OTel collector

#### E. Connectivity & Access
- **WebTTY:** Browser-based terminal access via `tsl0922/ttyd` (orchestrated subprocess)
- **Reverse Tunnel:** Secure tunnels for remote management in hosted scenarios

#### F. Hub Daemon (Hosted Mode)
- **Role:** Background daemon process for hub communication in hosted mode.
- **Trigger:** When `FABRIC_AGENT_MODE=hosted` and `FABRIC_HUB_ENDPOINT` is set, `fabrictool init` spawns itself as a daemon subprocess (`fabrictool daemon`).
- **Authentication:** Authenticates with the Hub using a short-lived, Hub-issued JWT provided via the `FABRIC_HUB_TOKEN` environment variable. See [Agent Authentication](hosted/auth/fabrictool-auth.md) for details.
- **Capabilities:**
  - **Heartbeat:** Periodic liveness reporting to the hub (configurable interval, default 30s)
  - **Status Sync:** Reports agent state (idle, busy, error) to the hub
  - **Command Receiver:** Listens for control commands from hub (future: pause, resume, terminate)
- **Command:** `fabrictool daemon --hub=<endpoint> --agent-id=<id>`
- **Lifecycle:** Spawned after the agent process starts; terminated during session-end hook.

```
┌─────────────────────────────────────────────────────────────┐
│                   fabrictool init (PID 1)                    │
│                                                             │
│   if FABRIC_AGENT_MODE=hosted && FABRIC_HUB_ENDPOINT set:    │
│                                                             │
│   ┌──────────────────┐      ┌──────────────────────────┐   │
│   │ Agent Process    │      │ fabrictool daemon         │   │
│   │ (Claude/Gemini)  │      │   • heartbeat → hub      │   │
│   │                  │      │   • status sync          │   │
│   └──────────────────┘      └──────────────────────────┘   │
│                                       │                     │
│                                       ▼                     │
│                              ┌─────────────────┐            │
│                              │   Fabric Hub     │            │
│                              └─────────────────┘            │
└─────────────────────────────────────────────────────────────┘
```

## 3. Technical Considerations

### 3.1. Build Infrastructure
- **Build Context:** The `cloudbuild.yaml` must use the repo root (`.`) as the build context to access `cmd/fabrictool`. The Dockerfile path will be specified explicitly.
- **CGO Requirement:** The `go-reaper` library requires CGO (for `prctl` syscall). The `fabric-base` builder image includes `gcc`, so this is supported.
- **Binary Size:** Use `go build -ldflags="-s -w"` to strip debug symbols (~30% size reduction).

### 3.2. Entrypoint & Command Flexibility

**Container Image Defaults:**
- **ENTRYPOINT:** `["fabrictool", "init", "--"]` (fixed, ensures fabrictool is always PID 1)
- **CMD:** `["gemini"]` (default command, can be overridden)

**Command Override Flow:**
The `fabric-agent.json` config or agent template may specify a custom command and arguments. These are passed to the container at runtime and forwarded by fabrictool to the child process.

```
Dockerfile:
  ENTRYPOINT ["fabrictool", "init", "--"]
  CMD ["gemini"]

Runtime override (from fabric-agent.json):
  docker run <image> tmux new-session -A -s main

Effective execution:
  fabrictool init -- tmux new-session -A -s main
                    └─────────────────────────┘
                      Passed as child command
```

**Supported Patterns:**
| Source | Command | Result |
|--------|---------|--------|
| Default (no override) | — | `fabrictool init -- gemini` |
| Agent config command | `["claude"]` | `fabrictool init -- claude` |
| Agent config with args | `["tmux", "new-session", "-A"]` | `fabrictool init -- tmux new-session -A` |
| Template with env expansion | `["${AGENT_CMD}"]` | Resolved at container start |

**Implementation Notes:**
- The `--` separator is critical: everything after it is treated as the child command
- `fabrictool init` must handle the case where no command is provided (use a sensible default or error)
- Environment variable expansion in command args is handled by the container runtime, not fabrictool

### 3.3. Signal Handling
When managing `tmux` or similar session managers:
- SIGTERM → Graceful shutdown (send to child, wait with timeout)
- SIGINT → Immediate forward to child
- SIGHUP → Reload configuration (future)
- Use a configurable grace period (default: 10s) before SIGKILL

## 4. Implementation Phases

### Phase 1: Build Infrastructure & Skeleton
**Goal:** Establish the binary structure and integrate with the build pipeline.

| Action | Files | Details |
|--------|-------|---------|
| Create CLI skeleton | `cmd/fabrictool/main.go`, `root.go`, `version.go` | Initialize Cobra CLI with `version` command |
| Update build context | `image-build/cloudbuild.yaml` | Change `dir: .` and specify Dockerfile path |
| Update Dockerfile | `image-build/fabric-base/Dockerfile` | Add multi-stage Go build for fabrictool |
| Update .dockerignore | `.dockerignore` | Ensure `.git` and large dirs are excluded |

### Phase 2: Init Process (PID 1) & Supervisor
**Goal:** Take over the container entrypoint with proper process management.

| Action | Files | Details |
|--------|-------|---------|
| Implement `init` command | `cmd/fabrictool/init.go` | Integrate `go-reaper`, spawn child, handle signals |
| Add supervisor package | `pkg/fabrictool/supervisor/` | Process lifecycle management |
| Update entrypoint | `image-build/fabric-base/Dockerfile` | Set `ENTRYPOINT ["fabrictool", "init", "--"]` |

### Phase 3: Hook System
**Goal:** Implement both Fabric lifecycle hooks and harness hook processing.

| Action | Files | Details |
|--------|-------|---------|
| Add lifecycle hooks to init | `cmd/fabrictool/init.go` | Call pre-start, post-start, session-end at appropriate points |
| Implement `hook` command | `cmd/fabrictool/hook.go` | CLI entry point for harness hooks with dialect parsing |
| Add hook handlers | `pkg/fabrictool/hooks/` | Shared handlers for both hook types |
| Add dialect parsers | `pkg/fabrictool/hooks/dialects/` | Claude and Gemini event format parsers |
| Replace fabric_hook.py | Agent config files | Update harness configs to call `fabrictool hook` instead of Python

### Phase 4: Telemetry (OTel)
**Goal:** Enable visibility into agent operations.

| Action | Files | Details |
|--------|-------|---------|
| Add OTel receiver | `pkg/fabrictool/telemetry/receiver.go` | OTLP receiver on localhost:4317 |
| Add forwarder | `pkg/fabrictool/telemetry/forwarder.go` | Batch and forward to backend |
| Integrate with init | `cmd/fabrictool/init.go` | Start OTel as managed subprocess |

### Phase 5: Hub Daemon (Hosted Mode)
**Goal:** Enable hub communication for hosted deployments.

| Action | Files | Details |
|--------|-------|---------|
| Implement `daemon` command | `cmd/fabrictool/daemon.go` | Long-running hub communication process |
| Add hub client | `pkg/fabrictool/hub/client.go` | HTTP/gRPC client for hub API |
| Implement heartbeat | `pkg/fabrictool/hub/heartbeat.go` | Periodic liveness reporting |
| Add status management | `pkg/fabrictool/hub/status.go` | Track and report agent state |
| Integrate with init | `cmd/fabrictool/init.go` | Spawn daemon when in hosted mode |

### Phase 6: Advanced Connectivity
**Goal:** Enable remote interaction patterns.

| Action | Files | Details |
|--------|-------|---------|
| WebTTY integration | `pkg/fabrictool/tty/` | Manage ttyd subprocess |
| Reverse tunnel | `pkg/fabrictool/tunnel/` | Secure tunnel to Fabric Hub |

## 5. Configuration

`fabrictool` is configured via environment variables and an optional JSON config file.

### Environment Variables
| Variable | Description | Default |
|----------|-------------|---------|
| `FABRIC_AGENT_MODE` | Operating mode: `solo` or `hosted` | `solo` |
| `FABRIC_HUB_ENDPOINT` | URL for the centralized hub | — |
| `FABRIC_AGENT_ID` | Unique agent identifier (required for hosted mode) | — |
| `FABRIC_HEARTBEAT_INTERVAL` | Hub heartbeat interval (hosted mode) | `30s` |
| `FABRIC_LOG_LEVEL` | Logging verbosity: `debug`, `info`, `warn`, `error` | `info` |
| `FABRIC_OTEL_ENDPOINT` | OTel backend endpoint | — |
| `FABRIC_GRACE_PERIOD` | Shutdown grace period | `10s` |

### Config File (Optional)
Location: `/etc/fabric/config.json` or `$FABRIC_CONFIG_PATH`

```json
{
  "agent_mode": "hosted",
  "agent_id": "agent-abc123",
  "hub": {
    "endpoint": "https://hub.example.com",
    "heartbeat_interval": "30s"
  },
  "otel": {
    "endpoint": "otel-collector:4317",
    "insecure": false
  },
  "hooks": {
    "pre_start": ["validate-workspace"],
    "post_command": ["log-metrics"]
  }
}
```

## 6. Verification Strategy

### Build Verification
```bash
# From repo root
docker build -f image-build/fabric-base/Dockerfile .
```

### PID 1 Verification
```bash
# Start container
docker run -it --rm fabric-base:test

# Inside container
ps aux | head -5   # Verify fabrictool is PID 1
```

### Signal Handling Test
```bash
# Terminal 1: Start container
docker run --name test-container fabric-base:test

# Terminal 2: Send SIGTERM
docker stop test-container

# Verify graceful shutdown in logs
docker logs test-container | grep -i shutdown
```

### Zombie Reaping Test
```bash
# Inside container, create a zombie
( sleep 1 & ) && sleep 2 && ps aux | grep defunct
# Should show no defunct/zombie processes
```

## 7. Risks & Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Container bloat** | Increased image size | Multi-stage builds, strip debug symbols (`-ldflags="-s -w"`) |
| **CGO dependency** | Build complexity, cross-compilation issues | Builder image includes gcc; document build requirements |
| **Signal handling with tmux** | Incorrect shutdown behavior | Test signal propagation thoroughly; configurable grace period |
| **Build context size** | Slow builds if large dirs sent to daemon | Robust `.dockerignore` excluding `.git`, `node_modules`, etc. |
| **Subprocess complexity** | Race conditions, resource leaks | Use established patterns (suture library if needed); comprehensive tests |

## 8. Future Considerations

- **Health endpoints:** HTTP health/ready endpoints for Kubernetes probes
- **Metrics exposition:** Prometheus-format metrics at `/metrics`
- **Hot reload:** Reload configuration without restart
- **Plugin system:** Dynamic hook loading for custom integrations

---

## Appendix A: Shutdown Hook Performance Analysis

*Added: 2026-01-30*

### Observed Behavior

Container shutdown logs show delays around these messages:
```
[fabrictool] Running pre-stop hooks...
[fabrictool] Running session-end hooks...
```

### What the Hooks Actually Do

**Pre-stop hooks** (`pkg/fabrictool/hooks/lifecycle.go:62-70`):
1. Looks for executable scripts in `/etc/fabric/hooks/pre-stop{,.sh,.d/*}` (none installed by default)
2. Calls `StatusHandler.Handle()` — writes "shutting_down" to `~/agent-info.json`
3. Calls `LoggingHandler.Handle()` — appends to `~/agent.log`

**Session-end hooks** (`lifecycle.go:72-79`):
- Same pattern: script hooks + status update + log append

All registered handlers perform simple local file operations (no network calls).

### Shutdown Sequence

```
SIGTERM/SIGINT received
    │
    ▼
[fabrictool] Running pre-stop hooks...
    │  └─ StatusHandler: write agent-info.json
    │  └─ LoggingHandler: append agent.log
    ▼
context.Cancel() called
    │
    ▼
Supervisor sends SIGTERM to child process
    │
    ▼
Child (Claude Code/Gemini) receives SIGTERM
    │  └─ Harness fires SessionEnd/Stop hooks
    │  └─ Each hook spawns: fabrictool hook --dialect=claude
    │
    ▼
Child process exits
    │
    ▼
[fabrictool] Running session-end hooks...
    │  └─ StatusHandler: write agent-info.json
    │  └─ LoggingHandler: append agent.log
    ▼
fabrictool exits with child's exit code
```

### Likely Sources of Delay

1. **Child process shutdown time** — The gap between pre-stop and session-end includes waiting for the child process (Claude Code/Gemini) to exit. The harness may be doing its own cleanup.

2. **Harness hook spawning** — When the child receives SIGTERM, Claude Code fires `SessionEnd` and `Stop` hooks (configured in `pkg/config/embeds/claude/settings.json:21-24`), each spawning a new `fabrictool hook` process.

3. **Process spawn overhead** — Each `fabrictool hook` invocation starts a new Go process. Multiple hooks during shutdown compound this latency.

4. **Filesystem latency** — On network storage or slow overlay filesystems, the atomic file writes could be slower than expected.

### Potential Optimizations

- **Add timing instrumentation** to identify the exact bottleneck
- **Batch harness hooks** to reduce process spawning during shutdown
- **Async file writes** for non-critical status updates
- **Reduce harness hooks on shutdown** — consider removing `Stop`/`SubagentStop` hooks if `SessionEnd` is sufficient
