# Hub Template Admin

**Status:** Draft
**Created:** 2026-04-27
**Related:** [template-editor.md](./template-editor.md), [grove-level-templates.md](./grove-level-templates.md), [agnostic-template-design.md](./agnostic-template-design.md)

---

## 1. Overview

### Goal

Give hub administrators a centralized view of all templates stored on the hub — global and per-grove — with the ability to browse, edit, delete, and import templates at the hub level. This closes a gap where global templates can only be managed via CLI bootstrap or filesystem seeding, and grove-scoped templates are only visible from within their grove's settings page.

### Current State

- **Global templates** are seeded at hub startup from a `templates/` directory via the bootstrap process (`template_bootstrap.go`). There is no web UI to manage them after boot.
- **Grove-scoped templates** are visible and importable from the grove settings page under Resources > Templates. Each grove only sees its own templates.
- **Template detail page** (`/groves/{groveId}/templates/{templateId}`) supports file browsing and inline editing via the shared file browser and file editor components.
- **Template import** is supported via the grove settings page with two modes: URL import and workspace import. This calls `POST /api/v1/groves/{groveId}/import-templates`.
- **No admin-level template view exists.** An administrator who wants to audit all templates across the hub has no web interface to do so.

### Scope

This document covers:
- A new admin page listing all hub templates across all scopes
- Filtering and sorting by scope, harness, status, and grove
- Template deletion with confirmation and optional file cleanup
- Re-use of the existing template detail/editor page for viewing and editing
- Hub-level template import (global scope)

This document does NOT cover:
- Template creation from scratch (new template wizard) — can be added later
- Template versioning or changelog tracking
- Cross-hub template federation or marketplace
- Changes to the template data model or API (existing endpoints are sufficient)

---

## 2. User Experience

### 2.1 Admin Navigation

Add a "Templates" entry to the admin navigation sidebar:

```
Admin
├── Hub Settings
├── Server Config
├── Scheduler
├── Users
├── Groups
├── Templates        ← new
└── Maintenance
```

Icon: `file-earmark-code` (matches template icon used in grove settings).

Route: `/admin/templates` → component `fabric-page-admin-templates`.

### 2.2 Template List Page

