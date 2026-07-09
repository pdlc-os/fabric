---
title: Multi-Broker Setup
description: Connect multiple machines to a single Fabric Hub for distributed agent execution.
---

## Overview

A single Fabric Hub can dispatch agents to **multiple Runtime Brokers**. Each broker is a machine — a laptop, cloud VM, or Kubernetes cluster — that runs agent containers. This lets teams pool compute resources and target specific machines for specific workloads.

## Architecture

```
                    ┌──────────┐
       ┌────────────┤ Fabric Hub├────────────┐
       │            └────┬─────┘            │
       │                 │                  │
  ┌────▼─────┐    ┌──────▼───┐    ┌────────▼──────┐
  │ Broker A  │    │ Broker B │    │   Broker C    │
  │ (laptop)  │    │(cloud VM)│    │ (K8s cluster) │
  └───────────┘    └──────────┘    └───────────────┘
```

Each broker maintains a persistent WebSocket connection to the Hub. The Hub acts as the control plane; brokers handle container execution locally.

## Adding a Broker

On each machine you want to register:

1. **Install Fabric** and configure the Hub endpoint (`fabric login`).
2. **Register the broker** with the Hub:
   ```bash
   fabric broker register
   ```
3. **Authorize projects** the broker should serve:
   ```bash
   fabric broker provide <project>
   ```

Repeat for each machine. See [Runtime Broker](/fabric/hosted/ha/runtime-broker/) for detailed setup.

## Broker Selection

When starting an agent, the Hub selects an available broker automatically. You can override this:

- **Target a specific broker** with the `--broker` flag:
  ```bash
  fabric start --broker my-cloud-vm
  ```
- **Check broker availability** across all registered brokers:
  ```bash
  fabric broker status
  ```

## Considerations

- Each broker manages its own **port pools, container images, and local storage**. Images must be available on each broker independently.
- **Shared directories** (mounted volumes) only work within a single broker — agents on different brokers cannot share a local directory.
- **Workspace strategy** may differ per broker: local brokers typically use git worktrees (`.fabric_worktrees/`), while hub-hosted git projects use a single workspace checkout.
- Broker capacity is determined by the machine's resources. The Hub does not enforce cross-broker resource limits.
