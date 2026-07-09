# Git Groves: Remote Git-Based Workspace Design

**Created:** 2026-02-19
**Status:** Accepted
**Supersedes:** `.design/kubernetes/scm.md` (largely out of date)
**Related:** `git-ws.md` (research), `secrets.md`, `hosted-architecture.md`, `sync-design.md`

---

## 1. Overview

This document defines how Fabric supports **git-anchored groves on the Hub** — groves defined by a remote git repository URL (and optionally a branch) that can provision agents with full git workspaces without requiring any local filesystem representation.

Today, groves are primarily created by running `fabric grove init` inside a local git checkout, with Hub registration as a secondary step. This design introduces a **Hub-first flow** where a user creates a grove directly on the Hub from a git URL, sets authentication credentials, and starts agents that clone the repository at initialization time.

### Goals

1. Allow users to create groves on the Hub with only a git URL and a secret — no local checkout required.
2. Agent containers clone the repository at startup using `fabrictool`, producing a fully functional git workspace.
3. Agents can commit and push changes back to the remote repository.
4. All lifecycle state changes are reported via Hub events for web UI visibility.

### Non-Goals

- GCS-based workspace sync (addressed separately in `sync-design.md`).
- Multi-provider git hosting abstraction (GitLab, Bitbucket, etc.) — this design targets GitHub as the initial implementation.
- GitHub App token exchange flow — deferred to a later phase.
- Automated branch protection enforcement — not possible with fine-grained PATs today.
- Token refresh mechanisms — out of scope; deferred to GitHub App phase.

---

## 2. Git URL as Grove Identity

### 2.1 Identity Model

Groves use a normalized git remote as their unique identity. The identity includes an **optional branch qualifier**, allowing multiple groves to exist for the same repository when they target different branches.

**Canonical form**: `github.com/org/repo` or `github.com/org/repo@branch`

When no branch is specified, the grove represents the repository's default branch (typically `main`). When a branch is specified, the grove is scoped to that branch — agents clone and branch from that base rather than `main`.

This means a team could have:
- `github.com/acme/widgets` — grove for mainline development
- `github.com/acme/widgets@release/v2` — grove for v2 release work

The normalization pipeline (`pkg/util/git.go:NormalizeGitRemote`) strips protocol, converts SSH notation, removes `.git` suffix, and lowercases. The branch qualifier (if present) is appended after normalization with an `@` separator.

### 2.2 Grove ID: Hash of Normalized URL

> **SUPERSEDED:** This section describes the original deterministic ID model. Grove IDs are now
> randomly generated UUIDs, and multiple groves may share the same git remote URL.
> See `.design/git-grove-duplicates.md` for the current design.

~~Rather than a system-generated UUID, the grove ID for git-anchored groves is a **deterministic hash of the normalized identity string**:~~

```
ID = SHA-256(normalized_git_identity)[:16]   // e.g., "a1b2c3d4e5f67890"
```

~~This means:~~
- ~~The same git URL always produces the same grove ID, regardless of which client creates it.~~
- ~~Idempotent creation — creating a grove for a URL that already exists is a no-op (or returns the existing grove).~~
- ~~No coordination needed between clients to agree on IDs.~~

The **full original URL** (as provided by the user) is also stored on the grove record for display and clone purposes, alongside the normalized form used for identity.

### 2.3 Accepted Inputs

The git URL used to anchor a grove should be **any valid remote git URL** — not limited to the `origin` remote of a local checkout. Acceptable inputs:

| Input | Normalized Form |
|-------|----------------|
| `https://github.com/acme/widgets.git` | `github.com/acme/widgets` |
| `git@github.com:acme/widgets.git` | `github.com/acme/widgets` |
| `ssh://git@github.com/acme/widgets` | `github.com/acme/widgets` |
| `https://github.com/acme/widgets` (no `.git`) | `github.com/acme/widgets` |
| `https://github.com/acme/widgets.git` + `--branch release/v2` | `github.com/acme/widgets@release/v2` |

SSH URLs are accepted as input but are always **converted to HTTPS** for clone operations in this phase. SSH key-based auth may be added as an alternative in a future iteration.

Repositories without a remote URL (purely local repos, or non-git directories) are treated as **regular workspaces with no git anchoring**. They cannot be created as Hub-first groves and continue to use the existing local worktree flow.

### 2.4 Slug Derivation

When creating a grove from a URL, the slug uses **org-repo hyphenated format** by default:

- **Default**: `acme-widgets` (extracted from `github.com/acme/widgets`)
- **With branch**: `acme-widgets-release-v2` (branch appended, slashes replaced with hyphens)
- **With `--slug` override**: user-specified slug

