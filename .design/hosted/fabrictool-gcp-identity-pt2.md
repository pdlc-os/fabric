# GCP Identity Web UI & Agent Assignment Integration

## Status: Complete

## Context

This document covers the remaining work to complete GCP identity support in Scion, building on the foundation established in [sciontool-gcp-identity.md](./sciontool-gcp-identity.md). Phases 1 (Foundation) and 2 (Hardening) are complete, delivering:

- Hub API endpoints for GCP service account CRUD and verification
- Hub token brokering endpoints (`/agent/gcp-token`, `/agent/gcp-identity-token`)
- CLI commands (`scion grove service-accounts add/list/remove/verify`)
- Hub client library (`GCPServiceAccountService` interface)
- Metadata server sidecar in sciontool with block/assign/passthrough modes
- iptables interception for Docker runtime
- Rate limiting, metrics, and audit logging

Two items remain from the Phase 2 roadmap:

1. ~~**Backend gap**: GCP identity assignment during agent creation is not wired through the Hub API~~ **COMPLETE**
2. ~~**Web UI**: Service account management and agent identity assignment via the web interface~~ **COMPLETE**

## 1. Backend Gap: Agent Creation GCP Identity Assignment ✅ Complete

### Problem

The `CreateAgentRequest` handler does not accept or process a `gcp_identity` field. While the storage layer (`Agent.AppliedConfig.GCPIdentity`) and runtime broker (`CreateAgentConfig.GCPIdentity`) both support GCP identity configuration, there is no path to set it through the agent creation API.

This means:
- The web UI cannot assign GCP identity during agent creation
- The CLI cannot assign GCP identity during agent creation
- The only way to get GCP identity on an agent would be manual database manipulation

### Required Changes

#### 1.1 CreateAgentRequest Extension

Add `GCPIdentity` to the agent creation request type:

```go
type CreateAgentRequest struct {
    // ... existing fields ...
    GCPIdentity *GCPIdentityAssignment `json:"gcp_identity,omitempty"`
}

type GCPIdentityAssignment struct {
    MetadataMode     string `json:"metadata_mode"`                // "block", "passthrough", "assign"
    ServiceAccountID string `json:"service_account_id,omitempty"` // Required when mode is "assign"
}
```

#### 1.2 Validation Logic

In the `createAgent` handler, validate the GCP identity assignment:

1. `metadata_mode` must be one of `block`, `passthrough`, `assign`
2. If mode is `assign`:
   - `service_account_id` is required
   - The referenced SA must exist and belong to the same grove
   - The SA must be verified (`Verified == true`)
3. If mode is `block` or `passthrough`, `service_account_id` must be empty

#### 1.3 Authorization

The design doc (Phase 1, deferred item) specifies: "permission check on service account resources." This needs resolution:

| Option | Description | Trade-off |
|--------|-------------|-----------|
| **Grove admin only** | Only grove admins can assign SAs to agents | Simple, consistent with SA registration permissions |
| **Explicit assign permission** | New permission on GCPServiceAccount resources | More granular, but adds complexity |
| **Any grove member** | Anyone who can create agents can assign any verified SA in the grove | Least restrictive, may be acceptable for initial release |

**Recommendation**: Start with grove admin only. This matches SA registration permissions and avoids introducing a new permission model. Revisit if users need delegation.

#### 1.4 Applied Config Population

In `buildAppliedConfig()`, resolve the GCP identity assignment:

```go
if req.GCPIdentity != nil && req.GCPIdentity.MetadataMode == "assign" {
    sa, err := s.store.GetGCPServiceAccount(ctx, req.GCPIdentity.ServiceAccountID)
    // validate SA belongs to grove, is verified
    appliedConfig.GCPIdentity = &store.GCPIdentityConfig{
        MetadataMode:        req.GCPIdentity.MetadataMode,
        ServiceAccountID:    sa.ID,
        ServiceAccountEmail: sa.Email,
        ProjectID:           sa.ProjectID,
    }
}
```

#### 1.5 Agent Token Scope Injection

When GCP identity mode is `assign`, the agent's JWT must include `grove:gcp:token:<sa-id>`. Verify this scope is added during agent provisioning/token generation.

#### 1.6 Post-Creation Updates (Deferred)

Changing an agent's GCP identity after creation is out of scope for this phase. It would require:
- A PATCH endpoint for agent GCP identity
- Restarting the metadata sidecar or agent container
- Revoking/reissuing the agent JWT with updated scopes

This can be added later if needed. For now, GCP identity is set at creation time.

## 2. Web UI Design

### 2.1 Technology & Patterns

The Scion web UI uses:

- **Lit** (Web Components) with TypeScript
- **Shoelace** component library (sl-button, sl-dialog, sl-select, etc.)
- **Client-side router** with lazy-loaded page components
- **SSE-based real-time state** via `StateManager`
- **Capabilities-based authorization** (`_capabilities` on API responses)
- **Shared reusable components** for CRUD operations (e.g., `scion-secret-list`, `scion-env-var-list`)

Key patterns to follow:

