# Harness-Agnostic Templates: Design Decision

## Status: Decided
## Date: 2026-02-16
## Related: `decouple-templates.md`, `versioned-settings-refactor.md`

---

## 1. Executive Summary

This document defines the design for transforming Fabric's template system from a harness-coupled model (each template is 1:1 with a harness) to a harness-agnostic model where a template defines the *role* of an agent (system prompt, agent instructions, skills, resources) and is combined with a harness-config at creation time.

The refactor is feasible but touches many layers of the system. The key challenges are: (1) separating harness-specific home directory content from portable template content, (2) rethinking how embedded defaults work, (3) adapting the provisioning flow to a two-phase "compose template + apply harness-config" model, and (4) organizing harness-config directories on disk to co-locate configuration and home directory base files.

The decided approach is **Harness-Config Base Layers with Optional Template Overrides**: harness-configs stored on disk at `~/.fabric/harness-configs/<name>/` provide the default base layer, while templates may optionally include a `harness-configs/` directory for template-specific overrides or defaults.

---

## 2. Current State Analysis

### 2.1 The Template ↔ Harness Coupling

Today, templates and harnesses are tightly coupled at every level:

**Source embeds** (`pkg/config/embeds/`): Each harness has its own embed directory containing a complete agent home layout. Common files (`.tmux.conf`, `.zshrc`, `.gitconfig`, `.geminiignore`) are embedded in the default agnostic template at `embeds/templates/default/home/` and seeded via `SeedAgnosticTemplate()`.

```
pkg/config/embeds/
├── templates/default/home/    # Common shell/terminal config (seeded as default template)
│   ├── .tmux.conf
│   ├── .zshrc
│   ├── .gitconfig
│   └── .gemini/.geminiignore
├── claude/                    # Claude-coupled template
│   ├── fabric-agent.yaml       # declares harness: claude
│   ├── .claude.json           # Claude Code state file
│   ├── settings.json          # Claude Code settings (hooks, env, permissions)
│   ├── claude.md              # Agent instructions (fabrictool hooks)
│   └── bashrc                 # harness-specific aliases
├── gemini/                    # Gemini-coupled template
│   ├── fabric-agent.yaml       # declares harness: gemini + auth config
│   ├── settings.json          # Gemini CLI settings (hooks, model, security)
│   ├── gemini.md              # Agent instructions (fabrictool hooks)
│   ├── system_prompt.md       # Placeholder
│   └── bashrc                 # harness-specific aliases
├── codex/                     # Codex-coupled template
│   ├── fabric-agent.yaml       # declares harness: codex
│   ├── config.toml            # Codex config (model, approval policy)
│   └── bashrc                 # harness-specific aliases
└── opencode/                  # OpenCode-coupled template
    ├── fabric-agent.yaml       # declares harness: opencode
    └── opencode.json          # OpenCode config (theme)
```

**Template seeding** (`Harness.SeedTemplateDir()`): Each harness implementation controls its own template directory layout. The `SeedAgnosticTemplate()` function in `pkg/config/init.go` seeds the default template (including common home files) in a single `fs.WalkDir()` pass.

**Template discovery** (`pkg/config/templates.go`): Templates are discovered by name (e.g., `gemini`, `claude`) and their `fabric-agent.yaml` declares which harness they bind to. The template name conventionally matches the harness name.

**Agent provisioning** (`pkg/agent/provision.go`): The provisioning flow copies `template/home/` into `agent/home/`, then calls `harness.Provision()` to do harness-specific setup (e.g., Claude updates `.claude.json` with workspace paths).

### 2.2 What Is Actually Harness-Specific vs Portable

To determine feasibility, we must classify every piece of template content:

#### Harness-Specific Content (must remain with harness)

| Content | Harness | Purpose |
|---------|---------|---------|
| `.claude.json` | Claude | CLI state file (onboarding, project trust, MCP servers) |
| `.claude/settings.json` | Claude | Claude Code settings (env vars, hooks, permissions, auto-updater) |
| `.gemini/settings.json` | Gemini | Gemini CLI settings (hooks, model, yolo mode, auth, security) |
| `.codex/config.toml` | Codex | Codex config (model, approval policy, workspace trust) |
| `.config/opencode/opencode.json` | OpenCode | OpenCode config (theme) |
| Auth discovery logic | All | Each harness finds credentials differently |
| Container command | All | Each harness has different CLI invocation (`gemini --yolo`, `claude --no-chrome`, etc.) |
| Interrupt key | All | Claude uses Escape, others use C-c |
| Hook dialect | All | `fabrictool hook --dialect=claude` vs `--dialect=gemini` |

#### Abstract Harness Capabilities

While many commands are harness-specific, several common behaviors can be defined at an abstract level on the `Harness` interface when harnesses support them:

| Capability | Description |
|-----------|-------------|
| Resume/Continue | Mechanism to resume the harness after it pauses or completes a turn |
| Message delivery | Getting the harness a message, with optional interrupt of current work |
| Telemetry on/off | Controlling whether the harness sends telemetry, with potential sub-settings |
| Supported dialect | Which fabrictool hook dialect the harness uses |