The `--slug` flag allows shorter names when collisions are not a concern.

---

## 3. Hub-First Grove Creation

### 3.1 New Command: `fabric hub grove create`

```
fabric hub grove create <git-url> [flags]
```

**Arguments:**
- `<git-url>` — Any valid remote git URL (HTTPS or SSH format)

**Flags:**
- `--slug <slug>` — Override the auto-derived slug
- `--name <name>` — Human-friendly display name (defaults to repo name)
- `--branch <branch>` — Base branch for the grove (defaults to detected default branch, or `main`)
- `--visibility <private|team|public>` — Grove visibility (defaults to `private`)

**Behavior:**

1. Validate the URL is a parseable git remote URL.
2. Normalize via `NormalizeGitRemote()`.
3. If `--branch` specified, append `@branch` to normalized form.
4. Compute deterministic ID from normalized identity.
5. Check for existing grove with same ID (return existing if found).
6. Derive slug: `org-repo` hyphenated (with branch suffix if applicable).
7. **Default branch detection**: If a `GITHUB_TOKEN` is available (e.g., already set at user scope), probe the remote with `git ls-remote --symref <url> HEAD` to detect the actual default branch. Otherwise, default to `main`.
8. Call `POST /api/v1/groves` with:
   ```json
   {
     "id": "<sha256-hash>",
     "name": "widgets",
     "slug": "acme-widgets",
     "gitRemote": "github.com/acme/widgets",
     "labels": {
       "fabric.dev/default-branch": "main",
       "fabric.dev/clone-url": "https://github.com/acme/widgets.git",
       "fabric.dev/source-url": "git@github.com:acme/widgets.git"
     }
   }
   ```
9. Print grove ID, slug, and next-steps guidance.

**Example Session:**

```
$ fabric hub grove create https://github.com/acme/widgets.git
Grove created:
  ID:     a1b2c3d4e5f67890
  Slug:   acme-widgets
  Remote: github.com/acme/widgets
  Branch: main

Next steps:
  1. Set git credentials:
     fabric hub secret set GITHUB_TOKEN --grove acme-widgets <your-pat>

  2. Start an agent:
     fabric start my-agent --grove acme-widgets "implement feature X"
```

### 3.2 Hub API Changes

The existing `POST /api/v1/groves` endpoint already supports creating groves without a broker. The request body needs no structural changes — the `gitRemote` field is already supported. Changes:

- Accept client-provided `id` (the deterministic hash) — the existing `POST /api/v1/groves/register` already supports this; ensure `POST /api/v1/groves` does as well.
- Return existing grove if the ID already exists (idempotent).

### 3.3 Local Grove Linking (Existing Flow, No Changes)

The existing `fabric hub link` command continues to work for users who have a local checkout. When a user runs `fabric hub link` in a git repository, the grove is registered/linked using the detected `origin` remote. This flow is unchanged.

---

## 4. Authentication: GitHub Fine-Grained PATs

### 4.1 Token Type

The initial implementation uses **GitHub Fine-Grained Personal Access Tokens (PATs)**. These tokens:

- Are scoped to specific repositories
- Support granular permissions (Contents: read/write, Metadata: read, etc.)
- Have configurable expiration
- Are bound to a single user account

### 4.2 Required Token Permissions

For agents to clone, commit, and push:

| Permission | Access | Purpose |
|------------|--------|---------|
| Contents | Read and write | Clone, commit, push |
| Metadata | Read | Repository info |
| Pull requests | Read and write | Open PRs (optional) |

### 4.3 Branch Protection Limitation

GitHub fine-grained PATs **do not support restricting push operations to specific branches**. A token with Contents write access can push to any branch the user has access to, including `main` and other protected branches (subject to GitHub branch protection rules configured on the repository).

**Mitigation strategies:**

1. **GitHub branch protection rules** — Configure the repository with branch protection on `main`/`master` requiring PR reviews. This is the primary defense and is external to Fabric.
2. **Fabric convention** — Agents are configured (via harness instructions and system prompt) to work on feature branches (`fabric/<agent-name>`) and never push directly to protected branches. This is an advisory control, not enforced at the credential level.
3. **Future: GitHub App tokens** — GitHub App installation tokens can be issued with more granular control. Deferred to a later phase.

### 4.4 Secret Storage

The PAT is stored as a secret using the existing secret management system:

```
fabric hub secret set GITHUB_TOKEN --grove acme-widgets <pat-value>
```

This creates a secret with:
- **Key**: `GITHUB_TOKEN`
- **Type**: `environment` (injected as env var)
- **Scope**: `grove`
- **ScopeID**: grove ID (resolved from slug `acme-widgets`)

