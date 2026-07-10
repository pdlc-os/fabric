# Fabric

Run multiple agents in parallel — each in its own container, with its own workspace, collaborating on your code or project files simultaneously.

_fab·ric /ˈfabrik/ — a structure of interwoven threads; an underlying framework._

Fabric is an experimental multi-agent orchestration testbed designed to manage "deep agents" running in containers.


Fabric orchestrates "deep agents" (Claude Code, Gemini CLI, and others) as isolated, concurrent processes. Each agent gets its own container, git worktree, and credentials — so they can work on different parts of your project without stepping on each other. Agents run locally, on remote VMs, or across Kubernetes clusters.

Rather than prescribing rigid orchestration patterns, Fabric takes a "less is more" approach: agents dynamically learn a CLI tool, letting the models themselves decide how to coordinate among agents. This makes it a rapid prototype testbed for experimenting with multi-agent patterns through natural language prompting. Read more in [Philosophy](https://pdlc-os.github.io/fabric/philosophy/).


## Quick Start

### Workstation Quick Start (Homebrew)

```bash
brew install pdlc-os/fabric/fabric
fabric server start
```

Your browser will open to the onboarding wizard at `http://127.0.0.1:8080/onboarding`, which walks you through machine setup, runtime detection, harness selection, and creating your first project.

### Install from Source

See the full [Installation Guide](https://pdlc-os.github.io/fabric/getting-started/install/), or install from source (requires Go 1.26+):

```bash
go install github.com/pdlc-os/fabric/cmd/fabric@latest
```

> **Warning:** `go install` builds only the Go binary. It does not build or embed the web frontend, so `fabric server start` will serve a blank web UI with missing frontend assets. Build from a clone with `make all` (as in the Quick Start above) for a ready-to-run install.

### Initialize your machine and a Project

> **Tip:** If you used `fabric server start` above, the onboarding wizard handles machine initialization automatically — you can skip this section.

Navigate to your project and create a Fabric project (the `.fabric` directory that holds agent config):

```bash
fabric init --machine
cd my-project
fabric init
```

> **Tip:** Add `.fabric/agents` to your `.gitignore` to avoid issues with nested git worktrees.

Fabric auto-detects your OS and configures the default runtime (Docker on Linux/Windows, Container on macOS). Override this in `.fabric/settings.yaml`.

**NOTE** Currently this project is early and experimental. Most of the concepts are settled in, but many features may not be fully implemented, anything might break or change and the future is not set. Local use is relatively stable, Hub based workflows now highly usable, Kubernetes runtime support still has rough edges.

### Start Agents

```bash
# Start and immediately attach to the session
fabric start debug "Help me debug this error" --attach
```

### Manage Agents

| Command | Description |
|---------|-------------|
| `fabric list` (`ps`) | List active agents |
| `fabric attach <name>` | Attach to a running agent's tmux session |
| `fabric message <name> "..."` (`msg`) | Send a message to a running agent |
| `fabric logs <name>` | View agent logs |
| `fabric stop <name>` | Stop an agent |
| `fabric resume <name>` | Resume a stopped agent |
| `fabric delete <name>` | Remove agent, container, and worktree |

## Key Features

- **Harness Agnostic** — Ships with Gemini CLI and Claude Code by default. Additional harnesses (OpenCode, Codex, Antigravity) are available as [opt-in bundles](harnesses/README.md). Adaptable to anything that runs in a container.
- **True Isolation** — Each agent runs in its own container with separated credentials, config, and a dedicated `git worktree`, preventing merge conflicts.
- **Parallel Execution** — Run multiple agents concurrently as fully independent processes, locally or remotely.
- **Attach / Detach** — Agents run in `tmux` sessions for background operation. Attach for human-in-the-loop interaction, enqueue messages while detached, and tunnel into remote agents securely.
- **Specialization via Templates** — Define agent roles ("Security Auditor", "QA Tester") with custom system prompts and skill sets. See [Templates](https://pdlc-os.github.io/fabric/local/templates/).
- **Multi-Runtime** — Manage execution across Docker, Podman, Apple containers, and Kubernetes via named profiles.
- **Observability** — Normalized OTEL telemetry across harnesses for logging and metrics across agent swarms.

## Core Concepts

| Concept | Description |
|---------|-------------|
| **Agent** | A containerized process running a deep agent harness (Claude Code, Gemini CLI, etc.) |
| **Project** | A project namespace and collection of agents, commonly 1:1 with a git repo |
| **Template** | An agent blueprint — system prompt plus a collection of skills |
| **Runtime** | A container runtime: Docker, Podman, Apple Container, or Kubernetes |
| **Hub** | Optional central control plane for multi-machine orchestration |
| **Runtime Broker** | A machine (laptop or VM) offering its runtimes to a Hub |

Not all concepts apply in every scenario — local mode is simpler. See [Concepts](https://pdlc-os.github.io/fabric/concepts/) for the full picture.

## Documentation

Visit our **[Documentation Site](https://pdlc-os.github.io/fabric/)** for comprehensive guides and reference.

- **[Overview](https://pdlc-os.github.io/fabric/overview/)**: Introduction to Fabric.
- **[Installation](https://pdlc-os.github.io/fabric/getting-started/install/)**: How to get Fabric up and running.
- **[Concepts](https://pdlc-os.github.io/fabric/concepts/)**: Understanding Agents, Projects, Harnesses, and Runtimes.
- **[CLI Reference](https://pdlc-os.github.io/fabric/reference/cli/)**: Comprehensive guide to all Fabric commands.
- **Guides**:
    - [Using Templates](https://pdlc-os.github.io/fabric/local/templates/)
    - [Using Tmux](https://pdlc-os.github.io/fabric/local/tmux/)
    - [Kubernetes Runtime](https://pdlc-os.github.io/fabric/hosted/ha/kubernetes/)

## Project Status

This project is early and experimental. Core concepts are settled, but expect rough edges:

- **Local mode** — relatively stable
- **Hub-based workflows** — ~80% verified
- **Kubernetes runtime** — early, with known rough edges

## Disclaimers

This is not an officially supported Google product. This project is not eligible for the [Google Open Source Software Vulnerability Rewards Program](https://bughunters.google.com/open-source-security).

## License

Apache License, Version 2.0. See [LICENSE](LICENSE).