| Pattern | Reference Implementation |
|---------|-------------------------|
| Scoped CRUD list with dialog | `web/src/components/shared/secret-list.ts` |
| Grove detail settings tab | `web/src/components/pages/grove-detail.ts` |
| Agent creation form | `web/src/components/pages/agent-create.ts` |
| Status badges | `web/src/components/shared/status-badge.ts` |
| Capabilities checks | `can()` / `canAny()` from `web/src/shared/types.ts` |

### 2.2 New TypeScript Types

Add to `web/src/shared/types.ts`:

```typescript
export interface GCPServiceAccount {
  id: string;
  scope: string;
  scopeId: string;
  email: string;
  projectId: string;
  displayName: string;
  defaultScopes: string[];
  verified: boolean;
  verifiedAt: string | null;
  createdBy: string;
  createdAt: string;
  _capabilities?: Capabilities;
}

export interface GCPIdentityConfig {
  metadataMode: 'block' | 'passthrough' | 'assign';
  serviceAccountId?: string;
  serviceAccountEmail?: string;
  projectId?: string;
}

export interface GCPIdentityAssignment {
  metadataMode: 'block' | 'passthrough' | 'assign';
  serviceAccountId?: string;
}
```

### 2.3 Service Account Management Component

**File**: `web/src/components/shared/gcp-service-account-list.ts`

**Pattern**: Follows `scion-secret-list` — a reusable component that can be embedded in the grove detail Settings tab as a compact section, or used standalone.

#### Properties

```typescript
@customElement('scion-gcp-service-account-list')
export class ScionGCPServiceAccountList extends LitElement {
  @property() groveId = '';
  @property({ type: Boolean }) compact = false;

  @state() private accounts: GCPServiceAccount[] = [];
  @state() private loading = true;
  @state() private error: string | null = null;

  // Dialog state for Add SA
  @state() private dialogOpen = false;
  @state() private dialogEmail = '';
  @state() private dialogProjectId = '';
  @state() private dialogDisplayName = '';
  @state() private dialogLoading = false;
  @state() private dialogError: string | null = null;
}
```

#### Table Layout

| Column | Content |
|--------|---------|
| Email | SA email address (primary identifier) |
| Project | GCP project ID |
| Name | Display name (optional) |
| Status | Verified badge with timestamp, or "Unverified" warning |
| Actions | Verify (if unverified), Delete |

#### Operations

| Action | UI Element | API Call |
|--------|-----------|----------|
| Add | "Add Service Account" button -> sl-dialog with form | `POST /api/v1/groves/{groveId}/gcp-service-accounts` |
| Verify | Inline icon button (checkmark) on unverified rows | `POST /api/v1/groves/{groveId}/gcp-service-accounts/{id}/verify` |
| Delete | Inline icon button (trash) with confirmation | `DELETE /api/v1/groves/{groveId}/gcp-service-accounts/{id}` |

#### Add Dialog Fields

| Field | Component | Required | Notes |
|-------|-----------|----------|-------|
| Service Account Email | `sl-input` | Yes | e.g., `agent-worker@project.iam.gserviceaccount.com` |
| GCP Project ID | `sl-input` | Yes | e.g., `my-project-123` |
| Display Name | `sl-input` | No | Human-friendly label |

On successful creation, the Hub automatically attempts verification. The response includes the updated `verified` status.

#### Verification UX

- Unverified SAs show a warning badge: `sl-badge variant="warning"` with text "Unverified"
- Verified SAs show a success badge with the verification timestamp
- The "Verify" action button is only visible on unverified SAs
- On verify failure, show an inline error explaining the likely cause (Hub SA lacks `serviceAccountTokenCreator` role on the target SA)

### 2.4 Grove Detail Integration

**File**: `web/src/components/pages/grove-detail.ts`

Add the GCP service account list as a new section in the Settings tab, alongside the existing secrets and environment variables sections:

```html
<sl-tab-panel name="settings">
  <scion-env-var-list .groveId=${this.groveId} compact></scion-env-var-list>
  <scion-secret-list scope="grove" .scopeId=${this.groveId} compact></scion-secret-list>
  <!-- New section -->
  <scion-gcp-service-account-list .groveId=${this.groveId} compact></scion-gcp-service-account-list>
</sl-tab-panel>
```

No new routes or navigation items required. SA management is a grove-level concern and belongs in the grove detail page.

### 2.5 Agent Creation Form: GCP Identity Section

**File**: `web/src/components/pages/agent-create.ts`

Add a new form section for GCP identity configuration. This section is conditional — it only appears when the selected grove has registered (and verified) service accounts, or when the user wants to explicitly set block mode.

#### New State

```typescript
@state() private gcpMetadataMode: 'block' | 'passthrough' | 'assign' = 'block';
@state() private gcpServiceAccountId = '';
@state() private gcpServiceAccounts: GCPServiceAccount[] = [];
```

#### Data Loading

When the grove selection changes, fetch the grove's GCP service accounts:

```typescript
private async onGroveChanged(groveId: string) {
  this.groveId = groveId;
  // ... existing logic ...

  // Load GCP service accounts for the selected grove
  const saRes = await apiFetch(`/api/v1/groves/${groveId}/gcp-service-accounts`);
  if (saRes.ok) {
    this.gcpServiceAccounts = await saRes.json();
  }
}
```