The `GITHUB_TOKEN` can also be set at **user scope** (omit `--grove`), which makes it available to all of the user's groves. This is useful when a single user manages their own token across multiple repositories. Grove-scoped secrets override user-scoped secrets per the standard secret resolution hierarchy (`user < grove < broker`).

The existing `fabric hub secret set` command already supports both scoping modes. The secret value is encrypted at rest (Hub DB encryption or GCP Secret Manager, per `secrets.md`).

---

## 5. Agent Workspace Provisioning

### 5.1 Flow Overview

When `fabric start my-agent --grove acme-widgets` is executed against a Hub-first git grove (no local checkout), the workspace must be provisioned by cloning the repository inside the agent container.

```
User CLI                  Hub                    Runtime Broker             Container
   |                       |                          |                        |
   |-- start agent ------->|                          |                        |
   |                       |-- resolve grove -------->|                        |
   |                       |   (git remote, secrets)  |                        |
   |                       |                          |                        |
   |                       |-- CreateAgent ---------->|                        |
   |                       |   (with gitCloneConfig)  |                        |
   |                       |                          |-- create container --->|
   |                       |                          |   (GITHUB_TOKEN env,   |
   |                       |                          |    GIT_CLONE_URL,      |
   |                       |                          |    GIT_BRANCH)         |
   |                       |                          |                        |
   |                       |                          |                  fabrictool init
   |                       |<-- status: CLONING ------|<-- event --------------|
   |                       |                          |                   ├─ git clone
   |                       |                          |                   ├─ git checkout
   |                       |                          |                   ├─ configure git
   |                       |<-- status: STARTING -----|<-- event --------------|
   |                       |                          |                   └─ start harness
   |                       |<-- status: RUNNING ------|<-- event --------------|
   |<-- agent started -----|                          |                        |
```

Note: `CLONING` status is reported **before** the clone operation begins, so the UI shows progress immediately.

### 5.2 CreateAgent Payload Extension

The `CreateAgent` command payload (sent from Hub to Runtime Broker via WebSocket control channel) is extended with git clone configuration:

```json
{
  "type": "command",
  "command": "create_agent",
  "payload": {
    "agentId": "...",
    "name": "my-agent",
    "config": {
      "template": "coder",
      "image": "claude-sandbox:latest",
      "harness": "claude",
      "gitClone": {
        "url": "https://github.com/acme/widgets.git",
        "branch": "main",
        "depth": 1
      },
      "env": ["GITHUB_TOKEN=<resolved-secret-value>"],
      ...
    }
  }
}
```

The `gitClone` object is a new field on the agent config. When present, it signals that the workspace should be populated via `git clone` rather than volume mounting or file sync.

### 5.3 Runtime Broker Handling

When the Runtime Broker receives a `CreateAgent` command with `gitClone` config:

1. **No worktree creation** — skip the `util.CreateWorktree()` call.
2. **No workspace mount** — the workspace will be created inside the container.
3. **Environment injection** — pass `GITHUB_TOKEN`, `FABRIC_GIT_CLONE_URL`, `FABRIC_GIT_BRANCH`, and `FABRIC_GIT_DEPTH` as environment variables to the container.
4. **Container start** — start the container with `fabrictool init` as the entrypoint (unchanged).

### 5.4 fabrictool: Git Clone Phase

`fabrictool init` gains a new **git clone phase** that runs before the harness process starts. This slots into the existing initialization sequence:

```
fabrictool init
  1. StartReaper()
  2. setupHostUser()
  3. Start Telemetry
  4. Initialize Lifecycle Hooks
  5. RunPreStart()
  6. *** NEW: gitCloneWorkspace() ***     <-- git clone phase
  7. Start Sidecar Services
  8. Create Supervisor
  9. Start Child Process (harness)
  ...
```

#### `gitCloneWorkspace()` Implementation

Triggered when the environment variable `FABRIC_GIT_CLONE_URL` is set and `/workspace` is empty (or does not exist).

```
gitCloneWorkspace():
  1. Report status: CLONING (via Hub status API, before clone begins)
     Include metadata: { repository, branch }

  2. Construct authenticated URL:
     https://oauth2:${GITHUB_TOKEN}@github.com/acme/widgets.git

  3. Execute:
     git clone --depth=${FABRIC_GIT_DEPTH:-1} \
       --branch=${FABRIC_GIT_BRANCH:-main} \
       <authenticated-url> /workspace

  4. Configure git identity:
     git -C /workspace config user.name "Fabric Agent (${FABRIC_AGENT_NAME})"
     git -C /workspace config user.email "agent@fabric.dev"

  5. Configure credential helper for subsequent operations:
     git -C /workspace config credential.helper \
       '!f() { echo "password=${GITHUB_TOKEN}"; echo "username=oauth2"; }; f'

  6. Create and checkout agent feature branch:
     git -C /workspace checkout -b fabric/${FABRIC_AGENT_NAME}

  7. Report status: STARTING (clone complete, proceeding to harness startup)
```

