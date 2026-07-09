# Toward MVP: Remaining Work Chunks

**Created:** 2026-02-18

This document identifies the remaining major chunks of work to advance Fabric toward a minimum viable product for the hosted platform. It is based on a cross-cutting audit of all design documents, milestones, and the actual codebase as of 2026-02-18.

---

## What's Already Working

Before outlining what's next, it's worth noting the foundation that's in place. The gap is narrower than the individual design docs suggest — many features they list as pending are actually implemented.

### CLI-Based Hosted Workflow (End-to-End) ✅

The full agent lifecycle works via the CLI:
- `fabric hub auth login` — authenticate with Hub
- `fabric create` / `fabric start` — provision and start agents on remote brokers
- `fabric attach` — interactive PTY via WebSocket through Hub
- `fabric sync to/from` — bidirectional workspace synchronization
- `fabric message` — send messages to running agents (including broadcast)
- `fabric stop` / `fabric delete` — lifecycle management
- `fabric template import` — import agent definitions from Claude/Gemini ecosystems

### Web Frontend (M1-M7, M9) ✅

- Koa server with Lit SSR, Web Awesome components, OAuth (Google/GitHub)
- Hub API proxy with auth forwarding
- Grove and agent management pages with start/stop/delete actions
- SSE + NATS server infrastructure (M7 — NATS client, SSE manager, `/events` endpoint) *(NATS approach abandoned 2026-02-19; being replaced by in-process Go channels — see `web-realtime.md`)*
- Full xterm.js terminal with WebSocket PTY proxy (M9)

### Backend Infrastructure ✅

- Hub API server with SQLite store (all CRUD endpoints)
- Runtime Broker with agent lifecycle, WebSocket control channel, NAT traversal
- API key management (generation, validation, revocation, expiration — backend complete)
- Agent services/sidecars (ServiceManager with lifecycle, restart, ready checks)
- Kubernetes runtime (substantially complete — pod lifecycle, sync, exec, resource limits)
- Versioned settings system (schema v1 with migration tooling)
- Broker heartbeat and health monitoring

---

## The Remaining Chunks

### Chunk 1: Client Real-Time State Management

**Ref:** Frontend M8 (`frontend-milestones.md`)

**Why this is first:** The server-side SSE infrastructure is complete (M7) — the Koa server has an SSE manager and `/events` endpoint with query-parameter-based subscriptions. *(Note: The NATS backend for this is being replaced by in-process Go channels as the Koa BFF is consolidated into the Go binary — see `web-realtime.md`.)* What's missing is the client side: the browser doesn't connect to the SSE endpoint or react to events. Without this, the web UI feels dead — users must manually refresh to see state changes.

**Scope:**

The server-side plumbing is done (M7). This chunk is purely client-side:

- SSE client class: `connect(subjects)` builds URL with query params, opens `EventSource`, handles reconnection with backoff and `Last-Event-ID` resume
- StateManager with view scoping: maps navigation to NATS subjects (`/groves/:id` → `sub=grove.{id}.>`), maintains full in-memory state map, merges deltas from SSE events
- View-scoped subscription lifecycle: `setScope()` called on navigation, closes/reopens SSE connection with correct subjects
- Component wiring: grove list, agent list, agent detail pages receive live updates; status badges update without refresh; created/deleted events add/remove items from lists
- Hydration from SSR data: parse `__FABRIC_DATA__` into StateManager on page load, SSE deltas applied on top

**Key deliverables:**
- [ ] SSE client class with reconnection and `Last-Event-ID` resume
- [ ] StateManager with view-scoped subscriptions
- [ ] Navigation-driven subscription lifecycle
- [ ] Component wiring for reactive UI updates
- [ ] SSR data hydration into StateManager

---

### Chunk 2: Web Agent Lifecycle + Taskless Refactor

**Ref:** Frontend M10, M11 (`frontend-milestones.md`), `taskless-refactor.md`

**Why this is second:** The web UI can display and manage existing agents but cannot create new ones. Users must context-switch to the CLI for the most basic operation. The taskless refactor removes an unnecessary gate that blocks interactive-first workflows — currently `fabric start` errors if no task is provided unless `--attach` is set.

**Scope:**

Taskless refactor (backend — design doc ready, changes scoped):
- Remove task-required validation from `pkg/agent/run.go`
- Remove prompt checks from `pkg/hub/handlers.go` (both existing-agent and new-agent paths)
- Remove workspace bootstrap task gate from `cmd/common.go`
- Simplify dispatch condition in Hub handlers
- Update tests

Agent creation wizard (frontend — M10):
- `<fabric-create-agent-dialog>` component
- Template selector (requires minimal M11 template browsing)
- Configuration form: name, optional task/prompt, branch
- Form validation, API submission, progress tracking
- Post-creation redirect to agent detail or terminal

Template browsing (frontend — partial M11):
- Template list endpoint integration (Hub API already exists)
- Template card display with harness type and description
- Enough to power the agent creation template picker; full template management UI (upload, edit, clone, delete) can follow

**Key deliverables:**
- [ ] Taskless refactor: 7 critical code changes per `taskless-refactor.md`
- [ ] Agent creation dialog component
- [ ] Template list/picker component (subset of M11)
- [ ] End-to-end: create agent from web UI → agent starts → terminal available

---

### Chunk 3: User, Permission & API Key Management

**Ref:** Auth Phases 3-5 (`auth-milestones.md`), Frontend M12, M13, M15 (`frontend-milestones.md`)

