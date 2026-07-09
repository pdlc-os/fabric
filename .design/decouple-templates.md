# Decoupling Templates from Harnesses

## Overview

This document explores the design trade-offs of decoupling agent templates from harnesses in Fabric. Currently, templates and harnesses are tightly coupled (1:1 mapping). This analysis evaluates whether and how to separate them.

## Current Architecture

Templates and harnesses are **tightly coupled**:
- Each template directory (e.g., `embeds/claude/`) maps 1:1 to a harness
- Templates contain both harness-specific files (`.claude.json`, `settings.json`) and potentially portable content (`claude.md` instructions, system prompts)
- The harness's `SeedTemplateDir()` method creates the template structure
- `fabric-agent.yaml` declares `harness: <name>` to bind them

### Key Files

| Path | Purpose |
|------|---------|
| `pkg/harness/harness.go` | Factory function mapping harness name to implementation |
| `pkg/api/harness.go` | `Harness` interface definition |
| `pkg/config/templates.go` | Template discovery, loading, and merging |
| `pkg/config/embeds/<harness>/` | Default template files per harness |
| `pkg/config/embeds/<harness>/fabric-agent.yaml` | Template config declaring harness binding |

---

## Pros of Decoupling

### Already Identified

**Re-use across harnesses**: A detailed set of common agent settings (system prompt, skills, MCP server config) can be defined once and used with different harnesses.

### Additional Pros

**1. Separation of "What" vs "How"**
- Template defines the agent's *purpose* (system prompt, capabilities, MCP servers, behavioral constraints)
- Harness defines *execution mechanics* (auth, command invocation, status parsing)
- This matches how users actually think: "I want a code reviewer agent" vs "I'll run it on Claude"

**2. A/B Testing and Migration**
- Same agent role can be tested across harnesses without duplicating template content
- Enables controlled migration from one LLM to another (Gemini → Claude) by swapping harness at runtime
- Useful for cost optimization (run expensive tasks on Opus, cheap ones on Haiku/Gemini)

**3. Template Marketplace/Sharing**
- Portable templates become sharable artifacts independent of vendor lock-in
- Hub users could publish "code-reviewer" or "security-auditor" templates usable with any supported harness

**4. Simpler Template Inheritance**
- If you wanted a "base-code-reviewer" template extended by "strict-code-reviewer", inheritance would be cleaner without harness entanglement

**5. Reduced Template Proliferation**
- Currently you'd need `code-reviewer-claude`, `code-reviewer-gemini`, etc.
- With decoupling: one `code-reviewer` template, harness chosen at start time

---

## Cons of Decoupling

### Already Identified

**Extension awkwardness**: Any harness-specific settings would need to be handled in an awkward "extension set" and template resolution into an agent would require more complex instantiation.

### Additional Cons

**1. Semantic Gap in Portability**
- Some template content is inherently non-portable:
  - Claude's `claude.md` uses Claude-specific syntax
  - Gemini's `system_prompt.md` may use Gemini-specific features
  - MCP server configs may only work with certain harnesses
- You'd need a "translation layer" or accept that some templates only *partially* port

**2. Increased Configuration Surface**
- Users must now understand two concepts and their interaction
- "Why doesn't my Gemini template work with Claude?" becomes a support burden
- Error messages become harder: is the failure a template issue or harness incompatibility?

**3. Validation Complexity**
- Currently: template is self-validating (harness knows what files it needs)
- Decoupled: need to validate template × harness compatibility at instantiation time
- Some combinations may fail silently or behave unexpectedly