**Error handling**: If `git clone` fails (bad URL, invalid token, network error), fabrictool reports status `ERROR` with a descriptive message and exits with a non-zero code. The Hub event includes the error detail for UI display.

**Security**: The authenticated URL is constructed in-process and never written to disk or logs. The `GITHUB_TOKEN` environment variable is available for the credential helper to use during subsequent push operations, but is not embedded in the git config.

### 5.5 Environment Variables

| Variable | Source | Description |
|----------|--------|-------------|
| `GITHUB_TOKEN` | Resolved secret (user or grove scope) | GitHub PAT for authentication |
| `FABRIC_GIT_CLONE_URL` | Grove gitRemote (HTTPS form) | Repository URL (without credentials) |
| `FABRIC_GIT_BRANCH` | Agent config / grove default branch | Branch to clone and checkout |
| `FABRIC_GIT_DEPTH` | Agent config (optional) | Clone depth (default: 1) |

`GITHUB_TOKEN` is resolved through the standard secret resolution pipeline — nothing special. It follows the same `user < grove < broker` scope hierarchy as any other environment-type secret.

### 5.6 Clone Depth

The default clone depth is **1** (shallow clone) for speed. This limits `git log` and `git blame` to the current commit, but agents that need full history can run `git fetch --unshallow` inside the container.

This can be noted as a tip in agent template instructions — e.g., "If you need full git history for blame or log analysis, run `git fetch --unshallow` first."

Override via `--depth` on `fabric start` or as a grove label (`fabric.dev/clone-depth`).

### 5.7 Branch Strategy

When an agent starts on a git grove:

1. The repository is cloned at the grove's base branch (stored in `fabric.dev/default-branch` label).
2. A new feature branch is created: `fabric/<agent-name>`.
3. The agent works on this feature branch exclusively.
4. Commits and pushes go to `fabric/<agent-name>` on the remote.

This matches the existing local worktree behavior where each agent gets its own branch.

---

## 6. Agent Lifecycle with Git Groves

### 6.1 Starting an Agent

```
fabric start my-agent --grove acme-widgets "implement login page"
```

**Resolution flow:**

1. CLI resolves `acme-widgets` to a grove ID via Hub API (`GET /api/v1/groves?slug=acme-widgets`).
2. CLI calls `POST /api/v1/agents` with the grove ID.
3. Hub resolves grove's git remote and secrets (including `GITHUB_TOKEN` via standard scope resolution).
4. Hub selects an available Runtime Broker for the grove.
5. Hub sends `CreateAgent` command to Broker with `gitClone` config and resolved secrets.
6. Broker starts container with appropriate environment.
7. `fabrictool` reports `CLONING`, clones repo, creates branch, reports `STARTING`, starts harness.
8. Status transitions visible in web UI: `PENDING` → `CLONING` → `STARTING` → `RUNNING`.

### 6.2 Status Events

The `fabrictool` status reporting system already supports arbitrary status values. The git clone phase introduces a new status:

| Status | Meaning |
|--------|---------|
| `PENDING` | Agent record created, container not yet started |
| `CLONING` | **New** — `fabrictool` is about to / currently cloning the git repository |
| `STARTING` | Clone complete, harness initializing |
| `RUNNING` | Harness active, agent accepting work |
| `WAITING_FOR_INPUT` | Agent needs human interaction |
| `COMPLETED` | Task finished |
| `ERROR` | Fatal error (including clone failure) |

These status transitions are reported via:
- `fabrictool` → Hub API (`POST /api/v1/agents/{id}/status`)
- Hub → WebSocket event bus → Web UI

Note: The realtime web event system is also evolving (see `web-realtime.md`), but changes there do not impact the primary scope of this design. The status reporting mechanism is stable.

### 6.3 Pushing Changes

Once the agent has made changes, it can commit and push using standard git operations. The credential helper configured during clone provides authentication for push:

```bash
git add .
git commit -m "implement login page"
git push -u origin fabric/my-agent
```

The harness instructions should guide the agent to:
1. Never push to `main` or other protected branches directly.
2. Always push to the `fabric/<agent-name>` branch.
3. Open a PR via the GitHub API (using `GITHUB_TOKEN`) if requested.

