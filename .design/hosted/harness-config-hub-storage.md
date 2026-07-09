# Harness-Config Hub Storage Design

## Status
**Proposed**

## 1. Overview

This document specifies the design for storing harness-configs in the Fabric Hub, enabling Hub-only agent creation workflows where the Runtime Broker does not have local harness-config directories. The design intentionally mirrors the existing hosted template machinery to maximize code reuse.

### 1.1. Current Harness-Config System (Solo Mode)

In solo mode, harness-configs are:
- **Directories on disk** at `~/.fabric/harness-configs/<name>/` (global) or `.fabric/harness-configs/<name>/` (grove-level), or within templates at `<template>/harness-configs/<name>/`
- **Composed of two parts**: a `config.yaml` (metadata: harness type, image, user, model, args, env, volumes) and a `home/` directory (harness-specific files like `.claude.json`, `settings.json`, `.bashrc`)
- **Resolved by precedence**: template-level > grove-level > global
- **Used at agent creation**: the `home/` directory forms the base layer of an agent's home, overlaid with template files

### 1.2. Problem Statement

In the hosted architecture, a Runtime Broker may not have local harness-config directories. When the Hub dispatches a `create_agent` command, the Broker needs access to the harness-config's `config.yaml` metadata and `home/` files. Currently, harness-configs are resolved locally on the machine running `fabric create`, which doesn't work when:
1. Agent creation is initiated through the Hub web UI or API (no local CLI context)
2. The Runtime Broker is a remote/ephemeral node without pre-seeded harness-configs
3. Templates reference a named harness-config that only exists on the user's machine

### 1.3. Key Insight: Structural Similarity to Templates

A harness-config and a template are structurally almost identical from a storage perspective:

| Aspect | Template | Harness-Config |
|--------|----------|----------------|
| On-disk shape | Directory with config + files | Directory with `config.yaml` + `home/` |
| Metadata file | `fabric-agent.yaml` | `config.yaml` |
| File payload | `home/`, optional workspace files | `home/` files |
| Scoping | global, grove, user | global, grove (+ template-level locally) |
| Content tracking | SHA-256 content hash | Same mechanism applicable |
| Upload flow | Two-phase (metadata + signed URLs + finalize) | Same flow applicable |
| Storage layout | `gs://bucket/templates/{scope}/{slug}/` | `gs://bucket/harness-configs/{scope}/{slug}/` |

This means the template upload/download/finalize pipeline, storage helpers, content hashing, caching, and CLI sync/push/pull patterns can all be reused with minimal adaptation.

---

## 2. What Can Be Reused Directly

### 2.1. Storage Layer (`pkg/storage/`)

**Fully reusable with a new path helper.** The existing `StorageClient` interface and GCS implementation handle signed URL generation, file existence checks, and bucket operations generically. Only a new path function is needed:

```go
// New function, parallel to TemplateStoragePath
func HarnessConfigStoragePath(scope, scopeID, slug string) string
func HarnessConfigStorageURI(bucket, scope, scopeID, slug string) string
```

### 2.2. Two-Phase Upload Flow (`pkg/hub/template_handlers.go`)

The create-upload-finalize pattern is directly applicable:
1. **Create**: Register harness-config metadata, get signed upload URLs
2. **Upload**: Client uploads `config.yaml` and `home/` files to signed URLs
3. **Finalize**: Verify manifest, compute content hash, mark active

The handler logic for generating signed URLs, verifying uploaded files, and computing content hashes is generic enough to factor into shared helpers or to duplicate with minimal changes.

### 2.3. Content Hashing

The manifest-based SHA-256 content hash computation (sort files by path, concatenate hashes, compute final hash) is identical. The same `TemplateManifest` / `TemplateFile` types can be reused or aliased.

### 2.4. Hub Client Upload/Download (`pkg/hubclient/`)

The `UploadFile` and `DownloadFile` methods on the hub client are generic HTTP operations. The `RequestUploadURLs`, `Finalize`, and `RequestDownloadURLs` patterns map directly. A new `HarnessConfigService` interface can mirror `TemplateService`.

### 2.5. CLI Sync/Push/Pull Patterns (`cmd/templates.go`)

The `syncTemplateToHub` function's logic — collect local files, compute hashes, create-or-update, delta upload, finalize — maps directly to syncing a local harness-config directory to the Hub. Similarly, `pullTemplateFromHubMatch` for downloading.

### 2.6. Runtime Broker Template Cache (`pkg/templatecache/`)

The cache structure (content-hash-based directories, LRU eviction, index file) works for harness-configs without modification. The cache could be extended to store both resource types, or a parallel `harness-config-cache` could share the same implementation.

---

## 3. What Needs to Be Added or Altered

### 3.1. Data Model (`pkg/store/models.go`)

