# Volume and Synchronization Approach Survey

This document provides a comprehensive survey and summary of the various approaches to volume management and data synchronization in the Fabric orchestration platform, as documented across several design specifications.

---

## 1. Core Synchronization Paradigms

Fabric employs three distinct paradigms for managing files and volumes, depending on the architecture (Solo vs. Hosted) and the lifecycle of the data.

### 1.1. Local Workspace Management (Solo Mode)
In local execution, Fabric prioritizes **isolation via Git Worktrees**.
- **Mechanism**: A new git worktree is created for each agent in a dedicated `.fabric_worktrees/` directory.
- **Mounting**: The worktree is bind-mounted into the container at `/workspace`. For git-aware harnesses, the parent repository's `.git` directory is also mounted to provide full repository context without duplication.
- **Reference**: `fabric.md`, `git-ws.md`.

### 1.2. On-Demand Workspace Sync (Hosted Relay)
In the Hosted architecture, where CLI and Runtime Broker are decoupled, Fabric uses **GCS as a synchronization relay**.
- **Mechanism**: The CLI and Runtime Broker synchronize local workspace state to/from a central GCS bucket.
- **Command**: `fabric sync to/from <agent>`.
- **Flow**:
    1. **Hub as Coordinator**: The Hub generates short-lived **Signed URLs** for direct CLI ↔ GCS and Broker ↔ GCS transfer. The Hub never touches the file content.
    2. **Incremental Sync**: Files are tracked via SHA-256 content hashes in a `manifest.json`. Only new or modified files are transferred.
    3. **Rclone Integration**: The Runtime Broker uses an embedded `rclone` library to perform efficient, multi-threaded synchronization between the agent's filesystem and GCS.
- **Reference**: `hosted/sync-design.md`, `walkthroughs/hosted-e2e.md`.

### 1.3. Persistent GCS Volumes (FUSE)
For persistent storage that survives agent lifecycle or is shared across agents, Fabric supports **native GCS FUSE mounts**.
- **Mechanism**: The `gcsfuse` utility is used inside the container to mount a GCS bucket (or prefix) directly as a filesystem.
- **Configuration**: Defined in `fabric-agent.json` via the `volumes` block with `type: "gcs"`.
- **Runtime Requirements**: Containers require `SYS_ADMIN` capabilities and access to `/dev/fuse`.
- **Reference**: `initial-gcs-volume-support.md`.

---

## 2. Workspace Bootstrap (Initialization)

A critical sub-problem is how an agent's workspace is initially populated when created on a remote broker.

| Strategy | Applicable To | Mechanism | Status |
|----------|---------------|-----------|--------|
| **GCS Bootstrap** | Non-Git Groves | CLI uploads workspace to GCS during `provisioning` phase; Broker downloads before container start. | Implemented |
| **Git Bootstrap** | Git-Backed Groves | Broker clones/fetches directly from the Git Remote (origin) into a managed worktree. | Proposed/Deferred |
| **Local Path** | Solo / Linked Groves | Broker uses a pre-existing local directory registered for the Grove. | Implemented |

**Reference**: `hosted/sync-design.md` (Section 13).

---

## 3. Storage Hierarchy and Conventions

All cloud-stored resources follow a unified naming convention in the Hub's GCS bucket.

```
gs://fabric-hub-{env}/
├── templates/             # Base agent definitions
│   └── {scope}/{slug}/
├── harness-configs/       # Harness base layers (home/ files)
│   └── {scope}/{slug}/
└── workspaces/            # Agent workspace relay
    └── {groveId}/
        └── {agentId}/
            ├── manifest.json
            └── files/     # Actual file content
```

**Reference**: `hosted/harness-config-hub-storage.md`, `hosted/sync-design.md`.

---

## 4. Supporting Technologies

### 4.1. Signed URLs
The foundational security and performance mechanism for Hosted mode. It allows high-bandwidth data transfers to bypass the Hub server entirely, reducing Hub CPU/memory pressure and eliminating the Hub as a bandwidth bottleneck.

### 4.2. Unified Transfer Package (`pkg/transfer`)
To maximize code reuse, a common transfer package handles the logic for:
- Collecting local files and calculating hashes.
- Building and verifying `manifest.json`.
- Uploading/Downloading batches of files via Signed URLs.
- Used by: Templates, Harness-Configs, and Workspace Sync.

### 4.3. Rclone
Embedded as a Go library, `rclone` provides the "heavy lifting" for the Runtime Broker. It handles retries, parallel transfers, and the core synchronization logic that makes the `sync` command robust.

### 4.4. Content Hashing (Deduplication)
Every file in a managed volume or workspace is identified by its SHA-256 hash. This allows the system to skip uploading/downloading files that already exist in the target storage, making synchronization "incremental" by default.

---

## 5. Volume Configuration Summary

| Feature | Local Bind | GCS FUSE | Workspace Sync |
|---------|------------|----------|----------------|
| **Latency** | Extremely Low | High (Network) | None (Local after sync) |
| **Persistence**| Host-managed | GCS-backed | Relay-backed |
| **Hosted Support**| No | Yes | Yes (Primary) |
| **Tooling** | Git Worktree | gcsfuse | rclone + pkg/transfer |
| **Typical Use** | Local Dev | Shared Data / Large Datasets | Distributed Code Agents |

---

## 6. Open Items and Gaps
- **Git Bootstrap in K8s**: The design for init-container git clones exists but implementation is pending.
- **Git-Aware Sync**: Current workspace sync is purely file-based; it does not preserve `.git` metadata unless explicitly included (usually excluded by default).
- **Credential Management**: Managing SSH/HTTPS credentials for remote git clones remains a complex area with multiple proposed but unimplemented strategies.