### 6.4 Detach / Reattach

Git groves work identically to local groves for attach/detach operations. The container persists with its cloned workspace between sessions:

```
fabric attach my-agent --grove acme-widgets
fabric detach
```

### 6.5 Agent Deletion and Unpushed Change Protection

When an agent is deleted, the container is removed. The cloned workspace is ephemeral (container-local storage) and is discarded. Any unpushed commits are lost. The remote branch (`fabric/my-agent`) remains on GitHub.

**Future feature: `--protect-git-changes`**

A future enhancement should add an agent-level option (set at creation time) that protects against accidental loss of unpushed work:

```
fabric start my-agent --grove acme-widgets --protect-git-changes "task"
```

When this flag is set on an agent:
- `fabric rm my-agent` checks git status inside the container before deletion.
- If there are uncommitted changes or commits not pushed to the remote, the command returns an error with a descriptive message (e.g., "agent has 3 unpushed commits; use --force to delete anyway").
- `fabric rm my-agent --force` overrides the protection and deletes regardless.

The mechanism for checking git status would be a `fabrictool` command invoked via `docker exec` / `kubectl exec` before container removal. This is deferred to a later implementation phase.

### 6.6 Workspace Persistence Across Restarts

- **Stop/Start** (`fabric stop` / `fabric start`): The container filesystem persists (Docker) or the pod is kept (K8s with restart policy). The cloned workspace is reused — no re-clone needed.
- **Delete/Recreate** (`fabric rm` / `fabric start`): The workspace is lost. A fresh shallow clone runs on the new container. This is fast for most repositories and acceptable.

---

## 7. New CLI Command: `fabric hub grove create`

### 7.1 Command Registration

Add to `cmd/hub.go` under the existing `hubGrovesCmd` group:

```go
var hubGroveCreateCmd = &cobra.Command{
    Use:   "create <git-url>",
    Short: "Create a grove on the Hub from a git repository URL",
    Long: `Creates a new grove on the Hub anchored to a remote git repository.
The grove can be used to start agents without a local checkout of the repository.

The grove ID is deterministically derived from the normalized git URL, so
creating a grove for the same URL is idempotent.`,
    Args: cobra.ExactArgs(1),
    RunE: runHubGroveCreate,
}
```

### 7.2 Implementation Sketch

```go
func runHubGroveCreate(cmd *cobra.Command, args []string) error {
    gitURL := args[0]

    // Validate URL format
    if !util.IsGitURL(gitURL) {
        return fmt.Errorf("invalid git URL: %s", gitURL)
    }

    normalized := util.NormalizeGitRemote(gitURL)
    if branchFlag != "" {
        normalized = normalized + "@" + branchFlag
    }

    // Deterministic ID from normalized identity
    groveID := util.HashGroveID(normalized)

    // Derive org-repo slug
    org, repo := util.ExtractOrgRepo(gitURL)
    slug := slugOverride
    if slug == "" {
        slug = util.Slugify(org + "-" + repo)
        if branchFlag != "" {
            slug += "-" + util.Slugify(branchFlag)
        }
    }

    displayName := nameOverride
    if displayName == "" {
        displayName = repo
    }

    // Detect default branch if token available
    defaultBranch := branchFlag
    if defaultBranch == "" {
        defaultBranch = detectDefaultBranch(gitURL) // probes remote if possible
        if defaultBranch == "" {
            defaultBranch = "main"
        }
    }

    client := getHubClient()

    // Create grove (idempotent — returns existing if ID matches)
    grove, err := client.Groves().Create(ctx, &hubclient.CreateGroveRequest{
        ID:        groveID,
        Name:      displayName,
        Slug:      slug,
        GitRemote: normalized,
        Labels: map[string]string{
            "fabric.dev/default-branch": defaultBranch,
            "fabric.dev/clone-url":      util.ToHTTPSCloneURL(gitURL),
            "fabric.dev/source-url":     gitURL,
        },
    })
    if err != nil {
        return err
    }

    fmt.Printf("Grove created:\n")
    fmt.Printf("  ID:     %s\n", grove.ID)
    fmt.Printf("  Slug:   %s\n", grove.Slug)
    fmt.Printf("  Remote: %s\n", grove.GitRemote)
    fmt.Printf("  Branch: %s\n", defaultBranch)
    fmt.Printf("\nNext steps:\n")
    fmt.Printf("  1. Set git credentials:\n")
    fmt.Printf("     fabric hub secret set GITHUB_TOKEN --grove %s <your-pat>\n\n", grove.Slug)
    fmt.Printf("  2. Start an agent:\n")
    fmt.Printf("     fabric start my-agent --grove %s \"your task\"\n", grove.Slug)

    return nil
}
```