These abstract capabilities allow the template and agent system to reason about harness behavior without coupling to specific implementation details.

#### Portable Content (should belong to template)

| Content | Current Location | Purpose |
|---------|-----------------|---------|
| System prompt | Not consistently present | Defines the agent's role and capabilities |
| Agent instructions | `.claude/claude.md`, `.gemini/gemini.md` | Agent-level guidance (e.g., fabrictool integration, workflow rules) |
| `.bashrc` (common portion) | Per-harness embeds | Common shell aliases and setup |
| `.zshrc` | Needs adding to common | Common zsh setup |
| `.tmux.conf` | `common/` | Terminal multiplexer config (already shared) |
| `fabric-agent.yaml` fields: `env`, `volumes`, `resources`, `services`, `max_turns`, `max_duration` | Template | Agent runtime configuration |
| Telemetry settings | Per-harness | Abstract on/off control that maps to harness-specific config |

#### Agent Instructions and System Prompts

Agent instructions are a common feature of agent harnesses — they are not fabric-specific. They provide guidance to the agent such as workflow rules, tool usage patterns, and integration instructions. System prompts define the agent's role, capabilities, and behavioral constraints.

A harness-agnostic template supports **both** system prompts and agent instructions as distinct portable artifacts. At the harness layer, these are generally combined and delivered to the harness CLI through its native mechanisms.

All harnesses now support `agents.md` as a common agent instruction file. This content lives in the template as portable agent instructions. At agent creation time, the harness either:
- **(Preferred)** Uses a harness-specific setting to read `agents.md` from a standard location
- Falls back to a symlink from the harness-expected location to `agents.md`

System prompt support is more varied across harnesses. Most harnesses have a mechanism for system prompts, but the location and format differ. The harness implementation code is responsible for delivering both template artifacts to the right location.

**System prompt downgrade:** When a harness does not support a custom system prompt (e.g., Codex, OpenCode currently), the system prompt content must be **downgraded** into the agent instructions — appended or prepended to the agent instructions content so that the agent still receives the role definition, even if not through the harness's native system prompt mechanism. The `InjectSystemPrompt()` harness method handles this: harnesses that support system prompts write to the native location; harnesses that don't support system prompts merge the content into agent instructions instead.

### 2.3 Key Observation

The current `claude.md` and `gemini.md` files are **identical** — they both contain the same agent instructions. This validates the premise that templates should be harness-agnostic: the actual agent instructions are already portable, only the delivery mechanism differs. With `agents.md` support across all harnesses, agent instruction delivery is a solved problem.

System prompt delivery remains a challenge that this design addresses through the `InjectSystemPrompt()` harness method with a downgrade-to-agent-instructions fallback for harnesses that lack native system prompt support.

---

## 3. Decided Approach: Harness-Config Base Layers with Optional Template Overrides

Harness-configs stored on disk at `~/.fabric/harness-configs/<name>/` provide the default base layer. Templates may **optionally** include a `harness-configs/` directory that provides template-specific overrides or defaults for specific harnesses.

### 3.1 Target Template Structure

A harness-agnostic template defines the agent's *purpose*, not its *execution mechanics*:

```yaml
# .fabric/templates/code-reviewer/fabric-agent.yaml
schema_version: "1"
name: code-reviewer
description: "Thorough code review agent with security focus"

# Portable agent configuration
agent_instructions: agents.md    # path relative to template dir
system_prompt: system-prompt.md  # path relative to template dir (optional)

# Optional: default harness-config to use if user doesn't specify one
default_harness_config: gemini

# Optional: template-specific harness-config overrides
# (see harness-configs/ directory below)

env:
  REVIEW_STRICTNESS: high

resources:
  requests:
    cpu: "500m"
    memory: "512Mi"
  limits:
    cpu: "2"
    memory: "4Gi"

max_turns: 100
max_duration: "4h"

services:
  - name: browser
    command: ["chromium", "--headless"]
    ready_check:
      type: tcp
      target: "localhost:9222"
```

The YAML format supports inline plain text content for both `agent_instructions` and `system_prompt` fields (e.g., using YAML `|` multi-line syntax), in addition to file path references. This is supported by standard YAML libraries and is convenient for simple templates, but will not be used in our default templates or documented initially.

```
.fabric/templates/code-reviewer/
├── fabric-agent.yaml             # Agent/template configuration (harness-agnostic)
├── agents.md                    # Portable agent instructions
├── system-prompt.md             # Portable system prompt (optional)
├── home/                        # Portable home directory content (optional)
│   └── .config/
│       └── lint-rules/          # Custom config files
└── harness-configs/             # Optional: template-specific harness-config overrides
    ├── gemini/
    │   └── config.yaml          # Override model, hooks, etc. for this template
    └── claude/
        └── config.yaml          # Override settings for this template
```

**Note on naming:** We retain `fabric-agent.yaml` rather than introducing `fabric-template.yaml`. A template defines an agent, and after provisioning, the `fabric-agent.yaml` in the agent directory represents the composed configuration (template settings + profile env vars + harness-config). Keeping the same filename creates a natural lineage from template to provisioned agent.

