# ADK Fabric Agent Example

An example [ADK (Agent Development Kit)](https://google.github.io/adk-docs/) agent that integrates with fabric's lifecycle management. The agent reports its status through fabric's `fabrictool` so it can be orchestrated alongside other agents in a grove.

## Prerequisites

- Python 3.11+
- `google-adk` from git HEAD (see `requirements.txt`; install with `pip install -r requirements.txt`). The `EnvironmentToolset` used by this agent is not yet in a released PyPI version.
- A Google AI API key or Vertex AI credentials

## Quick Start (Standalone)

```bash
# From the repository root:
cp examples/adk_fabric_agent/.env.example examples/adk_fabric_agent/.env
# Edit .env and set GOOGLE_API_KEY

# Interactive mode (no initial task):
cd examples
python -m adk_fabric_agent

# With an initial task via --input:
python -m adk_fabric_agent --input "write a hello world script"
```

The agent starts an interactive session. Type a task and the agent will work through it, using ADK's environment tools (ReadFile, WriteFile, EditFile, Execute) to interact with the workspace and `fabrictool_status` to signal lifecycle events. The `--input` flag sends an initial message before entering the interactive loop.

When running outside a fabric container, `fabrictool` won't be on PATH — the agent works normally but status reporting is silently skipped.

## Container Image

The included `Dockerfile` builds on `fabric-base` (which provides fabrictool, tmux, git, and Python 3):

```bash
docker build -t fabric-adk-agent examples/adk_fabric_agent/
```

The image installs `google-adk` into a virtualenv and copies the agent source to `/opt/adk_fabric_agent/`. The default CMD is `python -m adk_fabric_agent`, which uses a custom runner that supports `--input` for task delivery.

## Deploying via Fabric Template

A ready-to-use template is provided in `templates/adk/`. To deploy this agent in a grove:

```bash
# Copy the template into your grove's .fabric directory
cp -r examples/adk_fabric_agent/templates/adk .fabric/templates/adk

# Copy the harness-config (or place it globally at ~/.fabric/harness-configs/adk/)
cp -r examples/adk_fabric_agent/templates/adk/harness-configs/adk .fabric/harness-configs/adk

# Start an agent using the template
fabric start my-agent --template adk
```

The template uses the **generic** harness with `args` set to `["python", "-m", "adk_fabric_agent"]` and `task_flag: "--input"`. When fabric starts the agent with a task, it appends `--input <task>` to the command. The generic harness passes these as the container command, and fabric wraps it in a tmux session for message delivery.

## Running Inside a Fabric Container

When fabric launches this agent inside a container:

1. **fabrictool** runs as PID 1 and supervises the agent process.
2. The agent writes transient activity updates (`thinking`, `executing`, `idle`) to `$HOME/agent-info.json` via ADK callbacks.
3. Sticky activity transitions (`waiting_for_input`, `blocked`, `completed`, `limits_exceeded`) go through `fabrictool status` which also reports to the fabric Hub.
4. **Message delivery** works natively: `fabric message` sends text via tmux `send-keys` into ADK's `input()` loop.

### Status Lifecycle

```
User sends message
    │
    ▼
thinking          ← before_agent_callback
    │
    ├──► executing    ← before_tool_callback (WriteFile, Execute, etc.)
    │        │
    │        ▼
    │    thinking     ← after_tool_callback
    │        │
    │   (more tools...)
    │
    ▼
idle              ← after_agent_callback

If agent calls fabrictool_status("task_completed", ...):
    → completed (sticky — survives subsequent transient updates)

If agent calls fabrictool_status("ask_user", ...):
    → waiting_for_input (sticky — cleared when user responds)

If agent calls fabrictool_status("blocked", ...):
    → blocked (sticky — cleared when user responds)
```

## Auth Bridging

Fabric's Gemini harness sets `GEMINI_API_KEY`. ADK requires `GOOGLE_API_KEY`. The agent bridges this automatically at import time — if `GOOGLE_API_KEY` is unset but `GEMINI_API_KEY` is available, it copies the value over.

For Vertex AI, set `GOOGLE_GENAI_USE_VERTEXAI=true` and configure Application Default Credentials. See `.env.example` for all options.

## Tools

| Tool | Source | Purpose |
|---|---|---|
| `ReadFile` | EnvironmentToolset | Read file contents from the workspace. |
| `WriteFile` | EnvironmentToolset | Create or overwrite a file in the workspace. |
| `EditFile` | EnvironmentToolset | Make surgical text replacements in an existing file. |
| `Execute` | EnvironmentToolset | Run shell commands in the workspace directory. |
| `fabrictool_status(status_type, message)` | Custom | Signal `ask_user`, `blocked`, `task_completed`, or `limits_exceeded` to fabric. |

## Project Structure

```
adk_fabric_agent/
├── Dockerfile         # Container image (built on fabric-base)
├── __init__.py        # ADK package entry point (exports root_agent)
├── __main__.py        # python -m adk_fabric_agent entrypoint
├── run.py             # Custom runner with --input flag support
├── requirements.txt   # Python dependencies (google-adk>=1.28.0)
├── agent.py           # root_agent definition, auth bridging, model config
├── tools.py           # fabrictool_status tool
├── callbacks.py       # ADK callbacks → fabric activity updates
├── fabrictool.py       # Low-level fabrictool subprocess wrapper
├── .env.example       # Environment variable template
├── README.md          # This file
└── templates/
    └── adk/
        ├── fabric-agent.yaml           # Template definition
        ├── agents.md                  # Agent instructions (fabrictool lifecycle)
        └── harness-configs/
            └── adk/
                └── config.yaml        # Generic harness config (image + args)
```
