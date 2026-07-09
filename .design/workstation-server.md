# Workstation Server Mode

## Overview

Evolve `fabric server` to support a **workstation mode**: a single-user, local-first configuration optimized for developers running Fabric on their own machine. This complements the existing multi-user Hub deployment use-case.

Workstation mode is the **default** behavior of `fabric server start`. A bare `fabric server start` with no flags enables all components (Hub, Broker, Web), uses local backends, dev-auth, and binds to `127.0.0.1`. Production deployments opt in via `--production`, which restores the current flag-driven composition model.

The command also gains daemon lifecycle management (start/stop/restart/status) directly on `fabric server` (mirroring `fabric broker`), and a `--foreground` flag for integration with process managers like systemd and launchd.

## Motivation

Today, running Fabric locally as a "personal server" requires:

```bash
fabric server start --enable-hub --enable-runtime-broker --enable-web --dev-auth --auto-provide
```

This is verbose and leaks infrastructure concerns (Hub, Broker, dev-auth) that a single-user operator shouldn't need to think about. Meanwhile, the `fabric broker` command already has polished daemon management (`start`/`stop`/`restart`/`status` with `--foreground`), but `fabric server` has only `start` and always runs in the foreground.

**Goals:**
1. Make single-workstation usage the easy, zero-flag default path.
2. Add daemon lifecycle to `fabric server` (parity with `fabric broker`).
3. Add `--foreground` for systemd/launchd integration.
4. Disable GCP-dependent features (secrets, storage, Cloud Logging) by default.
5. Keep the existing flag-based composition available for production Hub deployments via `--production`.

## Design

### 1. Default Workstation Mode with `--production` Opt-In

`fabric server start` with no flags operates in workstation mode. This is the simple, "just works" path. A `--production` flag opts into the current explicit-flag composition model for multi-user Hub deployments.

#### Mode Selection Logic

```
fabric server start                          -> workstation mode (default)
fabric server start --production             -> production mode (explicit flags required)
fabric server start --production --enable-hub --enable-web  -> production, hub + web only
```

**Workstation mode** implies:

| Implied Setting | Value | Notes |
|---|---|---|
| `--enable-hub` | `true` | |
| `--enable-runtime-broker` | `true` | |
| `--enable-web` | `true` | |
| `--dev-auth` | `true` | Auto-generates token |
| `--auto-provide` | `true` | |
| `--host` | `127.0.0.1` | Loopback only for single-user security |
| `secrets.backend` | `"local"` | SQLite-backed secrets |
| `storage.provider` | `"local"` | Local filesystem storage |
| GCP Cloud Logging | disabled | No `FABRIC_LOG_GCP` |

**Production mode** (`--production`) restores the current behavior:
- No components enabled by default; the operator must explicitly pass `--enable-hub`, `--enable-runtime-broker`, `--enable-web`, etc.
- Binds to `0.0.0.0` by default.
- GCP backends are available and configurable via the existing flags/env vars.
- Dev-auth is off unless explicitly passed.

Explicit flags override implied workstation defaults, so `fabric server start --no-web` or `fabric server start --host 0.0.0.0` work in workstation mode.

#### Why `--production` instead of inverting to `--disable-*` flags

Three options were considered:

1. **Invert all `--enable-*` flags to `--disable-*`**: This would make workstation the default but creates a migration burden — every existing production deployment script using `--enable-hub` would need updating. It also doubles the flag surface (`--enable-hub` / `--disable-hub`).

