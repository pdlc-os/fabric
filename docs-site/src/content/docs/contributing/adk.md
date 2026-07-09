---
title: Agent Development Kit (ADK)
description: Developing custom autonomous agents with the Fabric ADK.
---

Fabric provides an Agent Development Kit (ADK) to facilitate the creation and integration of custom autonomous agents within the Fabric ecosystem.

## Overview

The ADK streamlines the process of building agents that don't rely on the standard pre-packaged harnesses (like Gemini CLI or Claude Code), but instead use custom logic or alternative frameworks, while still benefiting from Fabric's orchestration, workspace management, and observability features.

## ADK Runner Entrypoint

Fabric includes a specialized runner entrypoint designed specifically for ADK agents. This entrypoint provides native support for the `--input` flag, facilitating more robust automated execution and easier testing of agent behaviors.

## Example Project

To get started quickly, Fabric provides a complete example and Docker template located in the `examples/adk_fabric_agent/` directory of the repository.

This example demonstrates:
- How to structure a custom agent.
- How to build a compatible Docker image.
- How to define the corresponding Fabric template (`fabric-agent.yaml`).
- How to handle inputs and interact with the Fabric environment.

## Integration Points

When building an ADK agent, you will primarily interact with Fabric through:
1.  **Environment Variables**: Fabric injects configuration and context via environment variables (e.g., `FABRIC_AGENT_ID`, `FABRIC_WORKSPACE`).
2.  **Workspace Mount**: The designated workspace directory where your agent should perform its file operations.
3.  **Standard IO/fabrictool**: Using the `fabrictool` utility (injected into the container) to report status (`fabrictool status`) and log structured messages.
