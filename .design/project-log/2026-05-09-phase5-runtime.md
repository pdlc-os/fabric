# Phase 5: Container Labels and Runtime Discovery Transition

Date: 2026-05-09
Agent: developer

## Overview

Implemented Phase 5 transition for container labels and runtime discovery. This involves emitting both `fabric.project*` and `fabric.grove*` labels/annotations for backward compatibility, and updating discovery logic to prefer the new project variants while falling back to grove variants.

## Changes

### Container Label Emission
- **pkg/agent/run.go**: Updated `RunConfig` generation to include:
  - Labels: `fabric.project`, `fabric.project_id` (alongside `fabric.grove`, `fabric.grove_id`).
  - Annotations: `fabric.project_path` (alongside `fabric.grove_path`).
- **cmd/server_dispatcher.go**: Updated `hubAgent` label setting to include `fabric.project`.

### Discovery and Filtering Logic
- **pkg/agent/list.go**: Updated `List` to handle `fabric.project` and `fabric.project_path` in filters.
- **pkg/agent/manager.go**: Updated `MessageRaw` and `deliverImmediate` to use `fabric.project_id` in filters.
- **pkg/runtimebroker/handlers.go**:
  - Updated `matchesAgent` to check both `fabric.project_id` and `fabric.grove_id`.
  - Updated `listAgents` to support `fabric.project_id` filter.
  - Updated `agentKey` for deduping to use project ID.
  - Updated `resolveManagerForAgent` and `resolveRuntimeForAgent` to use `fabric.project_id` in filters.
- **pkg/runtime/docker.go**:
  - Updated `List` to populate `Project`, `ProjectID`, and `ProjectPath` from both label variants.
  - Updated filtering logic to support fallback from project keys to grove keys.
- **pkg/runtime/k8s_runtime.go**:
  - Updated `List` to populate `Project`, `ProjectID`, and `ProjectPath` from both label variants.
  - Updated `List` selector translation to use grove variants for the K8s API call (ensuring old and new pods are found).
  - Updated `createSharedDirPVCs` to emit both project and grove labels on PVCs.

### Command Line Interface
- Updated `attach`, `delete`, `list`, `message`, `stop`, and `suspend` commands to use `fabric.project` and `fabric.project_path` when filtering agents.

### State and Provisioning
- **pkg/agent/provision.go**: Updated `ProvisionAgent` to populate `ProjectID` and `ProjectPath` in the `AgentInfo` written to `agent-info.json`.
- **pkg/agent/provision.go**: Updated `StopProjectContainers` to use `fabric.project` for filtering.

## Verification Results

- Verified that all modified files contain the new label variants.
- Verified that discovery logic prefers project labels but successfully falls back to grove labels.
- Build and basic tests for modified packages pass.
