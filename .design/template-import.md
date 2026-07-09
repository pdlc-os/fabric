# Template Import: Research Report & Design Document

## Status: Phase 1 Complete
## Author: Agent (c-template-import)
## Date: 2026-02-13
## Updated: 2026-02-18

---

## 1. Overview

This document covers the research and design for a **template import** feature in fabric. The goal is to allow users to import agent or sub-agent definitions from existing harness ecosystems (Claude Code, Gemini CLI) and convert them into fabric templates.

These harnesses define specialized agent configurations as markdown files with YAML front matter containing a system prompt and metadata. Fabric can consume these definitions and produce first-class fabric templates, enabling users to quickly onboard their existing agent customizations.

---

## 2. Research: Source Formats

### 2.1 Claude Code Sub-Agent Definitions

**Locations:**
| Scope | Path |
|-------|------|
| Project | `.claude/agents/*.md` |
| User | `~/.claude/agents/*.md` |

**Format:** Markdown files with YAML front matter.

**Example:**
```markdown
---
name: code-reviewer
description: Reviews code for quality and best practices
tools: Read, Glob, Grep, Bash
disallowedTools: Write, Edit
model: sonnet
permissionMode: default
maxTurns: 10
---

You are a code reviewer. When invoked, analyze the code and provide
specific, actionable feedback on quality, security, and best practices.
```

**Front Matter Fields:**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | Yes | Lowercase with hyphens, unique identifier |
| `description` | string | Yes | Used for routing decisions |
| `tools` | comma-separated string | No | Available tools (inherits all if omitted) |
| `disallowedTools` | comma-separated string | No | Tools to deny |
| `model` | string | No | `sonnet`, `opus`, `haiku`, `inherit` |
| `permissionMode` | string | No | `default`, `acceptEdits`, `dontAsk`, `delegate`, `bypassPermissions`, `plan` |
| `maxTurns` | integer | No | Max agentic turns |
| `skills` | array | No | Preloaded skill names |
| `mcpServers` | object | No | MCP server configs |
| `hooks` | object | No | Lifecycle hooks |
| `memory` | string | No | `user`, `project`, `local` |

**Detection Signals:**
- File located under `.claude/agents/` directory
- Markdown file with YAML front matter
- Front matter contains `name` + `description` (both required)
- Presence of Claude-specific fields: `tools` as comma-separated string, `permissionMode`, `disallowedTools`
- Tool names use Claude Code conventions: `Read`, `Glob`, `Grep`, `Bash`, `Edit`, `Write`, `Task`, `WebFetch`, `WebSearch`

**Other Claude Code Configuration (NOT sub-agents, but relevant context):**
- `CLAUDE.md` files: Pure markdown, no front matter. Custom instructions only.
- `.claude/settings.json`: JSON config for permissions, hooks, env.
- `.claude/rules/*.md`: Conditional rules with `paths` in front matter.

---

### 2.2 Gemini CLI Sub-Agent Definitions

**Locations:**
| Scope | Path |
|-------|------|
| Project | `.gemini/agents/*.md` |
| User | `~/.gemini/agents/*.md` |

**Format:** Markdown files with YAML front matter.

**Example:**
```markdown
---
name: security-auditor
description: Specialized in finding security vulnerabilities in code.
kind: local
tools:
  - read_file
  - grep_search
model: gemini-2.5-pro
temperature: 0.2
max_turns: 10
timeout_mins: 5
---

You are a ruthless Security Auditor. Your job is to find vulnerabilities
in the codebase and report them with severity ratings.
```

**Front Matter Fields:**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `name` | string | Yes | Lowercase, numbers, hyphens, underscores |
| `description` | string | Yes | Used for delegation routing |
| `kind` | string | No | `local` (default) or `remote` (A2A protocol) |
| `tools` | array of strings | No | Available tools as YAML list |
| `model` | string | No | Model name or `inherit` |
| `temperature` | number | No | 0.0-2.0 |
| `max_turns` | number | No | Default 15 |
| `timeout_mins` | number | No | Default 5 |

**Remote agents** (kind: remote) additionally have:
- `agent_card_url`: URL to the A2A agent card

**Detection Signals:**
- File located under `.gemini/agents/` directory
- Markdown file with YAML front matter
- Front matter contains `name` + `description` (both required)
- Presence of Gemini-specific fields: `kind`, `timeout_mins`, `temperature`
- Tool names use Gemini CLI conventions: `read_file`, `write_file`, `edit_file`, `grep_search`, `glob`, `run_shell_command`, `web_fetch`, `web_search`
- `tools` field is a YAML array (not comma-separated)

