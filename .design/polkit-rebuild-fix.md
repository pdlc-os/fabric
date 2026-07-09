# Fix: Polkit Authorization for Rebuild Server Maintenance Task

**Created:** 2026-04-13
**Status:** Superseded — polkit approach replaced by sudoers rules (polkit 0.105 on Ubuntu LTS does not support JavaScript `.rules` files). See `scripts/starter-hub/gce-start-hub.sh` for the current sudoers-based implementation.
**Related:** `.design/server-routine-maintenance.md`, `pkg/hub/maintenance_executors.go`, `scripts/starter-hub/gce-start-hub.sh`

---

## Problem

The "Rebuild Server from Git" maintenance task fails at step 4 (`systemctl restart fabric-hub`) with:

```
Failed to restart fabric-hub.service: Interactive authentication required.
```

The fabric-hub systemd service runs as the `fabric` user. When the `RebuildServerExecutor` completes the build and attempts to restart the service, `systemctl restart` is invoked by the same `fabric` user. systemd delegates authorization for unit management to polkit, and without an explicit rule, polkit requires interactive authentication (a password prompt) — which cannot be satisfied from a non-interactive server process.

This means the build succeeds but the restart always fails, leaving the old binary running until the service is restarted manually via SSH with sudo.

## Root Cause

The deployment script (`gce-start-hub.sh`) performs all `systemctl` operations through `sudo` because it runs via an SSH session from the deployer's machine. The maintenance executor, by contrast, runs in-process as the `fabric` user and has no sudo access. No polkit rule existed to bridge this gap.

## Fix

Add a polkit rule that grants the `fabric` user permission to manage the `fabric-hub.service` unit without interactive authentication.

### Polkit Rule

Installed at `/etc/polkit-1/rules.d/50-fabric-hub-restart.rules`:

```javascript
polkit.addRule(function(action, subject) {
    if (action.id == "org.freedesktop.systemd1.manage-units" &&
        action.lookup("unit") == "fabric-hub.service" &&
        subject.user == "fabric") {
        return polkit.Result.YES;
    }
});
```

**Scope:** The rule is narrowly scoped:
- Only the `org.freedesktop.systemd1.manage-units` action (start/stop/restart units).
- Only the `fabric-hub.service` unit — no other services.
- Only the `fabric` user — no other accounts.

### Deployment

The rule is installed automatically during full deploys by `gce-start-hub.sh`, in the same phase as the systemd unit file and Caddyfile. It uses the same diff-before-replace pattern as the other config files to avoid unnecessary writes.

### Manual Application

For existing servers that need the fix before the next full deploy:

```bash
sudo tee /etc/polkit-1/rules.d/50-fabric-hub-restart.rules <<'EOF'
polkit.addRule(function(action, subject) {
    if (action.id == "org.freedesktop.systemd1.manage-units" &&
        action.lookup("unit") == "fabric-hub.service" &&
        subject.user == "fabric") {
        return polkit.Result.YES;
    }
});
EOF
```

No restart of polkit or the fabric-hub service is required — polkit picks up new rules immediately.

## Changes

| File | Change |
|------|--------|
| `scripts/starter-hub/gce-start-hub.sh` | Added polkit rule installation to the full-deploy infrastructure config phase |
| `.design/server-routine-maintenance.md` | Added **Privileges** note to the Rebuild Server Executor section documenting the polkit dependency |

## Why Not Sudo (for systemctl)?

The `fabric` user does not have sudo access on the hub server, and granting it would require either a sudoers entry or adding the user to a privileged group. Polkit is the intended authorization mechanism for systemd unit management and provides finer-grained control — the rule authorizes exactly one user for exactly one service, without granting any broader shell-level privilege escalation.

## Companion Fix: Binary Installation Sudoers Rule

The rebuild-server executor also needs to install the compiled binary into `/usr/local/bin/`, which the `fabric` user cannot write to. Unlike systemd management (which has polkit), filesystem operations require a sudoers rule. The deploy script installs a narrowly-scoped entry at `/etc/sudoers.d/fabric-install-binary` that permits only the exact `install` command with the specific source and destination paths. See `pkg/hub/maintenance_executors.go` for the executor implementation.
