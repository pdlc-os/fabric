# Automated QA: User Journey Test Plans

**Created:** 2026-03-20
**Status:** Proposal

---

## Overview

This document proposes methodologies and user journey test plans for systematic QA of the Fabric platform. The goal is to complement existing unit tests (268+ Go test files) with structured integration and user-journey testing across both CLI and Web interfaces.

The recent manual QA of the admin settings UI (see `admin-settings-ui-qa.md`) demonstrated the value of exercising real UI flows end-to-end. This proposal expands that approach into a repeatable framework covering the full platform surface.

---

## Test Methodologies

### M1: Agent-Driven CLI Testing (Self-Hostable)

**How it works:** A Fabric agent (or scripted automation) runs CLI commands against a locally-built `fabric` binary, validates output, and checks side effects (files created, config modified, containers started/stopped).

**Environment:** Build and run everything locally. The agent has access to `make build` to produce a fresh binary before each test run.

**What it covers:**
- Command parsing, flags, and help text
- Config file creation and mutation
- Agent lifecycle without a hub (solo mode)
- Error handling and edge cases (missing args, invalid state)

**Artifacts:** Structured test log with PASS/FAIL per step, plus any stderr output on failure.

**Limitations:** Cannot test container runtime operations if Docker is unavailable in the sandbox. Some commands require a running hub server (covered by M2).

---

### M2: Agent-Driven Hub Server Testing (Self-Hostable)

**How it works:** Build the binary, start a hub server in workstation mode (`fabric server start --foreground --enable-hub --dev-auth`), then exercise CLI commands and HTTP API calls against it. The agent runs the server as a background process and tests against `localhost`.

**Environment:** Requires the ability to bind ports and run the server process. Uses dev-auth to bypass OAuth.

**What it covers:**
- Hub API endpoint behavior (CRUD for groves, agents, brokers, templates)
- CLI-to-hub integration (auth, link, env/secret management)
- Server lifecycle (start, status, stop)
- WebSocket/SSE event streaming
- Admin operations (settings, user management)

**Artifacts:** Server logs (verbose), HTTP request/response logs, structured test results.

---

### M3: Browser-Automated Web UI Testing (Self-Hostable)

**How it works:** Start the hub server with `--enable-web`, then use Playwright or a headless browser to navigate the SPA, interact with forms, and validate rendered state. Screenshots captured at key steps.

**Environment:** Requires Playwright/Puppeteer and a built web frontend (`npm run build` in `web/`). The existing `web/test-scripts/realtime-lifecycle-test.js` is a precedent for this approach.

**What it covers:**
- Page rendering and navigation (27 routes)
- Form submission and validation
- Real-time updates via SSE
- Dark/light theme rendering
- Responsive layout (mobile drawer vs desktop sidebar)
- Authentication flows (dev-auth for testing)

**Artifacts:** Screenshots per step, DOM assertions, console error capture.

---

### M4: External Hub Log Analysis (Non-Self-Hosted)

**How it works:** Test against a running hub instance outside the agent's environment. The agent uses CLI commands and `curl`/API calls, then reviews verbose server logs (provided via file access or log streaming) to validate behavior.

**Environment:** Hub URL and dev-auth token provided as configuration. Agent cannot start/stop the server but can read its logs.

**What it covers:**
- Multi-broker dispatch scenarios
- OAuth authentication flows (real providers)
- Cross-broker agent lifecycle
- Template push/pull with real GCS
- Performance characteristics under real conditions

**Artifacts:** API response validation, log correlation, timing data.

---

### M5: Agent-as-User Dogfooding (Self-Hosted)

**How it works:** Use Fabric to test Fabric. Start agents via the CLI, attach to them, send messages, sync workspaces, and observe the full lifecycle. This is the most realistic test methodology but also the most complex.

**Environment:** Requires Docker runtime and the ability to run containers.

**What it covers:**
- True end-to-end agent lifecycle
- Worktree creation and isolation
- Container provisioning and harness initialization
- Attach/detach and message delivery
- Workspace sync back to host

**Limitations:** Requires container runtime; may not be available in all sandbox environments.

---

## User Journey Test Plans

### Area 1: Grove Initialization & Configuration

**Methodology:** M1 (CLI only)

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 1.1 | Fresh grove init | `fabric init` in empty dir → verify `.fabric/` structure, default templates, settings.yaml |
| 1.2 | Re-init existing grove | `fabric init` in dir with existing `.fabric/` → verify no data loss, idempotent behavior |
| 1.3 | Global grove operations | `fabric --global init` → verify `~/.fabric/` setup |
| 1.4 | Grove discovery | Create nested dirs with `.fabric/` → verify resolution order (flag > local > global) |
| 1.5 | Config get/set | `fabric config set key value` → `fabric config get key` roundtrip |
| 1.6 | Settings validation | `fabric config validate` with valid/invalid settings.yaml |
| 1.7 | Grove list & prune | Create groves → `fabric grove list` → delete dir → `fabric grove prune` |
| 1.8 | Grove reconnect | Move project dir → `fabric grove reconnect` → verify config updated |

