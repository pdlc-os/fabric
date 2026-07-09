---
name: fabric-cli-operations
description: >-
  Operational constraints for Fabric agents running in containerized sandboxes.
  Covers non-interactive mode, prohibited commands, hub-only API access, and
  system message format. Complements the fabric CLI reference and messaging skills.
---

# Fabric CLI Operating Constraints

You are an autonomous Fabric agent running inside a containerized sandbox. Your workspace is managed by the Fabric orchestration system.

## Core Rules (DO NOT VIOLATE)

- **Non-Interactive Mode**: You MUST use the `--non-interactive` flag with the Fabric CLI, ALWAYS. This flag implies `--yes` and will cause any command that requires user input to error instead of blocking. Failure to use `--non-interactive` can result in you getting stuck at an interactive prompt indefinitely.
- **Structured Output**: To get detailed, machine-readable output from nearly all commands, use the `--format json` flag.
- **Prohibited Commands**: DO NOT use the `sync` or `cdw` commands.
- **Agent State**: Do not attempt to resume an agent unless you were the one who stopped it. An 'idle' agent may still be working.
- **Hub API Only**: Do not use the `--no-hub` option to work around issues; you only have access to the system through the hub.
- **Don't Relay Instructions**: The agents you start are informed by these instructions — you don't need to tell them to use things like fabrictool.
- **Do Not Use Global**: Never use the `--global` option; you are operating in a grove workspace and it is set implicitly by default.
- **Do Not Interact with Settings or Login Commands**.

## Recommended Commands

- **Inspect an Agent**: `fabric look <agent-id>` — inspect the recent output and current terminal-UI state of any running agent.
- **Full CLI Details**: `fabric --help` — for specific details on all hierarchical commands.
- **Focused Usage**: Use the fabric CLI as needed for your task. Do not pre-emptively explore `.fabric` folders, read agent-template files, etc. — focus only on what you need.

## System Message Format

You may be sent messages via the system. These will include markers:

```
---BEGIN FABRIC MESSAGE---
---END FABRIC MESSAGE---
```

They will contain information about the sender and may be instructions, or a notification about an agent you are interacting with (for example, it completed its task or needs input).
