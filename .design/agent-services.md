# Agent Services: Template-Defined Sidecar Processes

## Status: Implemented

**Implementation:** `pkg/fabrictool/services/manager.go` (ServiceManager with 14 methods)

The design below has been fully implemented, including:
- Service spec parsing from `fabric-agent.yaml` (`pkg/api/types.go` ServiceSpec)
- Process lifecycle management (start, stop, restart on failure)
- Consecutive failure tracking (max 3 failures before abandonment)
- Log file management (stdout, stderr, lifecycle logs)
- Ready-check support between services
- Graceful shutdown with configurable grace period
- UID/GID synchronization for container user mapping

---

## Overview

Templates should be able to specify additional processes ("services") that start automatically alongside the main harness process inside the container. The motivating use case is MCP servers (e.g., Chrome DevTools MCP) and supporting daemons (e.g., Xvfb for headless browser rendering), but the mechanism should be general enough to support any background process an agent template needs.

Today, `fabrictool init` supervises exactly one child process (the harness command). This design extends that to support a set of named services defined in `fabric-agent.yaml`, managed by fabrictool with restart and lifecycle semantics.

## Current Architecture

### Container startup sequence

```
Container entrypoint
  └─ fabrictool init -- <harness-command>
       ├─ StartReaper()           (zombie cleanup, critical for PID 1)
       ├─ setupHostUser()         (UID/GID sync)
       ├─ Initialize telemetry
       ├─ Initialize lifecycle hooks
       ├─ RunPreStart hooks
       ├─ Create supervisor       (manages ONE child process)
       ├─ Start signal handler
       ├─ Spawn child process     (the harness: claude, gemini, etc.)
       ├─ RunPostStart hooks
       ├─ Hub reporting + heartbeat
       ├─ Wait for child exit
       └─ RunSessionEnd hooks
```

### Relevant types

- `api.FabricConfig` (`pkg/api/types.go:105`) — The struct backing `fabric-agent.yaml`. Currently has `harness`, `env`, `volumes`, `command_args`, `model`, `image`, etc.
- `supervisor.Supervisor` (`pkg/fabrictool/supervisor/`) — Manages the child process lifecycle with signal forwarding, grace period shutdown, and UID/GID dropping.
- Lifecycle hooks (`pkg/fabrictool/hooks/`) — Pre-start, post-start, pre-stop, session-end events with registered handlers.

### Current fabric-agent.yaml (minimal)

```yaml
harness: claude
```

## Schema

Add a `services` field to `fabric-agent.yaml`:

```yaml
harness: claude

services:
  - name: chrome-mcp
    command: ["npx", "@anthropic-ai/chrome-devtools-mcp@latest"]
    restart: on-failure
    env:
      CHROME_PATH: "/usr/bin/chromium"

  - name: xvfb
    command: ["Xvfb", ":99", "-screen", "0", "1280x1024x24"]
    restart: always
    env:
      DISPLAY: ":99"
```

Services are defined in `fabric-agent.yaml` only — they travel with the template and are provisioned per-agent. No per-agent service overrides are supported; modifying services for a specific agent instance is handled through post-create agent editing.

### ServiceSpec fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | yes | — | Unique identifier, used for logging and status |
| `command` | string[] | yes | — | Command and arguments (exec form only, no shell) |
| `restart` | string | no | `"no"` | Restart policy: `"no"`, `"on-failure"`, `"always"` |
| `env` | map[string]string | no | — | Environment variables, merged with template env |
| `ready_check` | ReadyCheck | no | — | Optional readiness gate (see below) |

### Corresponding Go type

```go
// ServiceSpec defines a sidecar process to run alongside the main harness.
type ServiceSpec struct {
    Name       string            `json:"name" yaml:"name"`
    Command    []string          `json:"command" yaml:"command"`
    Restart    string            `json:"restart,omitempty" yaml:"restart,omitempty"`
    Env        map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
    ReadyCheck *ReadyCheck       `json:"ready_check,omitempty" yaml:"ready_check,omitempty"`
}

type ReadyCheck struct {
    Type    string `json:"type" yaml:"type"`       // "tcp", "http", "delay"
    Target  string `json:"target" yaml:"target"`   // "localhost:9222", "http://localhost:8080/health", "3s"
    Timeout string `json:"timeout" yaml:"timeout"` // max wait before giving up
}
```