---

### Area 2: Template Management

**Methodology:** M1 (local), M2 (hub sync/push/pull)

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 2.1 | List default templates | `fabric templates list` → verify built-in templates present |
| 2.2 | Create custom template | `fabric templates create --name foo --harness claude` → verify files created |
| 2.3 | Clone template | `fabric templates clone base-template new-template` → verify independent copy |
| 2.4 | Delete template | `fabric templates delete foo` → verify removed, cannot start agent with it |
| 2.5 | Template import | `fabric templates import <path>` → verify template registered |
| 2.6 | Hub template sync | `fabric template sync` → verify template registered in hub |
| 2.7 | Hub template push/pull | Push template files → pull on another grove → verify contents match |
| 2.8 | Template status | `fabric template status` → verify shows sync state |

---

### Area 3: Agent Lifecycle (Solo/Local Mode)

**Methodology:** M1, M5 (if containers available)

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 3.1 | Create agent | `fabric create agent-name` → verify agent directory, worktree, config |
| 3.2 | Start agent | `fabric start agent-name` → verify container running, status updates |
| 3.3 | List agents | Start multiple agents → `fabric list` → verify all shown with status |
| 3.4 | View agent logs | `fabric logs agent-name` → verify log output |
| 3.5 | Look at terminal | `fabric look agent-name` → verify terminal snapshot |
| 3.6 | Send message | `fabric message agent-name "hello"` → verify delivery |
| 3.7 | Attach/detach | `fabric attach agent-name` → interact → detach → verify agent continues |
| 3.8 | Stop agent | `fabric stop agent-name` → verify container stopped, status updated |
| 3.9 | Resume agent | `fabric resume agent-name` → verify container restarted |
| 3.10 | Delete agent | `fabric delete agent-name` → verify cleanup (container, worktree) |
| 3.11 | Soft delete & restore | `fabric delete --soft agent-name` → `fabric restore agent-name` |
| 3.12 | Sync workspace | `fabric sync agent-name` → verify files pulled back |
| 3.13 | Create with inline config | `fabric start --set key=value agent-name` → verify config applied |
| 3.14 | Multiple agents isolation | Start 2 agents → verify separate worktrees and branches |

---

### Area 4: Hub Authentication & Connection

**Methodology:** M2

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 4.1 | Dev auth login | `fabric hub auth login` with dev token → verify credentials stored |
| 4.2 | Hub status | `fabric hub status` → verify connection info displayed |
| 4.3 | Hub enable/disable | `fabric hub enable` → verify config → `fabric hub disable` → verify removed |
| 4.4 | Hub link/unlink | `fabric hub link` → verify grove registered → `fabric hub unlink` |
| 4.5 | Token management | `fabric hub token create` → `list` → `revoke` → `delete` |
| 4.6 | Env var management | `fabric hub env set KEY=val` → `get KEY` → `list` → `clear` |
| 4.7 | Secret management | `fabric hub secret set KEY=val` → `get KEY` → `list` → `clear` |
| 4.8 | OAuth login (external) | Browser-based OAuth flow with real provider → verify session |

---

### Area 5: Hub-Mode Agent Lifecycle

**Methodology:** M2, M4 (for multi-broker)

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 5.1 | Create agent via hub | `fabric create agent-name` (hub-enabled) → verify hub record + broker dispatch |
| 5.2 | Start via hub | `fabric start agent-name` → verify dispatch to broker, container started |
| 5.3 | Stop via hub | `fabric stop agent-name` → verify dispatch, container stopped |
| 5.4 | Delete via hub | `fabric delete agent-name` → verify hub record removed, broker cleanup |
| 5.5 | Attach via hub | `fabric attach agent-name` → verify WebSocket PTY relay works |
| 5.6 | Logs via hub | `fabric logs agent-name` → verify log retrieval through hub |
| 5.7 | Broker heartbeat | Start broker → verify heartbeat appears in hub, status healthy |
| 5.8 | Broker disconnect | Stop broker → verify hub marks broker offline |

---

### Area 6: Server Management

**Methodology:** M2

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 6.1 | Server start modes | Start with `--enable-hub`, `--enable-broker`, both → verify components active |
| 6.2 | Server status | `fabric server status` → verify reports running components |
| 6.3 | Server stop | `fabric server stop` → verify graceful shutdown |
| 6.4 | Server restart | `fabric server restart` → verify PID changes, state preserved |
| 6.5 | Workstation mode | `fabric server start --enable-hub --enable-broker` → verify combo server |
| 6.6 | Health endpoint | `curl /healthz` → verify composite health response |
| 6.7 | Metrics endpoint | `curl /metrics` → verify Prometheus metrics emitted |

