---
title: Configuration Overview
description: Guide to the Fabric configuration file ecosystem.
---

Fabric uses a multi-layered configuration system to manage orchestrator behavior, agent execution, and server operations.

## Key Configuration Files

| File | Purpose | Scope | Reference |
| :--- | :--- | :--- | :--- |
| `settings.yaml` | **Orchestrator Settings**. Defines profiles, runtimes, and harness configurations. | Global (`~`) or Project (`.fabric`) | [Orchestrator Settings](/fabric/reference/orchestrator-settings/) |
| `fabric-agent.yaml` | **Agent Blueprint**. Defines the configuration for a specific agent or template. | Template or Agent | [Agent Configuration](/fabric/reference/agent-config/) |
| `state.yaml` | **Runtime State**. Tracks system state like sync timestamps. | Project (`.fabric`) | N/A (Managed by Fabric) |

:::note[YAML or JSON]
Fabric accepts both YAML and JSON for `settings` and `fabric-agent` files. YAML is preferred: `fabric init` writes `settings.yaml`, and when several files coexist the loader resolves them in the order `settings.yaml` → `settings.yml` → `settings.json` (the same precedence applies to `fabric-agent.*`). Use whichever format you prefer, but keep the content valid for that format — do not place JSON objects inside a `.yaml` file. Both formats validate against the [settings JSON schema](https://github.com/pdlc-os/fabric/blob/main/pkg/config/schemas/settings-v1.schema.json).
:::

## Server Configuration

Server configuration (for Hub and Runtime Broker) is now integrated into `settings.yaml` under the `server` key.

- [Server Configuration Reference](/fabric/reference/server-config/)

## Telemetry Configuration

Telemetry settings control agent observability — trace collection, cloud forwarding, privacy filtering, and debug output. These are configured via the `telemetry` block in `settings.yaml` and can be overridden per-template or per-agent in `fabric-agent.yaml`.

- [Orchestrator Settings — Telemetry](/fabric/reference/orchestrator-settings/#telemetry-configuration-telemetry)
- [Metrics & OpenTelemetry Guide](/fabric/hosted/single-node/metrics/)

## Project Settings

In a Hub-managed architecture, Project Settings are maintained by the Hub database and managed via the Web Dashboard or API, rather than a local file. These settings define constraints and capabilities for agents operating within a specific project.

Key configuration areas include:

- **General Settings**: The project's description, default branch, and external Git repository URLs for template synchronization.
- **Agent Limits**: Defines maximum resource constraints for agents in the project, including maximum concurrency, runtime duration limits, and maximum workspace storage. These values pre-populate the agent creation form.
- **Resources & Plugins**: Defines authorized Runtime Brokers and configures Message Broker plugins for the project.

When a project is exported or managed locally in a standalone environment, some of these settings may be serialized into `.fabric/state.yaml` or related project files.

## Configuration Hierarchy

Fabric resolves settings in the following order (highest priority first):

1.  **CLI Flags**: (e.g., `fabric start --profile remote`)
2.  **Environment Variables**: `FABRIC_*` overrides.
3.  **Project Settings**: `.fabric/settings.yaml` (Project level).
4.  **Global Settings**: `~/.fabric/settings.yaml` (User level).
5.  **Defaults**: System built-ins.

## Migration

To migrate legacy configuration files to the new schema v1 format:

```bash
# Migrate general settings
fabric config migrate

# Migrate server.yaml to settings.yaml
fabric config migrate --server
```