## Config Discovery

The `fabric-agent.yaml` file remains at the root of the template/agent definition. On agent creation, the host-side `fabric` CLI copies the `services` block from `fabric-agent.yaml` into a separate `fabric-services.yaml` file in the agent's home directory (`$HOME/.fabric/fabric-services.yaml`). At container startup, `fabrictool init` reads service definitions from this well-known path.

This separation keeps `fabric-agent.yaml` as the source of truth for template configuration while giving `fabrictool` a clean, dedicated file to consume without needing awareness of the full template config schema.

## Lifecycle Semantics

### Startup order

1. `fabrictool init` runs pre-start hooks (existing)
2. `fabrictool` reads `fabric-services.yaml` from the agent's home directory
3. Services are started in array order
4. If a service has a `ready_check`, fabrictool waits for it to pass before starting the next service
5. After all services are running (or ready), the main harness process is started via the existing supervisor
6. Post-start hooks run (existing)

### Steady state

- Services are monitored in a background goroutine (supervised sidecar model)
- If a service exits and its restart policy allows, it is restarted with backoff
- After 3 consecutive failed restarts, the service is abandoned (no further restart attempts). Failure events are logged to the service's fabrictool lifecycle log
- The main process (harness) remains the "primary" — the container's exit code is determined by the main process, not services

### Shutdown

1. Main process exits (or signal received)
2. Pre-stop hooks run (existing)
3. All services receive SIGTERM
4. After grace period, remaining services receive SIGKILL
5. Session-end hooks run (existing)

### Process identity

Services run with the same UID/GID as the main process (the `fabric` user after `setupHostUser()`).

## Logging

Each service produces two log streams at well-known paths under the agent's home directory:

### Service output logs

Service stdout and stderr are captured to per-service log files:

```
~/.fabric/services/logs/<name>.stdout.log
~/.fabric/services/logs/<name>.stderr.log
```

These contain the raw output from the service process.

### Fabrictool lifecycle logs

Fabrictool writes its own per-service lifecycle log covering management events:

```
~/.fabric/services/logs/<name>.lifecycle.log
```

Events recorded include:
- Service started (with PID, timestamp)
- Service exited (exit code, signal, timestamp)
- Restart attempted (attempt number, timestamp)
- Restart limit reached (after 3 consecutive failures)
- Ready check passed/failed
- Service stopped during shutdown

These lifecycle logs are also forwarded to the Hub logging endpoint via fabrictool's existing OpenTelemetry logging integration, making service health visible in the hosted architecture without additional plumbing.

### Log access

When tmux is enabled, users can create a new tmux window and tail logs from these well-known paths. No special tmux integration is built for services.

## Implementation Surface

### New code

- **`pkg/fabrictool/services/manager.go`** — `ServiceManager` struct
  - `Start(specs []ServiceSpec, uid, gid int) error` — starts all services in order, honoring ready checks
  - `Shutdown(ctx context.Context) error` — graceful stop with timeout
  - Background goroutine per service for restart monitoring (max 3 consecutive failures)
  - Per-service output logs written to `~/.fabric/services/logs/<name>.{stdout,stderr}.log`
  - Per-service lifecycle logs written to `~/.fabric/services/logs/<name>.lifecycle.log`

### Modified code

- **`pkg/api/types.go`** — Add `Services []ServiceSpec` to `FabricConfig`
- **`cmd/fabrictool/commands/init.go`** — Wire ServiceManager into `runInit()`:
  1. After pre-start hooks, read `fabric-services.yaml` from `$HOME/.fabric/`
  2. Start ServiceManager
  3. Start main process via existing supervisor
  4. During shutdown, stop ServiceManager before session-end hooks
- **Agent creation path** (host-side) — Extract `services` block from `fabric-agent.yaml` and write it as `fabric-services.yaml` into the agent's home directory during provisioning

### Unchanged

- The existing supervisor remains responsible for the main harness process only
- Lifecycle hooks are unaffected
- `fabric-agent.yaml` schema gains the `services` field but remains at the template root