**Why this is third:** Solo-mode and single-user hosted mode work without this. But any deployment with more than one person needs access control. The API key backend is complete; user/group management and permissions are the gaps.

**Scope:**

API key UI (M15 — backend exists, UI does not):
- API key list page showing prefix, last-used, expiry
- Create key dialog with name, optional expiry, scope
- One-time key display with copy button
- Revoke confirmation dialog

User & group management (M12):
- User list page with role badges, search
- User detail with group memberships
- Group CRUD (create, list, detail, add/remove members)
- Backend: user/group store and endpoints may need expansion

Permissions & policy management (M13):
- Policy list with scope/effect filtering
- Policy editor: scope type, resource type, action checkboxes, effect, priority
- Principal selector (user/group picker)
- Access evaluation tool for debugging

Auth hardening (Phases 4-5):
- Unified `authMiddleware` with token type detection (dev, OAuth session, API key, agent JWT)
- `UserTokenService` for Hub-issued JWTs (replaces session-only web auth)
- Rate limiting on auth endpoints
- Audit logging
- Token revocation

**Key deliverables:**
- [ ] API key management UI (M15)
- [ ] User list and detail pages (M12)
- [ ] Group CRUD UI (M12)
- [ ] Policy editor and access evaluator (M13)
- [ ] Unified auth middleware (Phase 5)
- [ ] Rate limiting and audit logging (Phase 4)

---

### Chunk 4: Environment, Secrets & Configuration UI

**Ref:** Frontend M14 (`frontend-milestones.md`)

**Why this is fourth:** Agents need credentials (LLM API keys, Git tokens, etc.). Currently these are managed through template files and environment variables manually. The Hub API for env vars and secrets already exists (`/api/v1/env`, `/api/v1/secrets`). What's missing is the web UI and encrypted-at-rest secret storage.

**Scope:**

Environment variables UI:
- Env settings page with scope selector (user / grove / broker)
- Env var table with key/value display, sensitive value masking
- Create/edit/delete env var dialogs
- Key validation (UPPER_SNAKE_CASE convention)

Secrets UI:
- Secret list showing key and metadata (no values)
- Create/update secret dialogs (write-only value field)
- Version tracking display
- Delete confirmation

Secret storage hardening:
- Encrypted-at-rest secrets in SQLite (currently plaintext)
- Consider external secret store integration (Vault, cloud KMS) as a future option
- Secret rotation support

**Key deliverables:**
- [ ] Env var management page with scope switching
- [ ] Secret management page (metadata-only display)
- [ ] Create/edit/delete dialogs for both
- [ ] Secret encryption at rest

---

### Chunk 5: Production Hardening & Deployment

**Ref:** Frontend M16, M17 (`frontend-milestones.md`), K8s M5 (`kubernetes/milestones.md`)

**Why this is last:** This is "make it shippable" work that should come after features stabilize.

**Scope:**

Web production hardening (M16):
- CSRF protection
- Input sanitization
- Asset bundling, minification, compression (gzip/brotli)
- Cache headers for static assets
- Global error boundary with user-friendly error pages
- Structured JSON logging with request ID tracing

Cloud Run deployment (M17):
- Multi-stage Dockerfile for web frontend
- Cloud Run service configuration
- Secret Manager integration for OAuth credentials, session secrets
- IAM and VPC connector for Hub access
- CI/CD pipeline (build → test → deploy)

Kubernetes runtime hardening (K8s M5):
- Replace env var injection with proper Kubernetes Secrets
- Pod SecurityContext (FSGroup, non-root)
- Status reconciliation with K8s Pod state
- Cleanup of Secrets, ConfigMaps, PVCs on agent delete

**Key deliverables:**
- [ ] Web security hardening (CSRF, sanitization, CSP)
- [ ] Asset optimization and caching
- [ ] Structured logging and error boundaries
- [ ] Cloud Run Dockerfile and deployment config
- [ ] CI/CD pipeline
- [ ] K8s secret management and pod security

---

## Sequencing and Dependencies

```
Chunk 1: Real-Time Web Experience
   │
   ▼
Chunk 2: Web Agent Lifecycle + Taskless Refactor
   │
   ├──── can overlap with ────┐
   ▼                          ▼
Chunk 3: Users/Permissions    Chunk 4: Env/Secrets UI
   │                          │
   └──────────┬───────────────┘
              ▼
Chunk 5: Production Hardening & Deployment
```

- **Chunk 1 → Chunk 2** is the tightest dependency: real-time updates make the agent creation flow usable (you can see your agent start). Without Chunk 1, Chunk 2 works but feels broken.
- **Chunks 3 and 4** can be developed in parallel with each other and partially overlap with Chunk 2.
- **Chunk 5** should come last, after features stabilize.

## Narrowest MVP Definition

If the goal is the smallest increment that makes the hosted web experience viable:

**Chunks 1 + 2** = Real-time status + agent creation from the browser.

This gives users the complete loop — create, monitor, interact, manage — without touching the CLI. Chunks 3-5 add multi-user support, security, and production readiness.

---

## Related Documents

- `hosted/frontend-milestones.md` — Detailed frontend milestone specs (M1-M16)
- `hosted/status.md` — Hosted architecture implementation status
- `hosted/auth/auth-milestones.md` — Authentication implementation phases
- `taskless-refactor.md` — Design for removing task requirement from `fabric start`
- `kubernetes/milestones.md` — K8s runtime hardening milestones
- `agent-services.md` — Sidecar process design (implemented)
- `template-import.md` — Template import design (Phase 1 implemented)
- `message-cmd.md` — Message command design (implemented)