A new `HarnessConfig` struct is needed. It is deliberately simpler than `Template` — harness-configs don't have inheritance, `fabric-agent.yaml`, or workspace files:

```go
type HarnessConfig struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Slug        string `json:"slug"`
    DisplayName string `json:"displayName,omitempty"`
    Description string `json:"description,omitempty"`

    // Core identity
    Harness string              `json:"harness"`     // claude, gemini, codex, opencode, generic
    Config  *HarnessConfigData  `json:"config,omitempty"` // Parsed config.yaml fields

    // Content tracking
    ContentHash string `json:"contentHash,omitempty"`

    // Scope & storage (identical pattern to Template)
    Scope         string `json:"scope"`
    ScopeID       string `json:"scopeId,omitempty"`
    StorageURI    string `json:"storageUri,omitempty"`
    StorageBucket string `json:"storageBucket,omitempty"`
    StoragePath   string `json:"storagePath,omitempty"`

    // File manifest
    Files []TemplateFile `json:"files,omitempty"` // Reuse TemplateFile type

    // Protection & status
    Locked bool   `json:"locked,omitempty"`
    Status string `json:"status"` // pending, active, archived

    // Ownership
    OwnerID    string `json:"ownerId,omitempty"`
    CreatedBy  string `json:"createdBy,omitempty"`
    UpdatedBy  string `json:"updatedBy,omitempty"`
    Visibility string `json:"visibility,omitempty"` // private, grove, public

    // Timestamps
    Created time.Time `json:"created"`
    Updated time.Time `json:"updated"`
}
```

`HarnessConfigData` mirrors the existing `HarnessConfigEntry` from settings, representing the parsed `config.yaml`:

```go
type HarnessConfigData struct {
    Harness          string            `json:"harness"`
    Image            string            `json:"image,omitempty"`
    User             string            `json:"user,omitempty"`
    Model            string            `json:"model,omitempty"`
    Args             []string          `json:"args,omitempty"`
    Env              map[string]string `json:"env,omitempty"`
    AuthSelectedType string            `json:"authSelectedType,omitempty"`
}
```

**Note:** `TemplateFile` is reused directly for the file manifest — the struct is generic (path, size, hash, mode).

### 3.2. Store Interface (`pkg/store/store.go`)

A new `HarnessConfigStore` interface, parallel to `TemplateStore`:

```go
type HarnessConfigStore interface {
    CreateHarnessConfig(ctx context.Context, hc *HarnessConfig) error
    GetHarnessConfig(ctx context.Context, id string) (*HarnessConfig, error)
    GetHarnessConfigBySlug(ctx context.Context, slug, scope, scopeID string) (*HarnessConfig, error)
    UpdateHarnessConfig(ctx context.Context, hc *HarnessConfig) error
    DeleteHarnessConfig(ctx context.Context, id string) error
    ListHarnessConfigs(ctx context.Context, filter HarnessConfigFilter, opts ListOptions) (*ListResult[HarnessConfig], error)
}

type HarnessConfigFilter struct {
    Name    string
    Scope   string
    ScopeID string
    Harness string
    OwnerID string
    Status  string
    Search  string
}
```

The SQLite/database implementation follows the same pattern as template CRUD. The `ListOptions` and `ListResult[T]` generics are already in place.

### 3.3. Store Implementation

A new table `harness_configs` with the same column structure as `templates`, minus template-specific fields (`baseTemplate`, `image` top-level). The schema migration is straightforward.

### 3.4. Hub API Handlers (`pkg/hub/`)

A new `harness_config_handlers.go` file, closely mirroring `template_handlers.go`. The handlers share the same patterns:
- List/Get/Create/Update/Delete CRUD
- Upload URL generation
- Finalize with manifest verification
- Download URL generation

**Refactoring opportunity:** Before duplicating the template handler code, consider extracting common helpers for:
- Signed URL generation for a set of files
- Manifest verification and content hash computation
- File existence checks against storage

These helpers would be called by both template and harness-config handlers.

### 3.5. Hub Client (`pkg/hubclient/`)

A new `HarnessConfigService` interface and implementation in `harness_configs.go`, parallel to `templates.go`. Methods: `List`, `Get`, `Create`, `Update`, `Delete`, `RequestUploadURLs`, `Finalize`, `RequestDownloadURLs`, `UploadFile` (reuse existing), `DownloadFile` (reuse existing).

### 3.6. Template → Harness-Config Reference

Templates today carry a `Harness` string field indicating harness *type*. To support hub-stored harness-configs, templates should also carry an optional reference to a specific named harness-config:

```go
// In store.TemplateConfig (addition)
HarnessConfig string `json:"harnessConfig,omitempty"` // Named harness-config reference
```

This allows a template to declare: "use the `claude-custom` harness-config from the Hub" rather than relying on local resolution.

### 3.7. Agent Creation Flow