**4. Loss of Harness-Specific Optimization**
- Current model allows harnesses to provide opinionated defaults (e.g., Claude's `--dangerously-skip-permissions`)
- Decoupled model may lead to "lowest common denominator" templates

**5. Breaking Change for Existing Users**
- Current templates would need migration
- `fabric template create --harness claude` workflow would change significantly

---

## Middle-Ground Options

### Option A: Template Composition with Harness Adapters

```
template/
  base/            # Portable content (system prompt, MCP servers)
  adapters/
    claude/        # Claude-specific overrides
    gemini/        # Gemini-specific overrides
```

At instantiation: merge `base/` + `adapters/<harness>/`

### Option B: Harness as Runtime Override

Keep templates as-is but allow `fabric start --harness gemini` to override the template's declared harness. The harness would then adapt/translate what it can.

### Option C: Abstract System Prompt Format

Define a portable DSL for system prompts that gets transpiled to harness-specific formats. Templates remain decoupled but compatibility is explicit.

---

## Decision Framework

Consider these questions:

1. **How often will users actually want the same template on different harnesses?**
   - If rarely: the complexity isn't worth it
   - If often: the duplication isn't worth it

2. **Is your primary audience harness-loyal or harness-agnostic?**
   - Claude shops won't care about portability
   - Enterprises hedging bets will value it highly

3. **How do you handle the hosted Hub scenario?**
   - If Hub stores templates, portability becomes more valuable (shared team templates)
   - If templates are always local, less pressing

4. **What's the upgrade story?**
   - Can you introduce decoupling later as an *optional* layer without breaking existing templates?

---

## Option A Deep Dive: Template Composition with Harness Adapters

This section provides detailed design for Option A, covering authoring, storage, and runtime instantiation.

### Conceptual Model

A template becomes a **layered configuration** with:

1. **Base Layer**: Harness-agnostic content defining the agent's role
2. **Adapter Layers**: Harness-specific implementations/overrides
3. **Instance Layer**: Per-agent customizations (existing behavior)

```
┌─────────────────────────────────────────┐
│           Instance Overrides            │  (agent-specific)
├─────────────────────────────────────────┤
│         Harness Adapter Layer           │  (claude/, gemini/)
├─────────────────────────────────────────┤
│              Base Template              │  (portable content)
└─────────────────────────────────────────┘
```

### Directory Structure

#### Template Authoring (in `.fabric/templates/` or Hub storage)

```
code-reviewer/
├── fabric-template.yaml          # Template metadata (replaces fabric-agent.yaml)
├── base/
│   ├── home/
│   │   └── .config/
│   │       └── mcp-servers.json # Portable MCP config
│   ├── system-prompt.md         # Abstract/portable system prompt
│   └── skills/                  # Skill definitions (if portable)
└── adapters/
    ├── claude/
    │   ├── home/
    │   │   └── .claude/
    │   │       ├── claude.md    # Claude-specific instructions
    │   │       └── settings.json
    │   └── adapter.yaml         # Claude-specific config overrides
    ├── gemini/
    │   ├── home/
    │   │   └── .gemini/
    │   │       ├── gemini.md
    │   │       ├── settings.json
    │   │       └── system_prompt.md  # Rendered from base + Claude specifics
    │   └── adapter.yaml
    └── opencode/
        └── ...
```

#### New Config Files

**fabric-template.yaml** (template metadata):
```yaml
# Template identity
name: code-reviewer
version: 1.0.0
description: "Thorough code review agent with security focus"

# Compatibility declaration
compatible_harnesses:
  - claude
  - gemini
default_harness: claude

# Base configuration (harness-agnostic)
base:
  env:
    REVIEW_STRICTNESS: high
  volumes:
    - source: ~/.config/lint-rules
      target: /config/lint-rules
      read_only: true

# Optional: abstract capabilities the adapters must implement
requires:
  - system_prompt
  - mcp_filesystem
```

**adapters/claude/adapter.yaml**:
```yaml
harness: claude

# Harness-specific config
config:
  command_args:
    - "--allowedTools"
    - "Read,Grep,Glob"

# File mappings (how to derive harness files from base)
mappings:
  # Copy base system-prompt.md content into claude.md with wrapper
  - from: base/system-prompt.md
    to: home/.claude/claude.md
    transform: claude-md-wrapper

# Additional env vars for this harness
env:
  CLAUDE_CODE_USE_BEDROCK: "0"
```

### Authoring Workflow

#### Creating a New Decoupled Template

```bash
# Create template with base + adapters structure
$ fabric template create code-reviewer --decoupled

# Or create from existing coupled template
$ fabric template decouple claude-default --name code-reviewer
```

The `template create --decoupled` command would:

1. Create the directory structure above
2. Generate a skeleton `fabric-template.yaml`
3. Create empty `base/` and `adapters/` directories
4. Optionally scaffold adapters for specified harnesses

#### Adding Harness Support

```bash
# Add gemini adapter to existing template
$ fabric template add-adapter code-reviewer --harness gemini
```

This would:
1. Create `adapters/gemini/` structure
2. Copy harness defaults from `pkg/config/embeds/gemini/`
3. Generate `adapter.yaml` with harness binding

### Storage (Local and Hub)

#### Local Storage

Templates stored in:
- Project: `.fabric/templates/<name>/`
- Global: `~/.fabric/templates/<name>/`

No change to discovery logic in `FindTemplate()`, but `LoadConfig()` would detect decoupled templates by presence of `fabric-template.yaml`.

#### Hub Storage

For hosted mode, templates stored in object storage:

```
gs://<bucket>/fabric/<grove-id>/templates/<template-name>/
├── fabric-template.yaml
├── base/
│   └── ...
└── adapters/
    └── ...
```

Hub API additions:
```
GET  /api/v1/templates/{name}              # Template metadata
GET  /api/v1/templates/{name}/base         # Download base layer
GET  /api/v1/templates/{name}/adapters/{h} # Download specific adapter
POST /api/v1/templates/{name}/adapters/{h} # Upload adapter
```

### Runtime Instantiation Flow

#### Agent Start Sequence

```
fabric start my-agent --template code-reviewer --harness gemini
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. Template Resolution                                       │
│    FindTemplate("code-reviewer")                             │
│    → Detect decoupled template (fabric-template.yaml exists)  │
│    → Parse template metadata                                 │
└─────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Harness Resolution                                        │
│    if --harness provided:                                    │
│        use specified harness                                 │
│    else:                                                     │
│        use template.default_harness                          │
│                                                              │
│    Validate: harness ∈ template.compatible_harnesses         │
└─────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Layer Assembly                                            │
│    a. Copy base/ to agent home                               │
│    b. Copy adapters/<harness>/ overlay                       │
│    c. Execute transforms defined in adapter.yaml             │
│    d. Apply instance overrides (env, volumes from CLI)       │
└─────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Harness Provisioning                                      │
│    harness.Provision(agentName, agentHome, agentWorkspace)   │
│    → Harness-specific setup (e.g., update .claude.json)      │
└─────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Container Start                                           │
│    runtime.Start(container, harness.GetCommand(...))         │
└─────────────────────────────────────────────────────────────┘
```

#### Layer Assembly Detail

The critical step is layer assembly. Pseudocode:

```go
func AssembleAgentHome(template *DecoupledTemplate, harness api.Harness, agentHome string) error {
    // 1. Copy base layer
    if err := util.CopyDir(template.BasePath(), agentHome); err != nil {
        return fmt.Errorf("copying base layer: %w", err)
    }

    // 2. Find and validate adapter
    adapterPath := template.AdapterPath(harness.Name())
    if _, err := os.Stat(adapterPath); os.IsNotExist(err) {
        return fmt.Errorf("template %s has no adapter for harness %s",
            template.Name, harness.Name())
    }

    // 3. Load adapter config
    adapter, err := LoadAdapter(adapterPath)
    if err != nil {
        return err
    }

    // 4. Copy adapter files (overlay on base)
    adapterHome := filepath.Join(adapterPath, "home")
    if err := util.CopyDirMerge(adapterHome, agentHome); err != nil {
        return fmt.Errorf("applying adapter layer: %w", err)
    }

    // 5. Execute transforms
    for _, mapping := range adapter.Mappings {
        if err := executeTransform(template, agentHome, mapping); err != nil {
            return fmt.Errorf("transform %s: %w", mapping.Transform, err)
        }
    }

    return nil
}
```

#### Transform System

Transforms allow deriving harness-specific files from base content:

```go
type Transform interface {
    Execute(baseContent []byte, context TransformContext) ([]byte, error)
}

// Built-in transforms
var transforms = map[string]Transform{
    "claude-md-wrapper": &ClaudeMdWrapper{},
    "gemini-system-prompt": &GeminiSystemPromptBuilder{},
    "passthrough": &Passthrough{},
}
```

Example: `claude-md-wrapper` transform:
```go
func (t *ClaudeMdWrapper) Execute(baseContent []byte, ctx TransformContext) ([]byte, error) {
    // Wrap portable system prompt in Claude-specific structure
    return []byte(fmt.Sprintf(`# Agent Role

%s

# Additional Claude-Specific Instructions

Follow the above role definition. Use available tools appropriately.
`, string(baseContent))), nil
}
```

### Backward Compatibility

Existing "coupled" templates continue to work:

```go
func (t *Template) IsDecoupled() bool {
    templateYaml := filepath.Join(t.Path, "fabric-template.yaml")
    _, err := os.Stat(templateYaml)
    return err == nil
}

func (t *Template) LoadConfig() (*api.FabricConfig, error) {
    if t.IsDecoupled() {
        return t.loadDecoupledConfig()
    }
    // Existing behavior for coupled templates
    return t.loadLegacyConfig()
}
```

### CLI Changes

```bash
# Existing (still works)
fabric start agent1 --template claude

# New: explicit harness selection
fabric start agent1 --template code-reviewer --harness gemini

# New: template management
fabric template create my-template --decoupled
fabric template add-adapter my-template --harness claude
fabric template add-adapter my-template --harness gemini
fabric template validate my-template --harness gemini  # Check compatibility
fabric template decouple old-template --name new-template  # Migrate
```

### Migration Path

1. **Phase 1**: Implement decoupled template support alongside existing system
2. **Phase 2**: Provide `fabric template decouple` command for migration
3. **Phase 3**: Update default templates in `pkg/config/embeds/` to decoupled format
4. **Phase 4**: (Optional) Deprecate coupled format with warning

### Open Questions

1. **Transform complexity**: How sophisticated should the transform system be? Simple text templating vs. full AST manipulation?

2. **Partial adapters**: Can an adapter omit files and inherit from base? Or must adapters be complete?

3. **Adapter inheritance**: Could `adapters/claude-bedrock/` extend `adapters/claude/`?

4. **Validation depth**: Should `fabric template validate` actually spin up a container, or just check file structure?

5. **Hub template versioning**: How do adapter versions relate to base template versions?

---

## Resource Specifications in Templates

Templates should declare compute resource requirements, similar to Kubernetes pod specifications. This enables predictable agent behavior across different runtime environments and workloads.

### Motivation

Different agent roles have different resource needs:
- A **code reviewer** may need minimal CPU but significant memory for large diffs
- A **build agent** may need high CPU for compilation tasks
- A **research agent** running multiple MCP servers may need both

Without explicit resource specs:
- Agents may be starved or over-provisioned
- Runtime behavior becomes unpredictable across environments
- Capacity planning for hosted deployments is guesswork

### Resource Schema

Add a `resources` section to template configuration:

```yaml
# In fabric-template.yaml (decoupled) or fabric-agent.yaml (coupled)
resources:
  requests:
    cpu: "500m"        # 0.5 CPU cores (minimum guaranteed)
    memory: "512Mi"    # 512 MiB (minimum guaranteed)
  limits:
    cpu: "2"           # 2 CPU cores (maximum allowed)
    memory: "4Gi"      # 4 GiB (maximum allowed, OOM-killed if exceeded)
```

#### Units

Follow Kubernetes conventions for familiarity:

| Resource | Unit Examples | Notes |
|----------|---------------|-------|
| CPU | `"100m"`, `"0.5"`, `"2"` | Millicores or decimal cores |
| Memory | `"256Mi"`, `"1Gi"`, `"2G"` | Binary (Mi/Gi) or decimal (M/G) |

#### Defaults

Templates without explicit resources inherit runtime defaults:

```yaml
# Runtime-provided defaults (configurable per broker)
resources:
  requests:
    cpu: "250m"
    memory: "256Mi"
  limits:
    cpu: "1"
    memory: "2Gi"
```

### Runtime Mapping

Each runtime translates resource specs to its native mechanism:

#### Kubernetes Runtime

Direct mapping to pod spec:

```go
func (k *K8sRuntime) applyResources(pod *corev1.Pod, res *api.ResourceSpec) {
    pod.Spec.Containers[0].Resources = corev1.ResourceRequirements{
        Requests: corev1.ResourceList{
            corev1.ResourceCPU:    resource.MustParse(res.Requests.CPU),
            corev1.ResourceMemory: resource.MustParse(res.Requests.Memory),
        },
        Limits: corev1.ResourceList{
            corev1.ResourceCPU:    resource.MustParse(res.Limits.CPU),
            corev1.ResourceMemory: resource.MustParse(res.Limits.Memory),
        },
    }
}
```

#### Docker Runtime

Maps to `docker run` flags:

```go
func (d *DockerRuntime) applyResources(config *container.HostConfig, res *api.ResourceSpec) {
    config.Resources = container.Resources{
        CPUQuota:  cpuToQuota(res.Limits.CPU),      // --cpu-quota
        CPUPeriod: 100000,                           // --cpu-period (default 100ms)
        Memory:    memoryToBytes(res.Limits.Memory), // --memory
        // Note: Docker doesn't have true "requests", only limits
        // Requests are advisory for scheduling decisions
    }
}
```

#### Apple Virtualization Runtime

Maps to VM configuration:

```go
func (a *AppleRuntime) applyResources(vmConfig *vz.VirtualMachineConfiguration, res *api.ResourceSpec) {
    vmConfig.SetCPUCount(cpuToCores(res.Limits.CPU))
    vmConfig.SetMemorySize(memoryToBytes(res.Limits.Memory))
    // Apple VZ doesn't support fine-grained CPU limits like millicores
    // Round up to nearest whole core
}
```

### Harness-Specific Resource Profiles

Adapters can override base resource requirements:

```yaml
# adapters/claude/adapter.yaml
harness: claude

# Claude Code benefits from more memory for large contexts
resources:
  requests:
    memory: "1Gi"    # Override base request
  limits:
    memory: "8Gi"    # Override base limit
    # CPU inherits from base
```

Resource merging follows the layer model:
```
Final = Base Resources <- Adapter Resources <- Instance Overrides
```

### CLI Support

```bash
# Override resources at start time
fabric start my-agent --template code-reviewer \
  --cpu-limit 4 \
  --memory-limit 8Gi

# View resource usage
fabric status my-agent --resources
# Output:
#   CPU:    0.45 / 2.00 cores (22%)
#   Memory: 1.2Gi / 4Gi (30%)

# List agents with resource info
fabric list --wide
# NAME          TEMPLATE        CPU-REQ  CPU-LIM  MEM-REQ  MEM-LIM  STATUS
# code-review   code-reviewer   500m     2        512Mi    4Gi      THINKING
# builder       build-agent     1        4        1Gi      8Gi      EXECUTING
```

### Hub and Broker Considerations

#### Broker Capacity

Brokers advertise available capacity:

```yaml
# Broker registration
broker:
  id: broker-01
  capacity:
    cpu: "16"       # 16 cores total
    memory: "64Gi"  # 64 GiB total
  allocated:
    cpu: "4.5"      # Currently allocated
    memory: "12Gi"
```

#### Scheduling

Hub considers resources when assigning agents to brokers:

```go
func (h *Hub) selectBroker(agent *api.Agent) (*api.Broker, error) {
    resources := agent.Template.Resources

    for _, broker := range h.availableBrokers(agent.GroveID) {
        if broker.CanAccommodate(resources) {
            return broker, nil
        }
    }

    return nil, fmt.Errorf("no broker with sufficient capacity for %s (needs %s CPU, %s memory)",
        agent.Name, resources.Requests.CPU, resources.Requests.Memory)
}
```

#### Resource Quotas

Groves can have resource quotas in hosted mode:

```yaml
# Grove configuration in Hub
grove:
  id: my-project
  quotas:
    max_agents: 10
    total_cpu: "32"      # Max 32 cores across all agents
    total_memory: "128Gi"
```

### Example Templates

#### Lightweight Review Agent

```yaml
name: quick-reviewer
resources:
  requests:
    cpu: "100m"
    memory: "256Mi"
  limits:
    cpu: "500m"
    memory: "1Gi"
```

#### Heavy Build Agent

```yaml
name: build-agent
resources:
  requests:
    cpu: "2"
    memory: "4Gi"
  limits:
    cpu: "8"
    memory: "16Gi"
```

#### Research Agent with MCP Servers

```yaml
name: research-agent
resources:
  requests:
    cpu: "1"
    memory: "2Gi"
  limits:
    cpu: "4"
    memory: "8Gi"
# Note: MCP servers run in the same container, consuming these resources
```

### Open Questions (Resources)

1. **GPU support**: Should we support `nvidia.com/gpu` style resource requests for ML workloads?

2. **Ephemeral storage**: Should templates specify disk requirements beyond the mounted volumes?

3. **Network bandwidth**: Is network QoS relevant for agent workloads?

4. **Resource classes**: Should we support named profiles (e.g., `small`, `medium`, `large`) that map to concrete specs per runtime?

5. **Vertical scaling**: Can running agents have resources adjusted, or only at start time?