#### Form Section Layout

```
GCP Identity
  Metadata Mode:  [block ▼]        ← sl-select with block/assign/passthrough options
                                      Help text below explaining selected mode

  Service Account: [Select SA ▼]   ← sl-select, only visible when mode = "assign"
                                      Populated with verified SAs from grove
                                      Shows email + display name per option
```

#### Mode Help Text

| Mode | Description shown to user |
|------|--------------------------|
| `block` | Prevents the agent from accessing any GCP identity. Token requests to the metadata server are denied. This is the default. |
| `assign` | Assigns a registered GCP service account to the agent. Standard GCP client libraries will automatically authenticate using this identity. |
| `passthrough` | No metadata interception. The agent inherits whatever GCP identity is available on the host (if any). |

#### Validation

- If mode is `assign` and no SA is selected, show validation error
- If mode is `assign`, only verified SAs appear in the dropdown (filter out unverified)
- If the grove has no verified SAs and mode is `assign`, show a message directing the user to register SAs in grove settings

#### Submission

Include GCP identity in the `CreateAgentRequest` body:

```typescript
const body: Record<string, unknown> = {
  name: this.name,
  groveId: this.groveId,
  // ... existing fields ...
};

if (this.gcpMetadataMode !== 'block') {
  body.gcp_identity = {
    metadata_mode: this.gcpMetadataMode,
    ...(this.gcpMetadataMode === 'assign' && {
      service_account_id: this.gcpServiceAccountId,
    }),
  };
}
```

Note: `block` is the default, so it only needs to be sent explicitly if the backend default differs.

### 2.6 Agent Detail: GCP Identity Display

**File**: `web/src/components/pages/agent-detail.ts`

In the agent detail Overview or Settings tab, display the agent's GCP identity configuration as a read-only info section:

| Field | Display |
|-------|---------|
| Metadata Mode | Badge: `block` (neutral), `assign` (primary), `passthrough` (warning) |
| Service Account | Email address (only if mode is `assign`) |
| GCP Project | Project ID (only if mode is `assign`) |

This is informational only — no editing (per the deferred post-creation update decision in Section 1.6).

### 2.7 Real-Time Updates (SSE)

GCP service account changes (create, delete, verify) are low-frequency administrative operations. Two options:

| Approach | Complexity | Benefit |
|----------|-----------|---------|
| **Reload on action** | None — component calls `loadAccounts()` after each mutation | Sufficient for admin-frequency operations |
| **SSE events** | Hub publishes on `grove.{id}.gcp-sa.*` subjects | Live updates if multiple admins manage SAs concurrently |

**Recommendation**: Start with reload-on-action. SSE integration can be added later if multi-admin concurrent editing becomes a real use case.

### 2.8 Capabilities Integration

The Hub API should include `_capabilities` on GCP service account list/detail responses, consistent with other resources:

```json
{
  "id": "uuid",
  "email": "agent@project.iam.gserviceaccount.com",
  "_capabilities": {
    "delete": true,
    "verify": true
  }
}
```

The list endpoint should also return a top-level `_capabilities` for collection-level actions:

```json
{
  "items": [...],
  "_capabilities": {
    "create": true
  }
}
```

UI elements (Add button, Delete button, Verify button) should be gated on these capabilities using the existing `can()` helper.

## 3. Files to Create/Modify

| File | Change |
|------|--------|
| `pkg/hub/handlers_agent.go` | Add `GCPIdentity` to `CreateAgentRequest`, validation, SA resolution |
| `pkg/api/types.go` | Add `GCPIdentityAssignment` request type (if not using hub-internal types) |
| `web/src/shared/types.ts` | Add `GCPServiceAccount`, `GCPIdentityConfig`, `GCPIdentityAssignment` interfaces |
| `web/src/components/shared/gcp-service-account-list.ts` | New: Reusable SA management component |
| `web/src/components/pages/grove-detail.ts` | Embed `scion-gcp-service-account-list` in Settings tab |
| `web/src/components/pages/agent-create.ts` | Add GCP identity section (metadata mode + SA select) |
| `web/src/components/pages/agent-detail.ts` | Display GCP identity info in overview/settings |
| `pkg/hub/handlers_gcp_identity.go` | Add `_capabilities` to SA API responses |

## 4. Implementation Order

1. **Backend**: Wire GCP identity assignment into `CreateAgentRequest` and `buildAppliedConfig()` (prerequisite for everything else)
2. **Types**: Add TypeScript interfaces to `web/src/shared/types.ts`
3. **SA List Component**: Build `scion-gcp-service-account-list` following `scion-secret-list` pattern
4. **Grove Detail**: Embed SA list in Settings tab
5. **Agent Create Form**: Add GCP identity section with mode selector and SA dropdown
6. **Agent Detail**: Add read-only GCP identity display
7. **Capabilities**: Add `_capabilities` to SA API responses and gate UI elements

Steps 3-4 and 5-6 can be developed in parallel.