When the Hub dispatches `create_agent`, it must resolve the harness-config and include its storage URI in the dispatch payload so the Runtime Broker can fetch it. The existing template resolution pattern applies:

1. Determine harness-config name (from template, explicit override, or default)
2. Resolve Hub harness-config record (scope hierarchy: grove > user > global)
3. Include `storageUri` and `contentHash` in the dispatch payload
4. Broker fetches/caches harness-config files alongside the template

### 3.8. Runtime Broker Hydration

The Broker's `Hydrator` (in `pkg/templatecache/hydrator.go`) needs to compose from two cloud-stored resources:
1. Fetch harness-config files → base layer for agent home
2. Fetch template files → overlay on top

This matches the local composition model (`harness-config home/` + `template home/`) but both sources come from cloud storage.

---

## 4. Hub API Inventory

### 4.1. New Endpoints

All endpoints under `/api/v1/harness-configs`. Route registration in `pkg/hub/handlers.go`.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/harness-configs` | List harness-configs (with scope/harness/status filters) |
| `POST` | `/api/v1/harness-configs` | Create harness-config (returns upload URLs) |
| `GET` | `/api/v1/harness-configs/{id}` | Get harness-config by ID |
| `PUT` | `/api/v1/harness-configs/{id}` | Update harness-config (upsert) |
| `PATCH` | `/api/v1/harness-configs/{id}` | Partial update (rename, metadata) |
| `DELETE` | `/api/v1/harness-configs/{id}` | Delete (soft-delete / archive) |
| `POST` | `/api/v1/harness-configs/{id}/upload` | Request signed upload URLs |
| `POST` | `/api/v1/harness-configs/{id}/finalize` | Finalize upload with manifest |
| `GET` | `/api/v1/harness-configs/{id}/download` | Get signed download URLs |

**Not needed (template-specific):**
- Clone endpoint (harness-configs are simpler; copy semantics less common)
- Inheritance/base-template (harness-configs are flat, no inheritance chain)

These can be added later if demand arises.

### 4.2. Query Parameters (List)

Same pattern as templates:
- `scope` — global, grove, user
- `scopeId` — grove or user ID
- `harness` — filter by harness type
- `status` — active, archived
- `search` — name/description search
- `limit`, `cursor` — pagination

### 4.3. Request/Response Types

Mirror the template types with adjusted naming:
- `CreateHarnessConfigRequest` / `CreateHarnessConfigResponse`
- `UpdateHarnessConfigRequest`
- `FinalizeHarnessConfigRequest`
- `DownloadHarnessConfigResponse`
- `ListHarnessConfigsResponse`

These are structurally identical to their template counterparts.

---

## 5. CLI Command Inventory

### 5.1. Existing Commands (modifications)

| Command | Change |
|---------|--------|
| `fabric harness-config list` | Add `--hub` flag to list Hub-stored harness-configs alongside local ones |
| `fabric harness-config reset` | No change (local-only operation) |
| `fabric create --harness-config` | When Hub is enabled, resolve harness-config from Hub if not found locally |

### 5.2. New Commands

| Command | Description | Hub Only |
|---------|-------------|----------|
| `fabric harness-config sync <name>` | Create or update harness-config in Hub from local directory (upsert, delta upload) | Yes |
| `fabric harness-config push <name>` | Alias for `sync` | Yes |
| `fabric harness-config pull <name>` | Download harness-config from Hub to local directory | Yes |
| `fabric harness-config show <name>` | Show harness-config details (local + Hub) | No |
| `fabric harness-config delete <name>` | Delete harness-config (local, Hub, or both) | No |

### 5.3. Implementation Approach

The CLI commands follow the same patterns as `cmd/templates.go`:
- `sync` reuses the file-collection, hash-computation, create-or-update, delta-upload, finalize flow
- `pull` reuses the download-URLs, fetch-files, write-to-disk flow
- `list` merges local and Hub results with `--local` / `--hub` filtering

These can be added to `cmd/harness_config.go` by extending the existing command tree.

---

## 6. Storage Layout

### 6.1. Bucket Structure

Parallel to templates, under a `/harness-configs` prefix:

```
gs://fabric-hub-{env}/
├── templates/
│   └── ...                          (existing)
└── harness-configs/
    ├── global/
    │   ├── claude/
    │   │   ├── manifest.json
    │   │   ├── config.yaml
    │   │   └── home/
    │   │       ├── .bashrc
    │   │       ├── .claude.json
    │   │       └── .claude/
    │   │           └── settings.json
    │   └── gemini/
    │       └── ...
    ├── groves/
    │   └── {groveId}/
    │       └── {harnessConfigSlug}/
    │           └── ...
    └── users/
        └── {userId}/
            └── {harnessConfigSlug}/
                └── ...
