# Multi-Agent Workflow Patterns

**Status:** EXPLORATORY
**Author:** coordinate-brainstorm
**Created:** 2026-02-11

## Overview

This document explores high-level patterns for orchestrating multi-agent workflows in Fabric, using specialized templates for distinct roles (planner, architect, security reviewer, code reviewer, integrator, etc.). It examines what the platform currently supports, identifies gaps, and proposes a layered approach to enabling complex, decomposable project work across a set of coordinated agents.

---

## Current State

### What Exists

Fabric already provides the right primitives for isolation and identity:

- **Container-based agent isolation** with dedicated home directories and environment.
- **Git worktrees** giving each agent an independent workspace without conflicts.
- **Template system** (`pkg/config/embeds/`) seeding harness config, system prompts, and environment per agent type.
- **Grove-scoped identity** with JWT tokens carrying `grove_id` and `scopes`.
- **Multiple harness backends** (Claude, Gemini, OpenCode, Codex, Generic).
- **Message broadcasting** via tmux to all agents in a grove.
- **Status file mechanism** (STARTING, THINKING, EXECUTING, WAITING_FOR_INPUT, COMPLETED, ERROR).

### What's Missing

The main gap is **orchestration** -- there is no mechanism for agents to decompose work, delegate to sub-agents, or coordinate autonomously:

- **No agent-initiated sub-agent creation.** Agent tokens lack the required scopes, and Hub handlers reject non-user callers. (A design for this exists at `.design/hosted/agent-hub-access.md` but is not yet implemented.)
- **No inter-agent communication.** Agents cannot message siblings or query each other's status.
- **No workflow DSL or declarative orchestration.**
- **No structured task artifacts.** Task input is a plain string; there is no schema for task definitions, dependencies, or deliverables.
- **No completion signaling.** No event/callback mechanism for detecting when agents finish.
- **No aggregated workflow progress tracking.**

---

## Layer 1: Role-Specialized Templates

Templates are the natural place to encode agent specialization. Today a template is mostly a harness config plus a system prompt. For role-based agents, templates should carry richer semantics:

```yaml
# .fabric/templates/architect/fabric-agent.yaml
harness: claude
role: architect
hub_access:
  scopes:
    - grove:agent:create
    - grove:agent:lifecycle
    - grove:agent:read
capabilities:
  - can_delegate        # can create sub-agents
  - can_merge           # has write access to integration branch
  - can_approve         # can mark other agents' work as accepted
constraints:
  max_sub_agents: 5
  allowed_templates:    # what this role can spawn
    - implementer
    - security-reviewer
    - test-writer
```

Each role template includes a `system_prompt.md` that gives the LLM its identity, goals, and behavioral constraints.

### Proposed Role Catalog

| Role | Responsibility | Key Capability |
|------|---------------|----------------|
| **Planner** | Decomposes a high-level goal into tasks with dependencies | `can_delegate`, creates task graph |
| **Architect** | Reviews planner output, validates design, sets branch strategy | `can_approve`, `can_delegate` |
| **Implementer** | Writes code on an isolated branch for a single task | Worktree-scoped, no delegation |
| **Security Reviewer** | Audits code changes for vulnerabilities | Read-only workspace, can flag/block |
| **Test Writer** | Writes tests for implemented features | Reads implementer branches |
| **Code Reviewer** | Reviews PRs, suggests changes | Read-only, can approve/reject |
| **Integrator** | Merges approved branches, resolves conflicts | Write access to integration branch |

---

## Layer 2: Workflow Patterns

Three core patterns, ordered by increasing autonomy.

### Pattern A: Fan-Out / Fan-In

The simplest and most immediately achievable pattern. A planner agent receives a high-level task, decomposes it into independent sub-tasks, and spawns one implementer per sub-task on its own branch. When all complete, an integrator agent merges.

```
         +--- Implementer A (feature-a branch)
         |
Planner -+--- Implementer B (feature-b branch)
         |
         +--- Implementer C (feature-c branch)
                      |
                      v
                 Integrator (merges all branches)
```

**Requirements:**
- Agent-to-hub access (the proposed design) for the planner to spawn implementers.
- A **completion signal** mechanism -- the planner needs to know when implementers finish. This could be polling `GET /agents/{id}` for status, or a webhook/event model.
- A **task artifact** -- a structured way for the planner to pass task definitions to implementers. Today this is just the `task` string on start; a `prompt.md` file or structured task payload would be better.

### Pattern B: Pipeline (Sequential Stages)

Each stage produces output that feeds the next. The output is the git branch state plus an optional structured artifact (review report, test plan, etc.).

```
Architect -> Implementer -> Security Review -> Test Writer -> Code Review -> Integrator
```

**Requirements:**
- A **handoff mechanism** -- when one agent completes, trigger the next. Options:
  - A coordinator agent that polls and launches stages.
  - A simple event/callback system in the Hub (`on_agent_complete: start <next-template>`).
  - Agent status transitions triggering Hub-side hooks.
- **Branch conventions** -- each stage works on the same branch or creates a follow-up branch. The template should encode whether the agent gets a fresh worktree or inherits a predecessor's branch.

### Pattern C: Autonomous Coordination

A top-level coordinator agent with broad `hub_access` scopes manages the entire workflow. It can react to events, re-plan when things fail, and make judgment calls.