---

### Area 7: Web UI - Dashboard & Navigation

**Methodology:** M3

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 7.1 | Login page | Navigate to `/login` → verify OAuth provider buttons rendered |
| 7.2 | Dev auth login | Login with dev token → verify redirect to dashboard |
| 7.3 | Dashboard loads | Navigate to `/` → verify quick actions, stats displayed |
| 7.4 | Sidebar navigation | Click each nav item → verify correct page loads, breadcrumb updates |
| 7.5 | 404 page | Navigate to `/nonexistent` → verify 404 page shown |
| 7.6 | Theme toggle | Switch dark/light → verify all pages render correctly in both |
| 7.7 | Mobile responsive | Resize viewport → verify drawer nav replaces sidebar |
| 7.8 | Session expiry | Wait for session timeout → verify redirect to login |

---

### Area 8: Web UI - Grove Management

**Methodology:** M3

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 8.1 | Grove list page | Navigate to `/groves` → verify groves listed with status badges |
| 8.2 | Create grove | Navigate to `/groves/new` → fill form → submit → verify created |
| 8.3 | Grove detail page | Click grove → verify agent list, grove info displayed |
| 8.4 | Grove settings | Navigate to grove settings → modify → save → verify persisted |
| 8.5 | Grove schedules | Navigate to grove schedules → create/edit/delete schedule |
| 8.6 | View toggle | Toggle grid/list view on grove page → verify both render correctly |
| 8.7 | Real-time agent updates | Start agent via API → verify grove detail updates via SSE without refresh |

---

### Area 9: Web UI - Agent Management

**Methodology:** M3

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 9.1 | Agent list page | Navigate to `/agents` → verify agents listed with status badges |
| 9.2 | Create agent | Navigate to `/agents/new` → fill form → submit → verify created |
| 9.3 | Agent detail page | Click agent → verify status, logs, config displayed |
| 9.4 | Agent configure | Navigate to `/agents/:id/configure` → modify settings → save |
| 9.5 | Agent terminal | Navigate to `/agents/:id/terminal` → verify xterm loads, PTY connects |
| 9.6 | Agent lifecycle from UI | Create → start → stop → delete agent entirely through web UI |
| 9.7 | Real-time status updates | Change agent status via API → verify UI updates in real-time |
| 9.8 | Agent log viewer | Open agent detail → verify log viewer shows output, auto-scrolls |
| 9.9 | Agent message viewer | Send message via API → verify message appears in UI |

---

### Area 10: Web UI - Admin Pages

**Methodology:** M3

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 10.1 | Server config (all tabs) | Navigate to `/admin/server-config` → verify all 6 tabs render, edit, save |
| 10.2 | Sensitive field masking | Verify tokens/passwords shown as `********`, not plaintext |
| 10.3 | Settings reset | Modify fields → click Reset → verify restored to last-saved state |
| 10.4 | User management | Navigate to `/admin/users` → verify user list, roles displayed |
| 10.5 | Group management | Navigate to `/admin/groups` → create group → add members → verify |
| 10.6 | Scheduler admin | Navigate to `/admin/scheduler` → view scheduled jobs |
| 10.7 | Non-admin access denied | Login as non-admin → navigate to admin pages → verify 403/redirect |

---

### Area 11: Web UI - User Profile

**Methodology:** M3

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 11.1 | Env vars page | Navigate to `/profile/env` → add/edit/delete env var → verify persisted |
| 11.2 | Secrets page | Navigate to `/profile/secrets` → add/delete secret → verify masked display |
| 11.3 | Tokens page | Navigate to `/profile/tokens` → create token → verify shown once → list |
| 11.4 | Profile settings | Navigate to `/profile/settings` → modify preferences → save |
| 11.5 | Env var scopes | Set env vars at user/grove/broker scopes → verify correct scope applied |

---

### Area 12: Web UI - Broker Management

**Methodology:** M3

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 12.1 | Broker list | Navigate to `/brokers` → verify brokers listed with health status |
| 12.2 | Broker detail | Click broker → verify details, capabilities, connected groves |
| 12.3 | Broker health updates | Stop broker process → verify UI shows broker as offline |

---

### Area 13: Web UI - Real-Time & SSE

**Methodology:** M3 (extends the existing `realtime-lifecycle-test.js`)

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 13.1 | SSE connection | Open page → verify EventSource connects to `/events` |
| 13.2 | Agent status stream | Create agent via API → verify UI updates without refresh |
| 13.3 | Multi-subscription | Open grove page (subscribes to grove + agent events) → trigger both → verify |
| 13.4 | Reconnection | Kill SSE connection → verify client reconnects automatically |
| 13.5 | Notification tray | Trigger notification → verify tray shows it, ack clears it |