### 7.3 URL Utilities

New utility functions in `pkg/util/git.go`:

**`IsGitURL(s string) bool`** — Validates that a string is a plausible git remote URL. Accepts HTTPS URLs, SSH format (`git@host:path`), and `ssh://` URLs. Rejects local paths, empty strings, and bare hostnames.

**`ToHTTPSCloneURL(gitURL string) string`** — Converts any valid git URL to HTTPS clone form:
- `git@github.com:org/repo.git` → `https://github.com/org/repo.git`
- `ssh://git@github.com/org/repo` → `https://github.com/org/repo.git`
- `https://github.com/org/repo.git` → passthrough

**`ExtractOrgRepo(gitURL string) (org, repo string)`** — Extracts organization and repository name components from a git URL. Used for slug derivation.

**`HashGroveID(normalized string) string`** — Computes the deterministic grove ID as a hex-encoded truncated SHA-256 hash of the normalized identity string.

### 7.4 Default Branch Detection

When creating a grove, if a `GITHUB_TOKEN` is available (at user scope or provided via environment), the command should attempt to detect the remote's default branch:

```go
func detectDefaultBranch(gitURL string) string {
    // Try: git ls-remote --symref <url> HEAD
    // Parse output for "ref: refs/heads/<branch>\tHEAD"
    // Return branch name, or "" if probe fails
}
```

This requires the URL to be accessible (public repo, or token available for private repos). If the probe fails, the command falls back to `main` and logs a note.

---

## 8. Config and Data Model Changes

### 8.1 AgentConfig Extension

Add `GitClone` field to the agent configuration model:

```go
// GitCloneConfig specifies how to clone a git repository into the workspace.
type GitCloneConfig struct {
    URL    string `json:"url"`              // HTTPS clone URL (without credentials)
    Branch string `json:"branch,omitempty"` // Branch to clone (default: main)
    Depth  int    `json:"depth,omitempty"`  // Clone depth (default: 1, 0 = full)
}
```

This appears in:
- `AgentAppliedConfig` (stored in Hub DB with agent record)
- `CreateAgent` command payload (Hub → Broker)
- `runtime.RunConfig` (Broker → container)

### 8.2 Grove Labels

The grove's configuration is stored as labels on the grove record:

| Label | Value | Purpose |
|-------|-------|---------|
| `fabric.dev/default-branch` | `main` | Base branch for new agents |
| `fabric.dev/clone-url` | `https://github.com/org/repo.git` | HTTPS-form URL for cloning |
| `fabric.dev/source-url` | `git@github.com:org/repo.git` | Original URL as provided by user |

Labels are used rather than new schema fields to avoid schema migration for what are effectively configuration preferences.

### 8.3 Hub Secret Resolution for Git Clone

When the Hub prepares the `CreateAgent` payload for a git grove, `GITHUB_TOKEN` is resolved through the standard secret resolution pipeline — the same `user < grove < broker` scope hierarchy used for all secrets. No special handling is needed beyond ensuring the token is present in the resolved environment.

---

## 9. Runtime Broker Changes

### 9.1 Provisioning Path for Git Groves

When the Runtime Broker receives a `CreateAgent` command with `gitClone` configuration:

1. **Skip worktree creation** — `ProvisionAgent()` detects `gitClone` config and skips the `util.CreateWorktree()` call.
2. **Skip workspace mounting** — no host-side workspace directory is bind-mounted.
3. **Inject git environment** — add `FABRIC_GIT_CLONE_URL`, `FABRIC_GIT_BRANCH`, `FABRIC_GIT_DEPTH` to container environment.
4. **Inject resolved secrets** — `GITHUB_TOKEN` is injected as a container environment variable (same as any environment-type secret).
5. **Ensure writable workspace** — the container's `/workspace` directory must be writable. For Docker, this is automatic (container filesystem). For Kubernetes, an `emptyDir` volume at `/workspace`.

### 9.2 Docker Runtime

No init container is needed for Docker. The `fabrictool init` entrypoint handles the clone as part of its startup sequence. The Docker run command looks like:

```bash
docker run -t -d \
  -e GITHUB_TOKEN=ghp_xxx \
  -e FABRIC_GIT_CLONE_URL=https://github.com/acme/widgets.git \
  -e FABRIC_GIT_BRANCH=main \
  -e FABRIC_GIT_DEPTH=1 \
  -e FABRIC_AGENT_NAME=my-agent \
  ... \
  claude-sandbox:latest \
  fabrictool init -- tmux new-session -s fabric claude
```

### 9.3 Kubernetes Runtime