2. **`--production` flag**: A single flag that says "I'm a production deployment, give me explicit control." Clean, minimal surface area, no migration needed for existing deployments that simply add `--production`. Existing `--enable-*` flags continue to work in both modes (they're just redundant in workstation mode).

3. **`--workstation` flag**: Requires the operator to remember and pass a flag for the common case. The whole point is that the common case should be zero-ceremony.

**Decision: `--production`**. It optimizes for the common case (workstation) while keeping the escape hatch simple and explicit.

#### Implementation

Early in `runServerStart()`, before the existing flag-changed checks, determine the mode and set defaults:

```go
productionMode := cmd.Flags().Changed("production") && production

if !productionMode {
    // Workstation mode: enable everything by default
    if !cmd.Flags().Changed("enable-hub") {
        enableHub = true
    }
    if !cmd.Flags().Changed("enable-runtime-broker") {
        enableRuntimeBroker = true
        cfg.RuntimeBroker.Enabled = true
    }
    if !cmd.Flags().Changed("enable-web") {
        enableWeb = true
    }
    if !cmd.Flags().Changed("dev-auth") {
        enableDevAuth = true
        cfg.Auth.Enabled = true
    }
    if !cmd.Flags().Changed("auto-provide") {
        serverAutoProvide = true
    }
    if !cmd.Flags().Changed("host") {
        cfg.Host = "127.0.0.1"
    }
    // Force local backends unless explicitly overridden
    if !cmd.Flags().Changed("storage-bucket") {
        cfg.Storage.Provider = "local"
    }
    cfg.Secrets.Backend = "local"
}
```

The existing `cmd.Flags().Changed()` guards ensure explicit overrides always win, regardless of mode.

### 2. Daemon Lifecycle for `fabric server`

Add `stop`, `restart`, and `status` subcommands to `fabric server`, mirroring the existing `fabric broker` implementation.

#### Current State

| Command | `fabric broker` | `fabric server` |
|---|---|---|
| `start` | daemon (default) or `--foreground` | foreground only |
| `stop` | sends SIGTERM via PID file | does not exist |
| `restart` | stop + start | does not exist |
| `status` | daemon + health check | does not exist |

#### Proposed State

| Command | Behavior |
|---|---|
| `fabric server start` | Daemon by default, workstation mode; `--foreground` for foreground |
| `fabric server start --production --enable-hub` | Production mode, daemon by default |
| `fabric server stop` | SIGTERM via PID file |
| `fabric server restart` | Stop + start with same args |
| `fabric server status` | Daemon status + component health checks |

#### PID/Log File Naming

The `pkg/daemon` package currently hardcodes `broker.pid` / `broker.log`. This needs to be generalized.

Add a `component` parameter to daemon functions:

```go
// Before:
const PIDFile = "broker.pid"
const LogFile = "broker.log"

// After:
func PIDFileName(component string) string { return component + ".pid" }
func LogFileName(component string) string { return component + ".log" }
```

The server uses `"server"` as the component, producing `server.pid` / `server.log` in `~/.fabric/`. The broker keeps `"broker"` for backward compatibility. Separate PID files allow a standalone broker and a full server to coexist independently, with port-conflict detection (already exists in `checkPort()`) preventing actual runtime collisions.

#### `fabric broker` Delegation Change

Currently `fabric broker start` delegates to `fabric server start --enable-runtime-broker` (both foreground and daemon modes). This should continue unchanged — the broker command remains a convenient alias for broker-only operation.

The new `fabric server start` (daemon mode) would delegate similarly to `fabric server start` (foreground) under the hood, just as `fabric broker start` does today.

### 3. `--foreground` Flag

Add `--foreground` to `fabric server start`. When set:
- Run the server process in the current terminal (current behavior, now opt-in)
- Do not write a PID file
- Stdout/stderr go to the terminal
- Process exits on SIGINT/SIGTERM

When **not** set (new default):
- Fork a detached child process (via `pkg/daemon`)
- Redirect stdout/stderr to `~/.fabric/server.log`
- Write PID to `~/.fabric/server.pid`
- Parent exits after confirming child started

This matches the `fabric broker start` behavior exactly.

**systemd integration example:**
```ini
[Unit]
Description=Fabric Workstation Server
After=network.target docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/fabric server start --foreground
ExecStop=/usr/local/bin/fabric server stop
Restart=on-failure
User=developer

[Install]
WantedBy=multi-user.target
```

**launchd integration example:**
```xml
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>io.fabric.server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/fabric</string>
        <string>server</string>
        <string>start</string>
        <string>--foreground</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

Note: `--production` is not needed in the service file examples above — workstation mode defaults are appropriate for a single-user managed service. Production deployments would add `--production` and the relevant `--enable-*` flags.

### 4. GCP Feature Gating

Several features default to GCP services and should be explicitly opt-in rather than silently failing or requiring credentials:

| Feature | Current Default | Workstation Default | Production Flag |
|---|---|---|---|
| Secrets backend | `"local"` | `"local"` | `--secrets-backend=gcpsm` |
| Storage provider | `"local"` | `"local"` | `--storage-bucket gs://...` |
| Cloud Logging | env-driven (`FABRIC_LOG_GCP`, `K_SERVICE`) | disabled | `FABRIC_LOG_GCP=true` |
| OAuth (Google/GitHub) | env-driven | disabled (dev-auth) | `FABRIC_SERVER_OAUTH_*` env vars |
| Telemetry GCP creds | hub-injected | not injected | configure via secrets |

The current defaults are already correct for local use. Workstation mode simply guarantees the local path by:
- Forcing `cfg.Secrets.Backend = "local"`
- Forcing `cfg.Storage.Provider = "local"` (unless `--storage-bucket` is given)
- Not setting `FABRIC_LOG_GCP`

No code changes are needed to the secret or storage backends themselves — they already support `"local"` mode.

### 5. Loopback Binding by Default

In workstation mode, the server binds to `127.0.0.1` instead of `0.0.0.0`. This is the secure default for single-user operation — the server is only accessible from the local machine.

To expose the server on the network (e.g., for accessing from another device on the LAN), the operator explicitly overrides:

```bash
fabric server start --host 0.0.0.0
```

Production mode (`--production`) retains `0.0.0.0` as the default, as production deployments typically sit behind a reverse proxy or load balancer.

### 6. `fabric server status` Command

Report composite status of all components:

```
Fabric Server Status
  Mode:          workstation
  Daemon:        running (PID: 12345)
  Log file:      /home/user/.fabric/server.log
  PID file:      /home/user/.fabric/server.pid
  Listening:     127.0.0.1:8080

Components:
  Hub API:       running (port 8080, mounted on web)
  Runtime Broker: running (port 9800)
  Web Frontend:  running (port 8080)

Broker:
  ID:            abc-123
  Name:          hostname
  Groves:        2 (global, my-project)
  Auto-provide:  true
```

This would probe the health endpoints (`/healthz`) on the known ports, and check daemon PID status.

## Implementation Plan

### Phase 1: Daemon Lifecycle (cmd/server.go, pkg/daemon) ✅ COMPLETE

1. ✅ **Generalize `pkg/daemon`**: Parameterize PID/log filenames by component name (`PIDFileName`, `LogFileName`, `StartComponent`, `StopComponent`, `StatusComponent`, etc.). Legacy broker-specific functions preserved as thin wrappers.
2. ✅ **Add `--foreground` flag** to `fabric server start` (default: false, daemon mode).
3. ✅ **Add `fabric server stop`**: Read `server.pid`, send SIGTERM.
4. ✅ **Add `fabric server restart`**: Stop + start with new binary.
5. ✅ **Add `fabric server status`**: Daemon status + component health checks (probes Hub, Broker, Web endpoints).
6. ✅ **Invert default**: `fabric server start` runs as daemon unless `--foreground`.

### Phase 2: Default Workstation Mode (cmd/server.go) ✅ COMPLETE

1. ✅ **Add `--production` flag** that opts into explicit-flag mode.
2. ✅ **Set workstation defaults** when `--production` is not present (all components enabled, dev-auth, auto-provide, loopback binding, local backends).
3. ✅ **Update help text and examples** to feature the zero-flag workstation experience prominently.
4. ✅ **Update `fabric broker` delegation** to pass `--production` to avoid triggering workstation defaults.
5. ✅ **Disable GCP logging** in workstation mode unless explicitly enabled via `FABRIC_LOG_GCP`.
6. ✅ **Force local storage/secrets backends** in workstation mode unless explicitly overridden.

### Phase 3: Configuration Support (pkg/config) ✅ COMPLETE

1. ✅ **Support `mode: production` in `settings.yaml`** so production deployments can set the mode once:
   ```yaml
   server:
     mode: production
   ```
   When `mode: production` is set in config, the server behaves as if `--production` were passed. Workstation mode remains the default when no mode is configured.
2. ✅ **Persist daemon args** so `fabric server restart` can re-launch with the same flags without requiring the user to re-specify them. Store in `~/.fabric/server-args.json`.

### Phase 4: Polish ✅ COMPLETE

1. ✅ **First-run experience**: `fabric server start` prints the dev token and a quickstart URL (Web UI URL + `export FABRIC_DEV_TOKEN=...`). Both daemon mode (reads from `~/.fabric/dev-token`) and foreground mode (prints after all components start) are covered.
2. ✅ **`fabric server install`**: Generate systemd/launchd service files for the current platform. Supports `--production` flag. Outputs service file to stdout with installation instructions on stderr.

## Files to Modify

| File | Changes |
|---|---|
| `cmd/server.go` | Add `--foreground`, `--production` flags; add `stop`, `restart`, `status` subcommands; implement workstation-mode defaults; default to loopback binding |
| `pkg/daemon/daemon.go` | Parameterize PID/log filenames by component name |
| `cmd/broker.go` | Update daemon calls to use new parameterized API |
| `pkg/config/hub_config.go` | Add `Mode` field to `GlobalConfig` for `settings.yaml` support |

## Backward Compatibility

- **`fabric server start` behavior change**: Previously ran in foreground with no components enabled. Now daemonizes in workstation mode with all components enabled. This is a deliberate UX improvement — the old behavior required verbose flags to be useful at all.
  - Users who relied on foreground execution can add `--foreground`.
  - Users who relied on selective component enabling can add `--production`.
  - Production deployments using process managers (systemd, Cloud Run) should add `--production --foreground` and their existing `--enable-*` flags.
- `fabric server start --production --enable-hub --enable-runtime-broker --foreground` is equivalent to the old `fabric server start --enable-hub --enable-runtime-broker`.
- `fabric broker start/stop/restart/status` continue to work unchanged. They manage a separate `broker.pid` and only start the runtime broker component.
- The `fabric broker start` delegation to `fabric server start --enable-runtime-broker` continues unchanged (it will need to pass `--production` internally to avoid triggering workstation defaults).
