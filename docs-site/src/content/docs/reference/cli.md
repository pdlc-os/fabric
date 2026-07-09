---
title: Fabric CLI Reference
---

The Fabric CLI is the primary interface for managing agents, projects, and server components.

## Global Flags

These flags are available on all commands:

- `-g, --project <string>`: Project identifier: path, slug (with Hub), or git URL (with Hub).
- `--global`: Use the global project (equivalent to `--project global`).
- `-p, --profile <name>`: Configuration profile to use.
- `--format <string>`: Output format (`json` or `plain`).
- `--hub <url>`: Hub API endpoint URL (overrides `FABRIC_HUB_ENDPOINT`).
- `--no-hub`: Disable Hub integration for this invocation (local-only mode).
- `-y, --yes`: Skip confirmation prompts.
- `--non-interactive`: Full non-interactive mode (implies `--yes`, errors on ambiguous prompts).
- `--debug`: Enable verbose debug output.

## Agent Lifecycle

### `fabric start` (or `run`)

Starts a new agent or resumes an existing one. Starting a **suspended** agent
implicitly resumes its harness session (continuing the prior conversation);
starting a **stopped** or **error** agent runs a fresh session. See
[`fabric suspend`](#fabric-suspend) and [`fabric resume`](#fabric-resume).

**Usage:** `fabric start <agent-name> [task] [flags]`

- **Arguments:**
    - `<agent-name>`: Unique name for the agent instance.
    - `[task]`: (Optional) The initial instruction/task for the agent.
- **Flags:**
    - `-b, --branch <string>`: Target branch for the agent workspace.
    - `-t, --type <string>`: Template to use (default "gemini").
    - `-i, --image <string>`: Override container image.
    - `-a, --attach`: Attach to the agent immediately after starting.
    - `--no-auth`: Disable authentication propagation.
    - `-d, --detached`: Run in detached mode (default true).
    - `--config <path>`: Path to inline agent config file (YAML/JSON) for Just-In-Time (JIT) overrides, or `-` for stdin.
    - `--harness-config <string>`: Named harness configuration to use.
    - `--harness-auth <string>`: Override auth method for the harness (e.g., `api-key`, `vertex-ai`, `auth-file`).
    - `--broker <string>`: Preferred runtime broker ID or name for execution.
    - `--notify`: Get notified via the browser or system when the spawned agent reaches a terminal state.

### `fabric stop`

Stops a running agent. This is a graceful shutdown (`SIGTERM`); the agent's
phase becomes `stopped` and the next `start` runs a fresh session.

**Usage:** `fabric stop <agent-name>`

### `fabric suspend`

Suspends a running agent, preserving its harness session for a later resume.
Unlike `stop`, suspending sets the agent's phase to `suspended`, and the next
`start` (or `resume`) **continues** the prior conversation instead of starting
fresh.

Only running agents can be suspended, and the agent's harness must support
session resume (Claude Code and Gemini CLI do; the generic harness does not —
use `stop` instead). See [Agent Lifecycle](/fabric/local/agent-lifecycle/).

**Usage:** `fabric suspend <agent-name> [flags]`

- **Flags:**
    - `-a, --all`: Suspend all running agents in the current project. Agents
      whose harness does not support resume are skipped.

### `fabric resume`

Resumes an existing agent. For a **suspended** agent, the harness session is
continued (Claude Code receives `--continue`, Gemini CLI `--resume`, etc.). For
a **stopped** agent, there is no session to continue, so a fresh session is
started.

A plain `fabric resume <agent-name>` (no task) simply **continues** the prior
session — the agent's original creation task is *not* re-injected. If you pass an
explicit prompt, it is sent as a **new message** on top of the continued
session.

**Usage:** `fabric resume <agent-name> [task] [flags]`

- **Flags:**
    - `-a, --attach`: Attach to the agent immediately.

### `fabric attach`

Connects to the interactive session of a running agent.

**Usage:** `fabric attach <agent-name>`

- **Key Bindings:**
    - `Ctrl+P, Ctrl+Q`: Detach from the session without stopping the agent.

### `fabric message` (or `msg`)

Sends a message to a running agent's harness by enqueuing it into its input stream (requires Tmux).

**Usage:** `fabric message [agent] <message> [flags]`

- **Arguments:**
    - `[agent]`: The name of the agent (optional if `--broadcast` is used).
    - `<message>`: The text to send to the agent.
- **Flags:**
    - `-i, --interrupt`: Interrupt the harness before sending the message.
    - `-b, --broadcast`: Send the message to all running agents in the current project.
    - `-a, --all`: Send the message to all running agents across all projects.
    - `--notify`: Get notified when the target agent(s) respond or reach a terminal state after receiving the message.

### `fabric messages` (aliases: `msgs`, `inbox`)

Manages bidirectional communication and persistent messages sent by agents to humans.

**Usage:** `fabric messages [command] [flags]`

- **Commands:**
    - `list` (default): View unread messages.
    - `read <message-id>`: Mark a specific message as read.
    - `read-all`: Mark all messages as read.
- **Flags:**
    - `--agent <string>`: Filter messages by a specific agent.
    - `--all`: Show all messages, including those already marked as read.

### `fabric logs`

Displays the logs of an agent.

**Usage:** `fabric logs <agent-name> [flags]`

- **Flags:**
    - `-f, --follow`: Stream logs.

### `fabric list` (or `ps`)

Lists all agents and their status.

**Usage:** `fabric list [flags]`

- **Flags:**
    - `-a, --all`: Show all agents (including stopped ones).
    - `-r, --running`: Filter for active (running) agents.

### `fabric delete` (or `rm`)

Deletes an agent, removing its container, home directory, and worktree.

**Usage:** `fabric delete <agent-name> [flags]`

- **Flags:**
    - `-b, --preserve-branch`: Preserve the git branch associated with the worktree (default: deleted).
    - `--stopped`: Delete all agents with stopped containers.

### `fabric sync`

Synchronizes the agent workspace between the host and the container.

**Usage:** `fabric sync [to|from] <agent-name> [flags]`

- **Flags:**
    - `--dry-run`: Preview changes without syncing.
    - `--exclude <glob>`: Exclude files matching the pattern.

## Configuration & Workspace

### `fabric project`

Manages the Fabric workspace (Project).

- `fabric project init`: Initialize a new project. By default, creates a `.fabric` directory in the current directory or the root of the current git repository.
    - Flags:
        - `--global`: Initialize the global project in the home directory.
        - `--machine`: Perform full machine-level setup (seeds harness-configs, templates, settings).
        - `--image-registry <string>`: Configure the container image registry path (e.g., `ghcr.io/myorg`).
    - **Note:** If you are in a git repository, add `.fabric/agents` to your `.gitignore` to avoid issues with nested git worktrees: `echo ".fabric/agents" >> .gitignore`
    - **Hub Integration:** If a Hub endpoint is configured, `init` will prompt to register the new project with the Hub.
- `fabric project list` (alias `ls`): List all projects known to Fabric on this machine, including their type, agent count, status, and workspace path.
- `fabric project prune`: Detect and remove project configurations whose workspace directories no longer exist. This stops any running containers associated with orphaned projects before cleaning up.
- `fabric project reconnect <new-workspace-path>`: Reconnect a moved workspace to its externalized project configuration. This fixes projects that show as "orphaned" after being relocated.

### `fabric clean`

Removes the fabric project configuration from the current project or global location.

**Usage:** `fabric clean [flags]`

- **Flags:**
    - `--skip-hub-check`: Skip Hub connectivity check before removing.

### `fabric config`

View and modify configuration settings.

- `list`: List all effective settings.
- `get <key>`: Get a specific configuration value.
- `set <key> <value>`: Set a configuration value.
- `validate`: Validate settings files against the schema.
- `migrate`: Migrate configuration to the latest versioned format.
- `dir`: Print the path to the active configuration directory.

### `fabric cd-config`

Open a new shell in the active Fabric configuration directory.

**Usage:** `fabric cd-config`

### `fabric cd-project`

Open a new shell in the active project's workspace directory.

**Usage:** `fabric cd-project`

### `fabric cdw`

Change directory to the workspace of an agent.

**Usage:** `fabric cdw <agent-name>`

### `fabric shared-dir`

Manages shared directories for agents within a project.

- `list`: List shared directories in the current project.
- `create <name>`: Create a new shared directory.
- `info <name>`: View details about a specific shared directory.
- `remove <name>`: Remove a shared directory (permanently deletes contents).

## Template Management

### `fabric templates`

Manages agent templates.

- `list`: List available templates.
- `show <name>`: Show configuration of a template.
- `create <name> [--harness <type>]`: Create a new template.
- `clone <src> <dest>`: Clone a template.
- `delete <name>` (alias `rm`): Delete a template.
- `import <source>`: Import agent definitions (from Claude/Gemini) as templates.
- `update-default`: Update the global default template with the latest from the binary.
    - Flags:
        - `--force`: Overwrite the existing default template if it already exists.
- `sync [--all]`: Sync project-level templates with the Hub. Use `--all` to sync all templates at once.
- `status`: Show the sync status of templates relative to the Hub.

## Hub Integration

### `fabric hub`

Manages connection to and interaction with a Fabric Hub. Authentication lives under `fabric hub auth` (there is no top-level `fabric auth` command).

- `fabric hub auth`: Manage Hub authentication.
    - `login`: Authenticate with Hub server (opens a browser; supports `--no-browser` for device flow and `--provider github`).
    - `logout`: Clear stored credentials.
- `fabric hub token`: Manage user access tokens (scoped, revocable bearer tokens for CI/CD and automation).
    - `create`: Create a new token. Flags: `--project`, `--name`, `--scopes`, `--expires`.
    - `list`: List your access tokens.
    - `revoke <token-id>`: Revoke a token (remains visible in listings as revoked).
    - `delete <token-id>`: Permanently delete a token.
- `fabric hub status`: Show the current Hub connection status.
- `fabric hub notifications`: Retrieve a list of recent system notifications and agent alerts.
- `fabric hub link`: Link the current local project to the Hub.
- `fabric hub unlink`: Unlink the current project from the Hub locally.
- `fabric hub projects`: List all projects registered on the Hub.
- `fabric hub brokers`: List all runtime brokers registered on the Hub.
- `fabric hub secret`: Manage write-only secrets on the Hub.
    - `set <key> <value>`: Set a secret.
    - `get [key]`: Get secret metadata.
    - `clear <key>`: Remove a secret.
- `fabric hub env`: Manage environment variables on the Hub.
    - `set <key>=<value>`: Set a variable.
    - `get [key]`: Get variable values.
    - `clear <key>`: Remove a variable.
- `fabric hub project create <git-url>`: Create a project from a remote git repository.
    - Flags: `--slug`, `--name`, `--branch`, `--visibility`, `--json`

## Infrastructure

### `fabric broker`

Manages the local host as a Runtime Broker.

- `fabric broker status`: Show status of the local broker server.
- `fabric broker start`: Start the broker server as a background daemon.
- `fabric broker stop`: Stop the broker daemon.
- `fabric broker register`: Register this host as a Runtime Broker with the Hub.
- `fabric broker deregister`: Remove this broker's registration from the Hub.
- `fabric broker provide`: Add this broker as a provider for a project.
- `fabric broker withdraw`: Remove this broker as a provider from a project.

### `fabric server`

Manages Fabric server components (Hub and Broker).

- `fabric server start`: Start one or more server components.
    - Flags: `--enable-hub`, `--enable-runtime-broker`, `--port`, `--db`, `--dev-auth`.

## Miscellaneous

### `fabric version`

Prints the Fabric version information.

**Usage:** `fabric version`