For Kubernetes, the clone is performed by `fabrictool init` in the main container (consistent with Docker — no separate init container):

```yaml
spec:
  containers:
  - name: agent
    image: claude-sandbox:latest
    command: ["fabrictool", "init", "--"]
    args: ["tmux", "new-session", "-s", "fabric", "claude"]
    env:
    - name: GITHUB_TOKEN
      valueFrom:
        secretKeyRef:
          name: fabric-git-creds-acme-widgets
          key: token
    - name: FABRIC_GIT_CLONE_URL
      value: "https://github.com/acme/widgets.git"
    - name: FABRIC_GIT_BRANCH
      value: "main"
    volumeMounts:
    - name: workspace
      mountPath: /workspace
  volumes:
  - name: workspace
    emptyDir: {}
```

---

## 10. GCS Workspace Sync (Side Note)

For groves that use GCS-based workspace synchronization (non-git workspaces, or groves where the user prefers file sync over git clone), the existing `sync-design.md` flow applies. Key differences:

| Aspect | Git Clone | GCS Sync |
|--------|-----------|----------|
| Source of truth | Git remote | GCS bucket |
| History | Full git history (within depth) | No history |
| Push-back | `git push` to remote | `fabric sync from` to download |
| Branch awareness | Yes | No |
| Credential type | Git PAT | GCS service account / signed URLs |
| Offline capability | Full (after clone) | Full (after sync) |

These two strategies are **mutually exclusive per grove** — a grove either has a git remote (and uses clone) or does not (and uses GCS sync). The presence of `gitRemote` on the grove record determines which path is used.

A more detailed design for GCS workspace sync improvements should be captured in a separate document.

---

## 11. Web UI Visibility

### 11.1 Status Reporting

All state transitions during agent startup with git clone are reported through the existing Hub event system:

1. **Agent created** → `PENDING` (Hub creates agent record)
2. **Container starting** → broker reports container status
3. **Clone starting** → `CLONING` (fabrictool reports via `POST /api/v1/agents/{id}/status`, before clone begins)
4. **Clone complete** → `STARTING` (fabrictool reports)
5. **Harness ready** → `RUNNING` (fabrictool reports)

The `CLONING` status includes metadata about the clone operation:

```json
{
  "status": "CLONING",
  "metadata": {
    "repository": "github.com/acme/widgets",
    "branch": "main"
  }
}
```

### 11.2 Error Reporting

If the clone fails, the error is reported with actionable context:

```json
{
  "status": "ERROR",
  "error": "git clone failed: authentication failed",
  "metadata": {
    "repository": "github.com/acme/widgets",
    "phase": "git-clone",
    "suggestion": "Check that GITHUB_TOKEN is set and has Contents read access"
  }
}
```

### 11.3 Event Stream

The existing WebSocket event endpoints (`WS /api/v1/agents/{id}/events`, `WS /api/v1/groves/{id}/events`) carry these status updates in real-time. The web frontend can display clone progress and errors without polling.

Note: The realtime web event system is also under active development (see `web-realtime.md`), but changes there do not impact the core status reporting mechanism used here.

---

## 12. End-to-End Example

### 12.1 Setup (One-Time)

```bash
# Create a grove from a GitHub repository
$ fabric hub grove create https://github.com/acme/widgets.git
Grove created:
  ID:     a1b2c3d4e5f67890
  Slug:   acme-widgets
  Remote: github.com/acme/widgets
  Branch: main

# Store a GitHub PAT as a grove secret
$ fabric hub secret set GITHUB_TOKEN --grove acme-widgets ghp_xxxxxxxxxxxxxxxxxxxx
Secret 'GITHUB_TOKEN' set for grove 'acme-widgets'
```

### 12.2 Start an Agent

```bash
# Start an agent on the grove (no local checkout needed)
$ fabric start my-coder --grove acme-widgets "implement user authentication"
Agent 'my-coder' starting on grove 'acme-widgets'...
  Cloning github.com/acme/widgets (branch: main)...
  Branch: fabric/my-coder
  Status: RUNNING

# Attach to interact
$ fabric attach my-coder --grove acme-widgets
```

### 12.3 Agent Operations (Inside Container)

```bash
# Agent is in /workspace with a full git clone
$ pwd
/workspace

$ git branch
* fabric/my-coder
  main

$ git remote -v
origin  https://github.com/acme/widgets.git (fetch)
origin  https://github.com/acme/widgets.git (push)

# After making changes...
$ git add .
$ git commit -m "implement user auth module"
$ git push -u origin fabric/my-coder
```

### 12.4 Multiple Agents