**Other Gemini CLI Configuration (NOT sub-agents, but relevant context):**
- `GEMINI.md` files: Pure markdown, no front matter. Custom instructions.
- `.gemini/settings.json`: JSON config for model, tools, security, experimental features.
- `.gemini/skills/*/SKILL.md`: Skills with YAML front matter (separate concept from agents).
- `.gemini/commands/*.toml`: Custom slash commands.

---

### 2.3 Gemini CLI Skills (Supplementary)

While not sub-agents, Gemini skills follow the [Agent Skills](https://agentskills.io) standard and may also be import candidates in the future:

```markdown
---
name: code-reviewer
description: Use this skill to review code.
---

# Code Reviewer
This skill guides the agent in conducting thorough code reviews...
```

Skills live in `.gemini/skills/<skill-name>/SKILL.md` with optional `scripts/`, `references/`, and `assets/` directories.

---

## 3. Comparison: Source Format vs Fabric Template

### 3.1 What a Fabric Template Contains

A fabric template is a directory with the following structure:

```
templates/<name>/
├── fabric-agent.yaml          # Harness configuration
└── home/
    ├── .bashrc               # Shell environment
    ├── .tmux.conf             # Terminal multiplexer config
    └── .<harness-config-dir>/ # Harness-specific config
        ├── settings.json      # Harness settings
        └── <instructions>.md  # System prompt / instructions
```

The `fabric-agent.yaml` contains a `FabricConfig`:

```yaml
harness: claude          # or gemini, opencode, codex
configDir: .claude       # harness config directory name
model: sonnet            # model specification
env:                     # environment variables
  KEY: value
commandArgs: []          # extra CLI arguments
detached: true           # run in background
```

### 3.2 Mapping: Source Fields → Fabric Template

| Source Field (Claude/Gemini) | Fabric Equivalent | Notes |
|------------------------------|------------------|-------|
| `name` | Template directory name | Slugified |
| `description` | `fabric-agent.yaml` → new `description` field or metadata | Currently no description field exists |
| `model` | `fabric-agent.yaml` → `model` | Needs model name normalization |
| Body (markdown below front matter) | `home/<config-dir>/<instructions>.md` | The system prompt |
| `tools` | Partial mapping to harness settings | Tool restrictions are harness-specific |
| `maxTurns` / `max_turns` | `fabric-agent.yaml` → `commandArgs` or harness settings | Could map to CLI args |
| `permissionMode` (Claude) | Claude settings.json | Harness-specific |
| `temperature` (Gemini) | Gemini settings.json | Harness-specific |
| `kind` (Gemini) | N/A | Remote agents not supported initially |
| `mcpServers` | Harness settings | Pass-through |
| `hooks` | Not mapped initially | Complex, harness-specific |

---

## 4. Design: Template Import System

### 4.1 Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    fabric template import                     │
│                                                              │
│  ┌──────────┐    ┌──────────────┐    ┌───────────────────┐  │
│  │ Detector │───>│   Parser     │───>│  Template Writer  │  │
│  │          │    │ (per-harness)│    │                   │  │
│  └──────────┘    └──────────────┘    └───────────────────┘  │
│       │                                       │              │
│       │  Identifies source type               │  Writes to   │
│       │  (claude, gemini, auto)                │  .fabric/     │
│       │                                       │  templates/  │
│       ▼                                       ▼              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Source Files                              │   │
│  │  .claude/agents/*.md  |  .gemini/agents/*.md          │   │
│  │  Single .md files     |  Directories                  │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 Detection Strategy

Detection operates in two modes:

**Auto-detect (from path):**
1. If path is a directory:
   - Check for `.claude/agents/*.md` → Claude source
   - Check for `.gemini/agents/*.md` → Gemini source
   - If both exist, import both (or let user choose)
2. If path is a single `.md` file:
   - Parse front matter
   - Score against harness signatures (see below)

**Signature-Based Detection (from file content):**

Each harness has a detection signature based on front matter fields:

```
Claude Code signals (weighted):
  +3  tools field is a comma-separated string (not YAML array)
  +3  permissionMode field present
  +2  disallowedTools field present
  +2  tool names match Claude conventions (Read, Glob, Grep, Bash, Edit, Write)
  +1  skills field present (shared with Gemini but less common)
  +1  memory field present

Gemini CLI signals (weighted):
  +3  tools field is a YAML array
  +3  kind field present
  +2  timeout_mins field present
  +2  temperature field present
  +2  tool names match Gemini conventions (read_file, grep_search, run_shell_command)
  +1  max_turns field present (vs maxTurns for Claude)

Common (non-discriminating):
  name, description, model, hooks, mcpServers
```

If a file scores >= 3 for one harness and < 2 for the other, classify it. If ambiguous, check the parent directory path for `.claude/` or `.gemini/` as tiebreaker.

**Explicit override:** The CLI always allows `--harness claude|gemini` to skip detection.

### 4.3 Parser Interface

```go
// pkg/config/templateimport/types.go

// ImportedAgent represents a parsed agent definition from any harness
type ImportedAgent struct {
    Name          string            // Agent/template name
    Description   string            // What this agent does
    Harness       string            // Source harness: "claude", "gemini"
    Model         string            // Model specification (normalized)
    SystemPrompt  string            // Markdown body (the system prompt)
    Tools         []string          // Tool list (harness-native names)
    MaxTurns      int               // Max turns/iterations
    Temperature   float64           // Temperature (Gemini only, 0 = unset)
    RawFrontMatter map[string]any   // Full front matter for pass-through
    SourcePath    string            // Original file path
}

// Importer parses harness-specific agent definitions
type Importer interface {
    // Detect returns a confidence score (0-10) that the file is this format
    Detect(path string, frontMatter map[string]any) int
    // Parse reads a file and returns an ImportedAgent
    Parse(path string) (*ImportedAgent, error)
    // ParseDir scans a directory for importable agent definitions
    ParseDir(dir string) ([]*ImportedAgent, error)
}
```

### 4.4 Per-Harness Importers

**Claude Code Importer** (`pkg/config/templateimport/claude.go`):
- Parses `.md` files with YAML front matter
- Handles `tools` as comma-separated string
- Maps `permissionMode` to Claude settings.json
- Maps `model` aliases: `sonnet` → model name, `opus` → model name, `haiku` → model name
- Writes system prompt to `home/.claude/CLAUDE.md` (agent instructions)
- Generates Claude `settings.json` with tool permissions derived from `tools`/`disallowedTools`

**Gemini CLI Importer** (`pkg/config/templateimport/gemini.go`):
- Parses `.md` files with YAML front matter
- Handles `tools` as YAML string array
- Skips `kind: remote` agents (not supported)
- Maps `temperature` into Gemini `settings.json`
- Maps `model` as direct model name pass-through
- Writes system prompt to `home/.gemini/GEMINI.md` (agent instructions)

### 4.5 Template Writer

The template writer takes an `ImportedAgent` and produces a fabric template directory:

```go
// pkg/config/templateimport/writer.go

func WriteTemplate(agent *ImportedAgent, grovePath string, force bool) (string, error)
```

**Output structure** (example for Claude import):
```
.fabric/templates/<name>/
├── fabric-agent.yaml
│   harness: claude
│   configDir: .claude
│   model: <mapped-model>
│   # description: <agent description>
│   # imported_from: <source path>
└── home/
    ├── .bashrc          # From embedded defaults
    ├── .tmux.conf       # From embedded defaults
    └── .claude/
        ├── CLAUDE.md    # System prompt from markdown body
        └── settings.json # Generated from tools/permissions
```

The writer:
1. Creates the template directory structure
2. Seeds common files from embedded defaults (via the default template base layer)
3. Writes the system prompt to the appropriate harness instruction file
4. Generates harness-specific settings from the parsed metadata
5. Writes `fabric-agent.yaml` with harness config and model

### 4.6 CLI Command

```
fabric template import [flags] <source>

Arguments:
  source    Path to an agent definition file (.md), a directory containing
            agent definitions, or a project root to scan for .claude/agents/
            and .gemini/agents/ directories.

Flags:
  --harness string    Force harness type (claude, gemini). Auto-detected if omitted.
  --name string       Override the template name (default: from front matter 'name' field)
  --grove string      Target grove path (default: resolved grove)
  --force             Overwrite existing template with same name
  --dry-run           Show what would be imported without writing files
  --all               Import all discovered agents from the source

Examples:
  # Import a single Claude sub-agent definition
  fabric template import .claude/agents/code-reviewer.md

  # Import all Gemini agents from a project
  fabric template import --all .gemini/agents/

  # Auto-detect and import all agents from project root
  fabric template import --all .

  # Import with explicit harness and custom name
  fabric template import --harness gemini --name my-auditor agents/security.md

  # Preview import without writing
  fabric template import --dry-run .claude/agents/code-reviewer.md
```

### 4.7 Model Name Normalization

Since Claude Code uses short aliases and Gemini uses full model names, we need normalization:

| Source | Source Value | Fabric Value | Notes |
|--------|-------------|-------------|-------|
| Claude | `sonnet` | `sonnet` | Pass through; fabric/harness resolves |
| Claude | `opus` | `opus` | Pass through |
| Claude | `haiku` | `haiku` | Pass through |
| Claude | `inherit` | (omit) | Use template default |
| Gemini | `gemini-2.5-pro` | `gemini-2.5-pro` | Pass through |
| Gemini | `gemini-2.5-flash` | `gemini-2.5-flash` | Pass through |
| Gemini | `inherit` | (omit) | Use template default |

Model names are stored as-is in `fabric-agent.yaml`. The harness runtime handles resolution.

---

## 5. Edge Cases and Considerations

### 5.1 System Prompt vs Custom Instructions
- Claude Code and Gemini CLI both distinguish between the **sub-agent system prompt** (the markdown body) and **project-level custom instructions** (CLAUDE.md / GEMINI.md at project root).
- The import only captures the sub-agent definition's body as the template's instruction file. It does NOT import project-level CLAUDE.md/GEMINI.md content.

### 5.2 Tool Mapping Limitations
- Fabric templates don't have a universal tool permission model. Tool restrictions are harness-specific.
- For Claude: tools map to `settings.json` permission rules
- For Gemini: tools map to `settings.json` tool configuration
- Imported tool lists are best-effort; users may need to adjust after import.

### 5.3 MCP Servers
- Both harnesses support MCP server definitions in sub-agents
- These can be mapped to harness-specific settings files during import
- Phase 1 may skip MCP import and log a warning

### 5.4 Hooks
- Both harnesses support hooks in agent definitions
- Hooks are highly specific to each harness's lifecycle events
- Phase 1 skips hook import and logs a warning

### 5.5 Remote Agents (Gemini)
- Gemini `kind: remote` agents reference external A2A endpoints
- These have no direct fabric equivalent
- Phase 1 skips remote agents and logs a warning

### 5.6 Cross-Harness Import
- A Claude sub-agent definition describes Claude-specific behavior. It should produce a fabric template for the Claude harness, not attempt to translate it to Gemini or vice versa.
- The `harness` field in the output template always matches the source harness.

### 5.7 Name Conflicts
- If a template with the same name already exists, the import fails unless `--force` is used.
- Names are slugified from the front matter `name` field (lowercase, hyphens, no special chars).

---

## 6. Implementation Phases

### Phase 1: Core Import (MVP) ✅ Complete
- [x] `Importer` interface and `ImportedAgent` type
- [x] Claude Code importer: parse front matter + body, detect signature
- [x] Gemini CLI importer: parse front matter + body, detect signature
- [x] Auto-detection logic (path-based + content-based)
- [x] Template writer: produce fabric template directory structure
- [x] CLI command: `fabric template import` (`cmd/template_import.go`, 301 lines)
- [x] Map `name`, `description`, `model`, `tools`, system prompt
- [x] Single file and directory import, auto-discovery, dry-run mode, force overwrite, custom name override
- [ ] Tests: unit tests for parsing, detection, and template output

### Phase 2: Enhanced Mapping
- [ ] MCP server configuration import
- [ ] Claude `permissionMode` → settings.json mapping
- [ ] Gemini `temperature` → settings.json mapping
- [ ] `maxTurns`/`max_turns` → command args or settings
- [ ] `--dry-run` output formatting (show diff/preview)

### Phase 3: Batch & Discovery
- [ ] Scan entire project root for all importable definitions
- [ ] Import report: summary of what was imported, warnings, skipped items
- [ ] Interactive mode: prompt user for name/harness when ambiguous

---

## 7. File Organization

```
pkg/config/templateimport/
├── types.go           # ImportedAgent, Importer interface
├── detect.go          # Auto-detection logic
├── claude.go          # Claude Code importer
├── gemini.go          # Gemini CLI importer
├── writer.go          # Template directory writer
├── frontmatter.go     # YAML front matter parser (shared)
└── templateimport_test.go  # Tests

cmd/
└── template_import.go # CLI command definition
```

---

## 8. Open Questions

1. **Description field in fabric-agent.yaml**: Currently `FabricConfig` has no `description` field. Should we add one, or store the description in a separate metadata file? Adding it to `FabricConfig` seems cleanest.

2. **Harness-specific settings generation**: Should we generate full harness settings files (e.g., Claude `settings.json` with tool permissions), or keep it minimal and let users customize post-import?

3. **CLAUDE.md vs dedicated file**: Should the imported system prompt go into `CLAUDE.md` (which is what the harness reads for instructions) or into a separate `system_prompt.md` that fabric manages? The current template embed uses `claude.md` (lowercase), not `CLAUDE.md`.

4. **Batch import naming**: When importing multiple agents from a directory, should we prefix names with the harness (e.g., `claude-code-reviewer`) to avoid collisions, or keep original names?