---

### Area 14: Notifications & Scheduling

**Methodology:** M1 (CLI), M2 (hub), M3 (web)

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 14.1 | Subscribe to notifications | `fabric notifications subscribe` → verify subscription created |
| 14.2 | Receive notification | Trigger event → verify notification delivered |
| 14.3 | Acknowledge notification | `fabric notifications ack` → verify cleared |
| 14.4 | Create schedule | `fabric schedule create` → verify cron job registered |
| 14.5 | Schedule triggers agent | Set up schedule → wait for trigger → verify agent started |

---

### Area 15: Shared Directories & GitHub App

**Methodology:** M1, M3

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 15.1 | Add shared dir | `fabric shared-dir add <path>` → verify mount config |
| 15.2 | List shared dirs | `fabric shared-dir list` → verify all listed |
| 15.3 | Remove shared dir | `fabric shared-dir remove <path>` → verify removed |
| 15.4 | GitHub app setup page | Navigate to `/github-app/installed` → verify setup instructions shown |

---

### Area 16: Error Handling & Edge Cases

**Methodology:** M1, M2

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 16.1 | Unknown command | `fabric nonexistent` → verify helpful error message |
| 16.2 | Missing required args | `fabric start` (no name) → verify usage displayed |
| 16.3 | Agent not found | `fabric logs nonexistent` → verify clean error |
| 16.4 | Hub unreachable | `fabric hub status` with bad URL → verify timeout/error message |
| 16.5 | Double start | Start already-running agent → verify idempotent or clear error |
| 16.6 | Delete running agent | `fabric delete running-agent` → verify prompt or `--force` required |
| 16.7 | Invalid config | Set malformed settings.yaml → verify graceful error on commands |
| 16.8 | Concurrent operations | Start multiple agents simultaneously → verify no race conditions |

---

### Area 17: Doctor & System Checks

**Methodology:** M1

**Journeys:**

| ID | Journey | Key Steps |
|----|---------|-----------|
| 17.1 | Doctor check | `fabric doctor` → verify checks runtime, dependencies, config |
| 17.2 | Version output | `fabric version` → verify version, commit hash, build info |
| 17.3 | Help output | `fabric --help`, `fabric start --help` → verify complete, well-formatted |
| 17.4 | Shell completions | Generate completions for bash/zsh → verify valid syntax |

---

## Implementation Priority

### Phase 1: CLI Smoke Tests (M1)
- Areas 1, 2 (local), 16, 17
- Can run in any environment with a built binary
- Highest value-to-effort ratio for catching regressions

### Phase 2: Hub Integration Tests (M2)
- Areas 4, 5, 6
- Requires server start capability
- Validates the core hosted architecture

### Phase 3: Web UI Journey Tests (M3)
- Areas 7, 8, 9, 10, 11, 12, 13
- Requires Playwright + built web frontend
- Catches rendering bugs, broken interactions, SSE issues

### Phase 4: Full E2E Dogfooding (M5)
- Area 3 (with real containers)
- Requires Docker runtime
- Most realistic but most constrained by environment

### Phase 5: External Hub Testing (M4)
- Areas 5 (multi-broker), 4.8 (real OAuth)
- Requires access to a running external hub
- Tests production-like scenarios

---

## Test Infrastructure Needs

| Need | Status | Notes |
|------|--------|-------|
| Go unit tests | Existing (268+ files) | Strong coverage of business logic |
| CLI output assertions | To build | Script or Go test harness for command output validation |
| HTTP API test client | To build | Reusable client for hub API assertions (could extend `hubclient/`) |
| Playwright test suite | Scaffold exists | Expand `web/test-scripts/realtime-lifecycle-test.js` |
| Screenshot diffing | Not started | Compare screenshots across runs for visual regression |
| CI integration | Partial | `make ci` runs unit tests; need to add integration test targets |
| Test data seeding | Not started | Scripts to populate hub with representative test data |
| Log analysis tools | Not started | Parse verbose server logs for assertion (M4 methodology) |

---

## Relationship to Existing Tests

This proposal does **not** replace existing unit tests. It layers on top of them:

```
Unit Tests (268 files)         → Function/package correctness
  ↑ existing
CLI Journey Tests (M1)         → Command-level behavior
Hub Integration Tests (M2)     → API + CLI-to-server flows
Web UI Tests (M3)              → Rendered UI + user interactions
E2E Dogfooding (M5)            → Full system behavior
  ↓ proposed
```

Each layer catches different classes of bugs. Unit tests catch logic errors; journey tests catch integration issues, UX regressions, and configuration problems that only manifest when components interact.