```bash
# Start additional agents on the same grove
$ fabric start reviewer --grove acme-widgets "review the auth PR"
$ fabric start tester --grove acme-widgets "write tests for the auth module"

# Each gets its own branch
# reviewer → fabric/reviewer
# tester   → fabric/tester
```

### 12.5 Branch-Scoped Grove

```bash
# Create a grove for a specific branch
$ fabric hub grove create https://github.com/acme/widgets.git --branch release/v2
Grove created:
  ID:     f9e8d7c6b5a49382
  Slug:   acme-widgets-release-v2
  Remote: github.com/acme/widgets@release/v2
  Branch: release/v2

# Agents on this grove branch from release/v2 instead of main
$ fabric start hotfix --grove acme-widgets-release-v2 "fix CVE-2026-1234"
# Branch: fabric/hotfix (based on release/v2)
```

---

## 13. Implementation Plan

### Phase 1: Hub-First Grove Creation

1. Add `fabric hub grove create <git-url>` command to `cmd/hub.go`.
2. Add `IsGitURL()`, `ToHTTPSCloneURL()`, `ExtractOrgRepo()`, and `HashGroveID()` utilities to `pkg/util/git.go`.
3. Implement default branch detection via `git ls-remote --symref`.
4. Update `NormalizeGitRemote()` to support `@branch` suffix in identity strings.
5. Store clone URL, source URL, and default branch as grove labels.
6. Verify existing `fabric hub secret set` works for this flow (no changes expected).

### Phase 2: fabrictool Git Clone

1. Add `gitCloneWorkspace()` function to `cmd/fabrictool/commands/init.go`.
2. Report `CLONING` status before clone begins, with repository and branch metadata.
3. Construct authenticated clone URL in-process (never written to disk/logs).
4. Configure git identity and credential helper post-clone.
5. Create and checkout agent feature branch (`fabric/<agent-name>`).
6. Handle errors with actionable messages and `ERROR` status reporting.

### Phase 3: Runtime Broker Integration

1. Add `GitCloneConfig` to agent config models (`pkg/store/models.go`).
2. Update `ProvisionAgent()` to detect git clone mode and skip worktree creation.
3. Update `buildCommonRunArgs()` to inject git clone environment variables.
4. Update Docker and Kubernetes runtime paths.

### Phase 4: Hub Orchestration

1. Update Hub's agent creation handler to resolve grove git remote and inject `gitClone` config into `CreateAgent` payload.
2. Ensure `GITHUB_TOKEN` is included in resolved agent environment via standard secret pipeline.
3. Pass `FABRIC_GIT_CLONE_URL` and `FABRIC_GIT_BRANCH` through to broker.

### Phase 5: Testing and Polish

1. Integration tests for the full flow (create grove → set secret → start agent → verify clone).
2. Error case testing (bad URL, expired token, private repo without token, network failure).
3. Web UI display of `CLONING` status.
4. Documentation updates.

---

## 14. Decisions Record

The following questions were raised during design review and resolved:

| # | Question | Decision |
|---|----------|----------|
| 1 | **Slug format**: repo-only vs org-repo? | Use `org-repo` hyphenated as default convention. Allow `--slug` override. |
| 2 | **Grove ID**: UUID vs deterministic? | Deterministic hash of normalized URL. Enables idempotent creation. |
| 3 | **Clone depth**: shallow vs full? | Default depth 1. Agents can `git fetch --unshallow` if needed (note in template instructions). |
| 4 | **Token refresh**: implement now? | Out of scope. Users set long-lived PATs. Deferred to GitHub App phase. |
| 5 | **SSH URL input**: support SSH auth? | Convert to HTTPS for clone. SSH key auth deferred. |
| 6 | **Forks**: track upstream? | Forks are independent groves. Upstream relationships handled at PR level. |
| 7 | **Default branch detection**: probe remote? | Yes, via `git ls-remote --symref` when token is available. Fall back to `main`. |
| 8 | **Workspace persistence**: re-clone on restart? | Reuse for stop/start. Re-clone on delete/recreate (shallow clone is fast). |
| 9 | **K8s clone mechanism**: init container vs fabrictool? | Use `fabrictool init` for consistency across runtimes. |
| 10 | **Branch in grove identity**: single repo = single grove? | Support `@branch` qualifier so same repo can have multiple groves for different branches. |
| 11 | **Secret scope for GITHUB_TOKEN**: grove only? | Grove is natural default, but user scope also supported (one token across groves). Standard resolution hierarchy applies. |
| 12 | **Unpushed change protection**: block deletion? | Future feature via `--protect-git-changes` flag and `--force` override on `fabric rm`. |