### 3.2 How It Combines with Harness-Config

At `fabric create`, the user specifies a template and optionally a harness-config:

```bash
# Explicit harness-config
fabric create my-agent --template code-reviewer --harness-config gemini

# Uses template's default_harness_config if set, otherwise falls back to
# the default_harness_config from settings.yaml
fabric create my-agent --template code-reviewer
```

Settings must include a `default_harness_config` field to replace the notion of "default_template" implying a harness. The `default_template` setting is retained but no longer implies a harness — it only selects which portable template to use.

The system resolves:
1. **Template** → `code-reviewer` (provides agent instructions, system prompt, env, resources, services)
2. **Harness-config** → resolved from: CLI `--harness-config` flag → template's `default_harness_config` → settings `default_harness_config`
3. **Harness-config on disk** → `~/.fabric/harness-configs/<name>/` (provides home dir base files, runtime config)
4. **Harness** → derived from harness-config's `harness` field (provides behavior, auth, command, hooks)

The agent home directory is assembled by composing:
1. Harness-config base layer (harness-specific config files from `harness-configs/<name>/home/`)
2. Template-specific harness-config overrides (if present in template's `harness-configs/` dir)
3. Template home content (portable files)
4. Agent instructions injection
5. System prompt injection (with downgrade fallback)
6. Common files (`.tmux.conf`, `.bashrc`, `.zshrc`)

### 3.3 Harness-Config Directories on Disk

Each harness-config is a named directory containing both its runtime configuration and base home files:

```
~/.fabric/
├── harness-configs/              # Harness-config base layers
│   ├── claude/                   # Default claude harness-config
│   │   ├── config.yaml           # Runtime params: image, user, model, env, auth
│   │   └── home/                 # Base home directory files
│   │       ├── .claude.json
│   │       ├── .claude/
│   │       │   └── settings.json
│   │       └── .bashrc
│   ├── gemini/                   # Default gemini harness-config
│   │   ├── config.yaml
│   │   └── home/
│   │       ├── .gemini/
│   │       │   └── settings.json
│   │       └── .bashrc
│   ├── codex/
│   │   ├── config.yaml
│   │   └── home/
│   │       ├── .codex/
│   │       │   └── config.toml
│   │       └── .bashrc
│   ├── opencode/
│   │   ├── config.yaml
│   │   └── home/
│   │       ├── .config/
│   │       │   └── opencode/
│   │       │       └── opencode.json
│   │       └── .bashrc
│   └── gemini-experimental/      # Custom user-defined harness-config variant
│       ├── config.yaml           # harness: gemini, but with different model/settings
│       └── home/
│           └── .gemini/
│               └── settings.json
├── templates/                    # Harness-agnostic templates
│   ├── code-reviewer/
│   │   ├── fabric-agent.yaml
│   │   ├── agents.md
│   │   ├── system-prompt.md
│   │   └── harness-configs/      # Optional: template-specific overrides
│   │       └── gemini/
│   │           └── config.yaml   # e.g., set a specific model for this template
│   └── researcher/
│       ├── fabric-agent.yaml
│       ├── agents.md
│       └── system-prompt.md
└── settings.yaml                 # References harness-configs by name, no longer inline
```

Grove-level overrides are also supported: `.fabric/harness-configs/<name>/` at the project level takes precedence over the global `~/.fabric/harness-configs/<name>/`.

Custom harness-config variants (e.g., `gemini-experimental`) are created manually by copying and modifying an existing harness-config directory. No dedicated CLI command is provided; the process is documented.

### 3.4 Template-Specific Harness-Config Overrides

Templates can optionally include a `harness-configs/` directory. This preserves one of the key advantages of the current 1:1 model — a template author can specify a particular model or other harness-specific settings that make sense for that template's purpose:

```
templates/researcher/
├── fabric-agent.yaml
│   # default_harness_config: gemini
├── agents.md
├── system-prompt.md
└── harness-configs/
    └── gemini/
        └── config.yaml     # Override: model: gemini-3-pro (research needs stronger model)
```

When the template specifies a `default_harness_config` and also has a matching override in `harness-configs/`, the template's override `config.yaml` scalar values take precedence over the base harness-config's `config.yaml` values during composition.

**Important limitation (initial release):** Deep merging of hooks (combining base harness-config hooks with template-specific hooks) is deferred to a future refactor. For the initial implementation, template-specific harness-config overrides apply to scalar `config.yaml` values only (e.g., `model`, `image`, `env` additions). Authors who want custom hooks must create a complete custom harness-config that includes both the standard fabric integration hooks and their custom hooks.

**Future direction:** The long-term goal is to support deep-merge of hooks, allowing template authors to add hooks that are merged with the base harness-config's hook settings. This may take the form of allowing an array of harness-configs that are merged in order, enabling composable configuration layers. This is deferred because it introduces significant complexity around merge semantics (append vs replace vs deduplicate) and validation.

### 3.5 Composition at Agent Creation

```
Agent Home = Harness-Config Base Layer (from ~/.fabric/harness-configs/<name>/home/)
           + Template Harness-Config Overrides (if template has harness-configs/<name>/)
           + Template Home (if any)
           + Agent Instructions Injection
           + System Prompt Injection
           + Common Files (.tmux.conf, .bashrc, .zshrc)
           + Settings/Profile Overrides
```

Steps:
1. Copy harness-config base home → agent home
2. Apply template harness-config scalar overrides to `config.yaml` values (if present)
3. Copy template home → agent home (overlay, template files win on conflict)
4. Inject agent instructions into harness-specific location (or configure harness to read `agents.md`)
5. Inject system prompt — write to harness-native location, or downgrade into agent instructions if harness lacks system prompt support
6. Common files (`.tmux.conf`, `.zshrc`, `.gitconfig`) are already included via the default template base layer
7. Call `Harness.Provision()` for dynamic setup (e.g., workspace paths in `.claude.json`)

### 3.6 Agent Instruction & System Prompt Injection

The harness interface gains new methods:

```go
type Harness interface {
    // ... existing methods ...

    // InjectAgentInstructions configures the harness to read agent instructions
    // from the standard agents.md location, or creates the necessary symlinks/copies.
    InjectAgentInstructions(agentHome string, content []byte) error

    // InjectSystemPrompt delivers the system prompt to the harness. If the harness
    // supports a native system prompt mechanism, writes to that location. If not,
    // the content is merged into agent instructions as a fallback (downgrade).
    InjectSystemPrompt(agentHome string, content []byte) error
}
```

Implementations for `InjectAgentInstructions`:
- **Claude**: Configures `.claude/settings.json` to read `agents.md`, or writes to `{agentHome}/AGENTS.md`
- **Gemini**: Configures setting to read `agents.md`, or symlinks `{agentHome}/.gemini/gemini.md` → `agents.md`
- **Generic**: Writes to `{agentHome}/agents.md`

Implementations for `InjectSystemPrompt`:
- **Claude**: Writes to `{agentHome}/.claude/CLAUDE.md`
- **Gemini**: Writes to `{agentHome}/.gemini/system_prompt.md`
- **Codex**: Downgrade — prepends system prompt content to agent instructions
- **OpenCode**: Downgrade — prepends system prompt content to agent instructions
- **Generic**: Writes to `{agentHome}/.fabric/system_prompt.md`

### 3.7 Embed Restructuring

```
pkg/config/embeds/
├── common/
│   ├── .tmux.conf
│   ├── bashrc                     # Common shell setup
│   └── zshrc                      # Common zsh setup
├── templates/                     # Default agnostic templates
│   ├── default/
│   │   ├── fabric-agent.yaml
│   │   ├── agents.md
│   │   └── system-prompt.md
│   └── (future: code-reviewer/, researcher/, etc.)
└── default_settings.yaml

pkg/harness/
├── claude/
│   └── embeds/                    # Claude harness-config defaults
│       ├── config.yaml
│       ├── .claude.json
│       ├── settings.json          # Goes to home/.claude/settings.json
│       └── bashrc                 # Harness-specific shell additions
├── gemini/
│   └── embeds/
│       ├── config.yaml
│       ├── settings.json          # Goes to home/.gemini/settings.json
│       └── bashrc
├── codex/
│   └── embeds/
│       ├── config.yaml
│       ├── config.toml            # Goes to home/.codex/config.toml
│       └── bashrc
└── opencode/
    └── embeds/
        ├── config.yaml
        └── opencode.json          # Goes to home/.config/opencode/opencode.json
```

**Pros:**
- Harness-specific content maintained once per harness-config, not duplicated per template
- Templates are purely about agent purpose — easy to author, share, version
- Templates can optionally customize harness behavior for their specific role
- Clean separation: harness embeds live with harness code in `pkg/harness/`
- Co-located config + home files under `harness-configs/` on disk
- Harness-config variants are easy to create (e.g., `gemini-experimental`)

**Cons:**
- More concepts: harness-configs (on disk), templates, composition
- Template authors who want harness-specific overrides still need some harness knowledge

---

## 4. Challenges

### 4.1 Harness-Specific Home Directory Files

**The core challenge.** Each harness requires specific files in the agent home directory:

- Claude needs `.claude.json` and `.claude/settings.json`
- Gemini needs `.gemini/settings.json`
- Codex needs `.codex/config.toml`
- OpenCode needs `.config/opencode/opencode.json`

These files are currently embedded in templates. In a harness-agnostic world, these files must come from somewhere else.

**Decision:** These base files are associated with the **harness-config**, not the harness itself. A harness is a convenience label for common behavior and code-level defaults; the harness-config is the user-facing, customizable entity that carries both runtime parameters and home directory base files.

Storage on disk:

```
~/.fabric/harness-configs/<name>/
├── config.yaml          # Runtime parameters (image, user, model, args, env, volumes, auth)
└── home/                # Base home directory files
    ├── .claude.json     # (for claude harness-configs)
    ├── .claude/
    │   └── settings.json
    └── .bashrc          # Harness-specific shell additions
```

This co-locates configuration and base files, keeping them associated with each other under a single named reference. The `config.yaml` replaces what was previously stored inline in `settings.yaml` under `harness_configs`.

### 4.2 Agent Instructions and System Prompt Delivery

Agent instructions and system prompts are distinct artifacts that serve different purposes:

- **Agent instructions** (`agents.md`): Operational guidance for the agent — workflow rules, tool integration instructions, reporting patterns. A common feature of agent harnesses.
- **System prompt** (`system-prompt.md`): Role definition, capabilities, behavioral constraints. Defines *what* the agent is.

Both are portable template content. At the harness layer, they are generally combined and delivered through the harness's native mechanisms.

**Agent instructions delivery:**

All harnesses now support `agents.md` as a common agent instruction file. The delivery mechanism is:

1. **(Preferred)** The harness-specific configuration includes a setting that tells the harness CLI to read `agents.md` from a known location
2. **(Fallback)** At agent creation, a symlink is created from the harness-expected location to the canonical `agents.md` location

**System prompt delivery:**

System prompt support varies across harnesses. Each harness reads from different locations:
- Claude: `.claude/CLAUDE.md`
- Gemini: `.gemini/system_prompt.md`
- Codex: No system prompt mechanism currently
- OpenCode: No system prompt mechanism currently

A harness-agnostic template may include a `system-prompt.md`. The harness's `InjectSystemPrompt()` method handles delivery:
- Harnesses with native system prompt support write to the expected location
- Harnesses **without** native support **downgrade** the system prompt by merging it into the agent instructions content, ensuring the agent still receives the role definition

### 4.3 Settings/Hooks Configuration

The `settings.json` files for Claude and Gemini contain critical integration hooks:

**Claude** (`settings.json`):
```json
{
  "hooks": {
    "SessionStart": [{"command": "fabrictool hook --dialect=claude"}],
    "PostToolUse": [{"command": "fabrictool hook --dialect=claude"}],
    ...
  },
  "env": { "CLAUDE_CODE_USE_VERTEX": "1", ... },
  "permissions": { "allow": ["*"] }
}
```

**Gemini** (`settings.json`):
```json
{
  "hooks": {
    "SessionStart": [{"command": "fabrictool hook --dialect=gemini"}],
    "BeforeAgent": [{"command": "fabrictool hook --dialect=gemini"}],
    ...
  },
  "yolo": true,
  "model": { "name": "gemini-3-flash-preview" }
}
```

These are harness-specific in format and content. They cannot be made portable. They live in the harness-config's `home/` directory.

**Hook customization (initial release):** Deep merging of hooks from multiple sources (base harness-config + template overrides) is deferred. For the initial implementation, any author who wants custom hooks must create a complete custom harness-config that preserves the common/base hooks needed for fabric integration alongside their custom hooks.

**Future direction:** Allow template authors to include custom hooks in the template that are merged with the harness-config's hook settings. This would likely take the form of allowing an array of harness-configs that merge in order, providing composable configuration layers. This is a natural extension once the base infrastructure is established.

### 4.4 SeedTemplateDir Inversion

Currently the harness controls template creation via `SeedTemplateDir()`. With harness-agnostic templates, there are two distinct operations:

1. **Template creation**: Creates the portable template structure (agent instructions, system prompt, env, resources). This is harness-independent.
2. **Harness-config setup**: Creates the harness-config directory with home files and `config.yaml`. This is template-independent.

The current `SeedTemplateDir()` conflates both. It must be split.

### 4.5 Keeping `fabric-agent.yaml`

The template configuration file remains `fabric-agent.yaml`. The `harness` field is removed entirely — harness binding is determined exclusively by the resolved harness-config at agent creation time.

A `fabric-agent.yaml` without a `harness` field is a harness-agnostic template. Templates that previously declared `harness: claude` (or similar) with that field present are invalid under the new format and will produce an error prompting the user to update their template.

The `fabric-agent.yaml` file serves as the base for the `fabric-agent.yaml` in the provisioned agent directory, which represents the full composed configuration (template + harness-config + profile overrides).

### 4.6 Embed Restructuring

The current `pkg/config/embeds/` directory is organized by harness. In a harness-agnostic world, we need to separate:

- **Harness-config base files**: Moved to `pkg/harness/` package — each harness owns its embedded default home files and config
- **Default templates**: Harness-agnostic templates that ship with the binary, remain in `pkg/config/embeds/`
- **Common files**: Shared across all (`.tmux.conf`, `.zshrc`, `.gitconfig`, `.geminiignore`), embedded in `embeds/templates/default/home/` and seeded as part of the default template via `SeedAgnosticTemplate()`

### 4.7 Harness.Provision() Must Handle Missing Files

Currently `Provision()` assumes the template has already placed harness-specific files (e.g., Claude's `Provision()` reads an existing `.claude.json` to update workspace paths). In the new model, `Provision()` must handle the case where these files don't exist yet and create them from embedded defaults.

### 4.8 Hub Template Storage

In hosted mode, templates are stored in the Hub. The Hub API and storage must support both harness-agnostic templates and harness-config definitions, mirroring the current template storage approach. Templates stored in the Hub include their optional `harness-configs/` overrides directory.

---

## 5. Key Design Decisions

### 5.1 Template config file naming

**Decision:** Keep `fabric-agent.yaml`. The `harness` field is removed entirely. A template defines an agent, and the same file format carries forward into provisioned agents as the composed configuration.

**Rationale:** Introducing `fabric-template.yaml` would create two parallel config formats. Since `fabric-agent.yaml` in a provisioned agent will represent the composed result of template + harness-config + profile, keeping the same filename provides a natural lineage.

### 5.2 Where do harness-config base files live?

**Decision:** On disk at `~/.fabric/harness-configs/<name>/` (global) and `.fabric/harness-configs/<name>/` (grove-level override). Each directory contains `config.yaml` plus a `home/` subdirectory with base files.

**Rationale:** Co-locating `config.yaml` and `home/` keeps all aspects of a harness-config associated. Named references (`--harness-config gemini`) map directly to directory names. Grove-level overrides allow per-project customization.

### 5.3 Should harness-config runtime params stay in settings.yaml?

**Decision:** No. Move harness-config definitions from inline `settings.yaml` entries to on-disk `harness-configs/<name>/config.yaml`. The `settings.yaml` retains only a `default_harness_config` reference and any other non-harness-config-specific settings. Since the `harness_configs` settings block was only just implemented and has no users, this is a direct change with no migration needed.

**Rationale:** Mixing home directory files and runtime parameters in different places is confusing. The on-disk directory approach creates a single source of truth per harness-config.

### 5.4 How does a user customize harness-config files?

**Decision:** Users edit files directly in `~/.fabric/harness-configs/<name>/`. For example, to add custom hooks to Claude's settings.json, they edit `~/.fabric/harness-configs/claude/home/.claude/settings.json`. To change the default model, they edit `~/.fabric/harness-configs/claude/config.yaml`.

A reset mechanism is provided: `fabric harness-config reset claude` restores defaults from embedded files.

### 5.5 What happens during initialization?

**Decision:** Split initialization into two tiers:

- **`fabric init --machine`** (global, first-time): Sets up `~/.fabric/`, seeds all default harness-configs, creates `settings.yaml` with `default_harness_config`, seeds default template(s). This is a prerequisite.
- **`fabric init`** (project-level): Creates `.fabric/` in the project. Lightweight — does not populate harness-config defaults or global templates. If `~/.fabric/` does not exist, errors with guidance to run `fabric init --machine` first.

### 5.6 What about legacy templates?

**Decision:** Breaking change. Templates with a `harness` field in `fabric-agent.yaml` are invalid under the new format. The system will error with a clear message: "Invalid template: 'harness' field is no longer supported in fabric-agent.yaml. Remove it and use --harness-config to specify the harness."

Templates must also have required front-matter fields (e.g., `name`). Invalid or incomplete templates produce an error.

**Rationale:** The project is in alpha. Maintaining backward compatibility with a format that is being fundamentally redesigned adds complexity for limited benefit. A clean break with clear error messages is preferable.

### 5.7 Harness-config resolution for `fabric create`

**Decision:** The harness-config is resolved from (in priority order):
1. CLI `--harness-config` flag
2. Template's `default_harness_config` field in `fabric-agent.yaml`
3. Settings `default_harness_config` field
4. Error if none resolved

The resolved harness-config name maps to a directory, from which the `harness` type is read from `config.yaml`. There is no `harness` field in templates or in the provisioned agent's `fabric-agent.yaml` — the harness is always derived from the harness-config.

### 5.8 System prompt and agent instructions

**Decision:** Templates support both `system_prompt` and `agent_instructions` as distinct portable artifacts. Both fields in `fabric-agent.yaml` accept either a file path (relative to template dir) or inline YAML content.

At the harness layer:
- Agent instructions are delivered via `InjectAgentInstructions()` — all harnesses support this
- System prompts are delivered via `InjectSystemPrompt()` — harnesses that support native system prompts write to the expected location; harnesses that don't support system prompts **downgrade** the content by merging it into agent instructions

**Rationale:** System prompts and agent instructions serve different purposes and are combined at the harness layer. The downgrade mechanism ensures templates work across all harnesses without requiring the template author to know which harnesses support system prompts.

### 5.9 Abstract harness capabilities

**Decision:** Abstract capabilities (resume/continue, message delivery, telemetry on/off, supported dialect) are part of the `Harness` interface, not a separate type.

**Rationale:** These behaviors are intrinsic to what a harness is. A separate type would add indirection without meaningful benefit.

---

## 6. Implementation Phases

This work is divided into four progressive phases. Each phase produces a working system (no broken intermediate states) and can be planned and implemented independently, consulting this design document for context.

### 6.1 Phase 1: Harness-Config Infrastructure

**Goal:** Establish the on-disk harness-config directory model and relocate harness embeds out of `pkg/config/embeds/` into `pkg/harness/`.

**Scope:**
- Create `pkg/harness/<harness>/embeds/` directories by extracting harness-specific home files (`.claude.json`, `settings.json`, `config.toml`, etc.) and harness-specific `.bashrc` additions from the current `pkg/config/embeds/<harness>/` directories
- Define the `config.yaml` schema for harness-configs (harness type, image, user, model, args, env, volumes, auth)
- Implement harness-config loading and resolution from on-disk `~/.fabric/harness-configs/<name>/` and grove-level `.fabric/harness-configs/<name>/` directories
- Implement `SeedHarnessConfig()` to populate harness-config directories from embedded defaults
- Add `.zshrc` to `pkg/config/embeds/common/`

**Milestone:** Harness-configs exist as a loadable on-disk concept with embedded defaults. The existing template/provisioning flow continues to work unchanged — this phase only adds new infrastructure alongside the existing system.

**Key references:** §3.3 (harness-config directories), §4.1 (harness-specific home files), §5.2 (where base files live), §3.7 (embed restructuring)

### 6.2 Phase 2: Agnostic Template Format & Harness Injection

**Goal:** Templates become harness-agnostic. Harnesses gain methods to inject agent instructions and system prompts from portable template artifacts.

**Scope:**
- Update `fabric-agent.yaml` schema: remove the `harness` field entirely, add `agent_instructions`, `system_prompt`, and `default_harness_config` fields
- Update template loading and validation — templates with a legacy `harness` field produce an error with migration guidance
- Add `InjectAgentInstructions()` and `InjectSystemPrompt()` to the `Harness` interface
- Implement both methods for each harness (Claude, Gemini, Codex, OpenCode, Generic), including the system prompt downgrade-to-agent-instructions fallback for harnesses without native system prompt support
- Update template discovery to handle agnostic templates
- Create default agnostic template(s) in `pkg/config/embeds/templates/` with `agents.md` and `system-prompt.md`

**Milestone:** Templates no longer declare a harness. The harness interface supports portable content injection. Default agnostic templates are available as embedded resources. The old per-harness template embeds in `pkg/config/embeds/` can be removed.

**Key references:** §3.1 (target template structure), §3.6 (injection methods), §4.2 (instruction/prompt delivery), §5.1 (config naming), §5.6 (legacy templates), §5.8 (system prompt and agent instructions)

### 6.3 Phase 3: Composition & Provisioning

**Goal:** Wire the full composition flow so that agent creation assembles the agent home from harness-config base layer + template overlay + injection.

**Scope:**
- Implement harness-config resolution chain: CLI `--harness-config` flag → template's `default_harness_config` → settings `default_harness_config` → error
- Build the composition flow in `ProvisionAgent()`: copy harness-config base home, apply template harness-config scalar overrides (if present), overlay template home (common files are included via the default template base layer), inject agent instructions and system prompt, call `Harness.Provision()`
- Add `--harness-config` flag to `fabric create`
- Update `Harness.Provision()` to handle cases where base files may already exist from the harness-config layer (rather than assuming the template placed them)

**Milestone:** End-to-end agent creation works with the new composition model. `fabric create --template X --harness-config Y` produces a correctly assembled agent home directory. The old `SeedTemplateDir()` flow is replaced.

**Key references:** §3.2 (how it combines), §3.4 (template-specific overrides), §3.5 (composition at agent creation), §4.4 (SeedTemplateDir inversion), §5.7 (harness-config resolution)

### 6.4 Phase 4: Init Restructuring, Settings & CLI

**Goal:** Restructure initialization into machine/project tiers, update settings to reference harness-configs by name, and provide management commands.

**Scope:**
- Implement `fabric init --machine` for global setup: seeds all default harness-configs to `~/.fabric/harness-configs/`, creates `settings.yaml` with `default_harness_config`
- Make `fabric init` (project-level) lightweight: creates `.fabric/` without populating harness-config defaults; errors with guidance if `~/.fabric/` is not set up
- Add `default_harness_config` to `settings.yaml` schema
- Remove inline `harness_configs` block from `settings.yaml` (moved to on-disk directories; no migration needed as this block has no existing users)
- Implement `fabric harness-config reset <name>` to restore defaults from embedded files
- Clean up any remaining legacy code paths from the old template-harness coupling

**Milestone:** The full harness-agnostic template system is operational. Initialization, settings, and CLI commands reflect the new model. The transition from harness-coupled to harness-agnostic templates is complete.

**Key references:** §3.3 (harness-config directories), §4.3 (settings/hooks), §5.3 (settings.yaml), §5.4 (user customization), §5.5 (initialization)

---

## 7. Impact on Existing Code

### 7.1 Files That Must Change

| File | Change | Scope |
|------|--------|-------|
| `pkg/api/harness.go` | Add `InjectAgentInstructions()`, `InjectSystemPrompt()` to interface | Small |
| `pkg/harness/*.go` | Implement new interface methods; add `embeds/` directories per harness; implement system prompt downgrade for harnesses that lack native support | Medium |
| `pkg/config/init.go` | Add `SeedHarnessConfig()`, split `InitProject()`/`InitGlobal()`, add `InitMachine()` | Medium |
| `pkg/config/embeds/` | Remove per-harness directories (moved to `pkg/harness/`); add `templates/`, update `common/` with `.zshrc` | Medium |
| `pkg/config/templates.go` | Remove `harness` field handling; validate new `fabric-agent.yaml` format; support `default_harness_config`, `agent_instructions`, and `system_prompt` fields | Medium |
| `pkg/agent/provision.go` | Implement composition logic (harness-config base + template overlay) | Large |
| `cmd/create.go`, `cmd/start.go` | Add `--harness-config` flag; implement resolution chain; remove legacy template flow | Medium |
| `cmd/init.go` | Add `--machine` flag; implement two-tier initialization | Medium |
| `pkg/config/settings_v1.go` | Add `default_harness_config` field; remove inline `harness_configs` (moved to disk) | Medium |
| `pkg/config/harness_config.go` | New: load harness-config from on-disk directory structure | Medium |

### 7.2 Files That Should NOT Change

| File | Reason |
|------|--------|
| `pkg/agent/run.go` | The `Start()` flow doesn't need to know about template type — by start time, the agent home is already composed |
| `pkg/runtime/*.go` | Runtime is agnostic to template model — it just runs a container |
| `cmd/attach.go`, `cmd/list.go` | Post-creation commands are template-model-agnostic |

---

## 8. Migration Path

Since this is a breaking change (alpha project), the migration is direct. The implementation phases in §6 are designed to be progressive — each phase produces a working system. The migration corresponds to completing all four phases:

1. **Phase 1** establishes harness-config infrastructure alongside the existing system (no breaking changes)
2. **Phase 2** introduces the new template format and harness injection methods (breaking change: templates with `harness` field become invalid)
3. **Phase 3** replaces the provisioning flow with the new composition model
4. **Phase 4** restructures initialization and settings to complete the transition

Existing user templates with a `harness` field will produce a clear error after Phase 2 directing the user to remove the field and use `--harness-config` instead. No automated migration tool is provided given the project's alpha status.

---

## 9. Risks

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Composition order bugs (harness-config base vs template override vs common) | Files in wrong location or overwritten | Comprehensive tests with all harness × template combinations |
| Agent instructions delivery fails for new harnesses | Agent runs without instructions | `InjectAgentInstructions` returns error; provisioning fails early |
| System prompt downgrade produces poor results | Agent role definition lost in noise of agent instructions | Downgraded content is prepended with clear section delimiter; test with each harness |
| Users customize harness-configs then `fabric init --machine --force` overwrites them | Lost customizations | Backup before overwrite, warn user, add `--no-harness-configs` flag |
| Agnostic template + missing harness-config = confusing error | User doesn't understand what to specify | Clear error message: "No harness-config resolved. Specify --harness-config, set default_harness_config in the template, or set default_harness_config in settings." |
| Hub storage format for harness-configs | Complex Hub API | Mirror existing template storage approach |
| Harness-config base files drift from embedded defaults | Stale config | `fabric harness-config check` to compare on-disk files with embedded defaults |
| Breaking change invalidates existing user templates | User friction on upgrade | Clear error messages with migration guidance; provide `fabric template migrate` helper |

---

## Appendix A: Alternatives Considered

### A.1 Approach A: Template Composition with Adapter Directories

Each template contains a `base/` directory and per-harness adapter directories. Self-contained but duplicates harness-specific files across every template. Maintaining adapters becomes a burden — updating Claude's `settings.json` format requires updating it in every template. Template authors need to understand harness internals.

**Verdict:** Scales poorly due to per-template duplication of effectively identical harness content.

### A.2 Approach B: Pure Harness-Config Base Layers (No Template Overrides)

Templates contain only portable content with no ability to influence harness-specific settings. Good separation of concerns but templates cannot customize harness behavior (e.g., a "researcher" template that wants a specific model).

**Verdict:** Good foundation but lacks the ability for templates to influence harness-config settings. The decided approach (A+B Hybrid) adds optional template overrides on top of this.

### A.3 Approach C: Harness-Generated Home (No Persistent Base Layer)

The harness generates all harness-specific files on-the-fly during provisioning from Go code. Simplest model with no disk management, but harness-specific files are not user-customizable and there is no way for templates to influence harness-specific settings.

**Verdict:** Too rigid — breaks user customizability, which is a core requirement.

### A.4 Comparison Table

| Aspect | A: Adapters | B: Base Layers | A+B Hybrid (Decided) | C: Generated |
|--------|-------------|----------------|---------------------|-------------|
| Template authoring complexity | High | Low | Low (optional overrides) | Low |
| Harness file duplication | Per-template | Single copy | Single copy + optional overrides | None (in code) |
| User customizability | Per-template | Per-harness-config | Per-harness-config + per-template | Not customizable |
| Template-specific harness tuning | Full control | None | Selective override | None |
| Maintenance burden | High (N×M) | Low (M) | Low (M + optional) | Low (in code) |
| Sharing templates | Large, self-contained | Small, portable | Small, portable + optional config | Small, portable |