```
                    Coordinator
                   /     |     \
            Architect  Planner  Security
               |         |        |
           [spawns]   [spawns]  [audits]
              |         |        |
         Impl A,B    Impl C,D   Reviews
              \         /        |
               Integrator <------+
```

**Requirements:**
- Everything from Patterns A and B.
- **Inter-agent communication** beyond status polling. Options:
  1. **Shared files in grove**: Agents write structured artifacts to `.fabric/artifacts/<agent>/` which others can read.
  2. **Hub message queue**: A simple pub/sub or mailbox API on the Hub where agents post structured messages.
  3. **Git-based coordination**: Agents commit structured files (e.g., `REVIEW.md`, `TASKS.json`) that other agents read from the repo.

---

## Layer 3: The Coordination Protocol

The most critical missing piece. This should be a lightweight protocol layered on top of the Hub API.

### Task Definition Format

```yaml
# .fabric/tasks/task-001.yaml  (committed to repo or stored in Hub)
id: task-001
title: "Implement user authentication middleware"
created_by: planner-agent
assigned_to: implementer-auth
depends_on: []
blocks: [task-005]
status: pending  # pending | in_progress | completed | failed | blocked
branch: feature/auth-middleware
acceptance_criteria:
  - "Middleware validates JWT tokens"
  - "Returns 401 for invalid tokens"
  - "Passes existing test suite"
artifacts:
  input:
    - "main:pkg/hub/auth.go"  # reference files for context
  output:
    - "review-report.md"      # expected deliverables
```

### Agent Status Extensions

Extend the current status model beyond THINKING/EXECUTING/COMPLETED:

```go
type WorkflowStatus struct {
    Phase       string   // "planning", "implementing", "reviewing", "blocked"
    TaskID      string   // current task being worked on
    SubAgents   []string // IDs of spawned sub-agents
    Artifacts   []string // paths to output artifacts
    BlockedBy   []string // task IDs this agent is waiting on
}
```

This would be written to the existing status file mechanism and relayed through the Hub.

### Completion and Handoff

When an agent completes its task:

1. Commit all work to its branch.
2. Write a structured completion artifact (results, branch ref, notes).
3. Update task status to `completed`.
4. Update agent status to `COMPLETED`.

The coordinator or next-stage agent detects this via Hub API polling or event subscription and proceeds.

---

## Layer 4: Implementation Sequence

Prioritized phases for building this out.

### Phase 1: Enable the "Lead Agent" Pattern

- Complete the agent-to-hub access design (token scopes, handler changes per `.design/hosted/agent-hub-access.md`).
- Add a `GET /agents/{id}/status` endpoint that agents can poll.
- Add a structured task payload field to `StartOptions` beyond a plain string.

### Phase 2: Shared Artifacts and Task Tracking

- Define the task artifact format (YAML/JSON files in `.fabric/tasks/`).
- Build a `fabric tasks` CLI command for listing and updating tasks.
- Add Hub API endpoints for task CRUD in hosted mode.

### Phase 3: Workflow Templates (Declarative)

Introduce a `workflow.yaml` file that declares the agent graph:

```yaml
name: "full-review-pipeline"
stages:
  - name: plan
    template: planner
    task: "Decompose: ${GOAL}"
  - name: implement
    template: implementer
    fan_out: true  # one agent per task from planner output
    depends_on: [plan]
  - name: security
    template: security-reviewer
    depends_on: [implement]
  - name: integrate
    template: integrator
    depends_on: [security]
```

Add a `fabric workflow start <workflow.yaml> --goal "..."` command that launches the coordinator.

### Phase 4: Event-Driven Coordination

- Hub-side webhooks or event streams for agent status changes.
- Replace polling with push notifications.
- Enable reactive patterns (security reviewer auto-triggered on branch push).

---

## Design Considerations

### Git Branch Strategy

The biggest practical challenge in multi-agent code workflows is merge conflicts, not orchestration. Each implementer needs a clean, isolated branch. The integrator role is non-trivial; it must:

- Merge branches in dependency order.
- Detect and resolve conflicts (or escalate to a human).
- Run the full test suite after integration.
- Potentially ask implementers to rebase if conflicts are too complex.

**Recommendation:** Implementers branch from a stable base (`main` or a designated integration branch). The integrator merges sequentially rather than all at once, running tests after each merge.

### Scope Ceiling Enforcement

A planner agent that can spawn sub-agents which can also spawn sub-agents creates a recursion risk. Mitigations:

- Template-level `max_sub_agents` and `allowed_templates` fields prevent runaway agent creation.
- The Hub should enforce a grove-wide agent count limit.
- Sub-agents should not receive broader scopes than their parent.
- All agent-initiated operations should be logged with the originating agent ID.

### Communication Layer

Git itself is an excellent coordination substrate. Agents already share a repository. Having them communicate via committed files (task definitions, review reports, status updates) is simple, auditable, and works with the existing worktree infrastructure. A message queue is tempting but premature -- file-based coordination with Hub polling is sufficient for most workflows.

### Coordinator Optionality

The coordinator agent should be optional, not mandatory. Simple fan-out workflows don't need a persistent coordinator. A `fabric workflow` command can handle launching and monitoring. The autonomous coordinator pattern should be reserved for complex, adaptive workflows where replanning is expected.

---

## Recommended Starting Point

Get agent-to-hub access working, define the task artifact format, and build one end-to-end fan-out/fan-in workflow as a proof of concept. The declarative workflow DSL and event system can follow once the basic patterns are validated.