```
┌─────────────────────────────────────────────────────────────────────┐
│ Templates                                          [Import Template]│
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│ Scope: [All ▾]  Harness: [All ▾]  Status: [Active ▾]  [🔍 Search] │
│                                                                     │
│ ┌─────────────────────────────────────────────────────────────────┐ │
│ │ Name            Harness   Scope     Grove        Status    ⋮   │ │
│ ├─────────────────────────────────────────────────────────────────┤ │
│ │ claude          claude    global    —            active    ⋮   │ │
│ │ gemini          gemini    global    —            active    🔒  │ │
│ │ custom-research claude    grove     my-project   active    ⋮   │ │
│ │ data-analyst    gemini    grove     analytics    active    ⋮   │ │
│ │ codex           generic   global    —            archived  ⋮   │ │
│ └─────────────────────────────────────────────────────────────────┘ │
│                                                                     │
│ Showing 1-5 of 12                              [← Prev] [Next →]   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

**Columns:**
| Column | Source | Notes |
|--------|--------|-------|
| Name | `displayName` falling back to `name` | Links to template detail page |
| Harness | `harness` | Rendered as a badge |
| Scope | `scope` | `global`, `grove`, `user` |
| Grove | Grove name (resolved from `scopeId`) | `—` for global/user scope |
| Status | `status` | Badge: green=active, yellow=pending, gray=archived |
| Actions | `⋮` dropdown | See §2.4 |

A lock icon (🔒) appears next to templates where `locked: true`.

**Filters:**
- **Scope**: All / Global / Grove / User
- **Harness**: All / claude / gemini / generic / codex / opencode (populated dynamically)
- **Status**: Active (default) / Pending / Archived / All
- **Search**: Free-text search on name and description

**Sorting:** Clickable column headers. Default sort: name ascending.

**Pagination:** Cursor-based, consistent with other admin pages. Default page size: 50.

### 2.3 Template Detail (Re-use)

Clicking a template name navigates to the existing template detail page. For admin context, the route supports access without a grove context:

- **Grove-scoped templates**: `/groves/{groveId}/templates/{templateId}` — existing route, works as-is.
- **Global/user-scoped templates**: `/admin/templates/{templateId}` — new route pointing to the same `fabric-page-template-detail` component, with admin breadcrumbs (`Admin > Templates > {name}`).

The template detail page already supports:
- File browser with directory tree
- Inline file editor with preview
- File upload
- Metadata display (name, description, harness, scope, content hash)

The lock/unlock toggle (see §2.4) is surfaced on the detail page for locked global templates so admins can unlock and edit files in place without a separate workflow.

### 2.4 Actions

The `⋮` dropdown menu on each row provides:

| Action | Behavior | Confirmation |
|--------|----------|--------------|
| View / Edit | Navigate to template detail page | None |
| Clone | Clone template (opens dialog for name/scope) | None |
| Lock / Unlock | Toggle `locked` flag via `PATCH` | None |
| Archive | Set status to `archived` via `PATCH` | Confirmation dialog |
| Delete | Delete template and storage files | Confirmation dialog (see §2.5) |

### 2.5 Delete Confirmation

Deleting a template is destructive and affects any groves or agents referencing it. The confirmation dialog should make the impact clear:

```
┌──────────────────────────────────────────────┐
│ Delete Template                              │
│                                              │
│ Are you sure you want to delete              │
│ "custom-research"?                           │
│                                              │
│ Scope: grove (my-project)                    │
│ Harness: claude                              │
│                                              │
│ ☐ Also delete stored template files          │
│                                              │
│ This action cannot be undone.                │
│                                              │
│                    [Cancel]  [Delete]         │
└──────────────────────────────────────────────┘
```

- **Locked templates**: The dialog warns that force deletion will be used. Consider requiring an extra confirmation step (e.g., type the template name).
- **File deletion checkbox**: Maps to the `deleteFiles=true` query parameter. Checked by default for admin deletions; unchecked retains storage files as a safety net.
- **Delete button**: Styled as `danger` variant. Disabled while request is in flight.

### 2.6 Hub-Level Template Import

The "Import Template" button in the page header opens a dialog that re-uses the same import UI pattern from grove settings, adapted for hub-level scope:

```
┌──────────────────────────────────────────────────────┐
│ Import Templates                                     │
│                                                      │
│ Target Scope:  ○ Global   ○ Grove ▾ [select grove]   │
│                                                      │
│ Source:  ○ URL   ○ Workspace Path                    │
│                                                      │
│ [https://github.com/org/templates.git          ]     │
│                                                      │
│                         [Cancel]  [Import]            │
└──────────────────────────────────────────────────────┘
```

**Scope selection:**
- **Global**: Imports templates at hub-wide global scope. Calls a new or adapted import endpoint (see §3.2).
- **Grove**: Imports into a specific grove. Uses the existing `POST /api/v1/groves/{groveId}/import-templates` endpoint. A grove selector dropdown appears when this option is chosen.

**Source modes:**
- **URL**: Git repository URL or HTTP archive URL. This is the only mode available for global-scope imports; workspace path import is only supported for grove-scoped imports (where an agent container provides the filesystem context).

---

## 3. Implementation

### 3.1 Frontend

#### New Files

| File | Purpose |
|------|---------|
| `web/src/components/pages/admin-templates.ts` | Admin template list page component |

#### Modified Files

| File | Change |
|------|--------|
| `web/src/client/main.ts` | Add `/admin/templates` and `/admin/templates/[id]` routes |
| `web/src/components/shared/nav.ts` | Add "Templates" to `ADMIN_SECTION` items |
| `web/src/components/pages/template-detail.ts` | Support admin breadcrumb context when accessed from `/admin/templates/{id}` |
| `web/scripts/copy-shoelace-icons.mjs` | Ensure `file-earmark-code` is in `USED_ICONS` (likely already present) |

#### Component Structure (`admin-templates.ts`)

Follow the pattern established by `admin-users.ts`:

```typescript
@customElement('fabric-page-admin-templates')
export class AdminTemplatesPage extends LitElement {
  @state() private loading = true;
  @state() private templates: Template[] = [];
  @state() private error: string | null = null;

  // Filters
  @state() private scopeFilter = 'all';
  @state() private harnessFilter = 'all';
  @state() private statusFilter = 'active';
  @state() private searchQuery = '';

  // Sorting
  @state() private sortField = 'name';
  @state() private sortDir: 'asc' | 'desc' = 'asc';

  // Pagination
  @state() private cursor: string | null = null;
  @state() private hasMore = false;
  @state() private totalCount = 0;

  // Action state
  @state() private actionInProgress = false;
  @state() private actionFeedback: ActionFeedback | null = null;
  @state() private deleteTarget: Template | null = null;
  @state() private showImportDialog = false;
}
```

#### Data Fetching

Use the existing `GET /api/v1/templates` endpoint with query parameters:

```
GET /api/v1/templates?scope={scope}&harness={harness}&status={status}&search={query}&limit=50&cursor={cursor}
```

For grove name resolution on grove-scoped templates, either:
- **Option A**: Batch-fetch grove names from `GET /api/v1/groves` for the set of `scopeId` values in the response. Cache grove name map in component state.
- **Option B**: Have the template list API include a `groveName` field in responses (requires a backend join — see §3.2).

Option A is preferred as it avoids backend changes and the grove count per page is small.

### 3.2 Backend

The existing template API already supports the needs of this admin page. Minimal backend changes are required:

#### Global Template Import

Currently `POST /api/v1/groves/{groveId}/import-templates` is grove-scoped. For hub-level global import, add:

```
POST /api/v1/templates/import
```

**Request body:**
```json
{
  "sourceUrl": "https://github.com/org/templates.git",
  "scope": "global"
}
```

**Alternative**: Re-use the existing grove import handler but allow an optional `scope=global` parameter that bypasses grove scoping. This is simpler but muddies the grove endpoint semantics.

**Recommendation**: Add the dedicated `/api/v1/templates/import` endpoint. It keeps the grove endpoint clean and makes admin-level import a first-class operation.

#### Handler Location

Add `handleTemplateImport` to `pkg/hub/template_handlers.go` (or a new `template_import_handlers.go` if the file is already large). The handler should:

1. Validate admin authorization.
2. Accept `sourceUrl` and `scope` (global or grove with `scopeId`).
3. Delegate to the existing `importTemplatesFromRemote` logic.
4. Return the list of imported template names and count.

#### Route Registration

In `server.go`:
```go
s.mux.HandleFunc("/api/v1/templates/import", s.handleTemplateImport)
```

This must be registered **before** the wildcard `/api/v1/templates/` route to avoid being swallowed by the ID handler.

### 3.3 Authorization

- The admin templates page and global import endpoint require admin role.
- Grove-scoped template deletion from the admin page should still respect grove-level capability checks, but admin role can override.
- Locked template deletion requires `force=true` — the UI handles this via the confirmation dialog.

---

## 4. API Summary

All endpoints already exist unless noted:

| Method | Endpoint | Purpose | New? |
|--------|----------|---------|------|
| GET | `/api/v1/templates` | List all templates (with filters) | No |
| GET | `/api/v1/templates/{id}` | Get template details | No |
| PATCH | `/api/v1/templates/{id}` | Update template (lock/unlock, archive) | No |
| DELETE | `/api/v1/templates/{id}?deleteFiles=true&force=true` | Delete template | No |
| POST | `/api/v1/templates/{id}/clone` | Clone template | No |
| POST | `/api/v1/templates/import` | Import templates at hub level | **Yes** |
| GET | `/api/v1/templates/{id}/files` | List template files | No |
| GET/PUT | `/api/v1/templates/{id}/files/{path}` | Read/write template files | No |

---

## 5. Phasing

### Phase 1: Read-Only Admin View
- New `admin-templates.ts` page with list, filters, sorting, pagination
- Nav entry and route registration
- Links to existing template detail page
- No new backend changes

### Phase 2: Admin Actions
- Delete with confirmation dialog
- Lock/unlock toggle
- Archive action
- Clone dialog

### Phase 3: Hub-Level Import
- New `POST /api/v1/templates/import` endpoint
- Import dialog in admin UI with scope selection
- Support for global template import from URL

### Phase 4: Polish
- Template usage indicators on the detail page (which groves reference a template, lazy-loaded)
- Bulk actions (select multiple → delete/archive)