```

### 6.2. Storage Path Functions

```go
func HarnessConfigStoragePath(scope, scopeID, slug string) string {
    switch scope {
    case "global":
        return "harness-configs/global/" + slug
    case "grove":
        return "harness-configs/groves/" + scopeID + "/" + slug
    case "user":
        return "harness-configs/users/" + scopeID + "/" + slug
    default:
        return "harness-configs/global/" + slug
    }
}
```

---

## 7. Agent Creation Integration

### 7.1. Dispatch Payload Changes

The `create_agent` dispatch payload includes both template and harness-config resolution:

```json
{
  "command": "create_agent",
  "payload": {
    "agentId": "agent-123",
    "template": {
      "id": "template-abc",
      "contentHash": "sha256:...",
      "storageUri": "gs://bucket/templates/..."
    },
    "harnessConfig": {
      "id": "hc-xyz",
      "name": "claude",
      "contentHash": "sha256:...",
      "storageUri": "gs://bucket/harness-configs/...",
      "config": {
        "harness": "claude",
        "image": "fabric-claude:latest",
        "user": "fabric",
        "model": "sonnet-4"
      }
    }
  }
}
```

### 7.2. Runtime Broker Composition

The Broker composes the agent home in the same layered order as solo mode:
1. Fetch harness-config `home/` files from cloud storage → base layer
2. Fetch template `home/` files from cloud storage → overlay
3. Inject agent instructions / system prompt → harness-specific injection
4. Apply runtime overrides (env, model, etc.)

### 7.3. Harness-Config Resolution Order (Hub)

When resolving a harness-config name in the Hub:
1. Grove-scoped harness-config matching the name
2. User-scoped harness-config matching the name
3. Global harness-config matching the name

This matches the local resolution order (grove > global) with the addition of user scope.

---

## 8. Refactoring Opportunities

### 8.1. Shared Upload/Finalize Helpers

Extract from `template_handlers.go` into a shared package or helper file:

```go
// pkg/hub/storage_helpers.go (or similar)
func generateUploadURLs(stor storage.StorageClient, basePath string, files []FileUploadRequest) ([]UploadURLInfo, error)
func verifyAndComputeContentHash(stor storage.StorageClient, basePath string, manifest *Manifest) (string, error)
func generateDownloadURLs(stor storage.StorageClient, basePath string, files []ManifestFile) ([]DownloadURLInfo, error)
```

Both template and harness-config handlers would call these shared functions, eliminating duplicated signed-URL and hashing logic.

### 8.2. Shared CLI Sync Logic

The file-collection and delta-upload logic in `syncTemplateToHub` can be extracted into a reusable function that accepts a directory path and a target (template or harness-config service).

### 8.3. Generic Manifest Types

`TemplateManifest` and `TemplateFile` are not template-specific. Consider renaming or aliasing:
- `TemplateFile` → `StorageFile` (or type alias `StorageFile = TemplateFile`)
- `TemplateManifest` → `StorageManifest`

This is a cosmetic improvement and not strictly required — using `TemplateFile` for harness-config manifests works fine mechanically.

---

## 9. Scope and Non-Goals

### 9.1. In Scope
- Hub CRUD API for harness-configs
- Cloud storage of harness-config files (config.yaml + home/)
- CLI sync/push/pull commands
- Runtime Broker fetching harness-configs from Hub storage
- Integration with existing agent creation flow

### 9.2. Out of Scope (Future)
- Harness-config versioning (same deferral as templates)
- Harness-config inheritance/composition (harness-configs are flat)
- Web UI for harness-config management (follow-on to API)
- Migration tooling for bulk-uploading existing local harness-configs

---

## 10. Implementation Sequence

1. **Store layer**: Add `HarnessConfig` model, `HarnessConfigStore` interface, SQLite implementation, migration
2. **Storage helpers**: Add `HarnessConfigStoragePath` / `HarnessConfigStorageURI` to `pkg/storage/`
3. **Hub API handlers**: Add `harness_config_handlers.go` with CRUD + upload/finalize/download
4. **Hub client**: Add `HarnessConfigService` to `pkg/hubclient/`
5. **CLI commands**: Extend `cmd/harness_config.go` with sync/push/pull/show/delete
6. **Agent dispatch**: Include harness-config in `create_agent` payload; update Broker hydration
7. **Refactor (optional)**: Extract shared helpers from template handlers if duplication is excessive

---

## 11. References

- **Hosted Template Design**: `hosted-templates.md` (primary reference for reusable patterns)
- **Hub API Specification**: `hub-api.md`
- **Local Harness-Config System**: `pkg/config/harness_config.go`
- **Settings Harness-Config Entry**: `pkg/config/settings_v1.go` (`HarnessConfigEntry`)
- **Template Handlers**: `pkg/hub/template_handlers.go`
- **Template CLI Commands**: `cmd/templates.go`
- **Harness-Config CLI Commands**: `cmd/harness_config.go`
