# Initial GCS Volume Support

## Overview
This document details the implementation of Google Cloud Storage (GCS) volume support for Fabric agents. This feature allows agents to mount GCS buckets directly into their filesystem, enabling persistent storage and data sharing across sessions and agents.

## Changes

### 1. API Updates (`pkg/api/types.go`)
The `VolumeMount` struct has been extended to support GCS-specific fields:
- `Type`: "local" (default) or "gcs".
- `Bucket`: The name of the GCS bucket.
- `Prefix`: The subdirectory within the bucket to mount (optional).
- `Mode`: Mount options (e.g., read-only, specific permissions).

### 2. Runtime Implementation

#### Common Logic (`pkg/runtime/common.go`)
- **Detection**: The runtime now identifies volumes with `Type: "gcs"`.
- **Command Injection**: For GCS volumes, the container entrypoint command is wrapped with `gcsfuse`. This ensures the bucket is mounted before the user's shell or harness starts.
  - Example: `mkdir -p /target && gcsfuse bucket /target && exec <original_command>`
- **Capabilities**: Containers with GCS volumes are granted `SYS_ADMIN` capability and access to `/dev/fuse` to allow FUSE mounting.
- **Metadata**: A `fabric.gcs_volumes` label is added to the container, containing a base64-encoded JSON description of the mounted volumes. This is used for introspection and sync operations.

#### Docker Runtime (`pkg/runtime/docker.go`)
- **Sync Support**: The `Sync` method has been updated to handle GCS volumes. It reads the `fabric.gcs_volumes` label and uses the `pkg/gcp` package to perform sync operations (upload/download) between the local path and the GCS bucket.

### 3. GCP/Rclone Integration (`pkg/gcp/storage.go`)
- **Refactoring**: The `SyncToGCS` and `SyncFromGCS` functions have been completely refactored.
- **Rclone Library**: Instead of using the native Google Cloud Storage Go client directly for recursive operations, we now embed `rclone` as a library.
- **Benefits**: This leverages `rclone`'s mature, robust logic for file syncing, including efficient transfers, retries, and potential for future advanced filtering/exclusion support.
- **Dependencies**: Added `github.com/rclone/rclone` and its dependencies to `go.mod`.

### 4. Base Image (`image-build/fabric-base/Dockerfile`)
- **Dependencies**: The Dockerfile has been updated to install `gcsfuse`, which is required for the FUSE mount inside the container.

## Workflow
1.  **Configuration**: User defines a volume in their agent config with `type: "gcs"`.
2.  **Provisioning**: `fabric` parses the config.
3.  **Execution**:
    *   Docker runtime constructs the `run` command.
    *   It adds `--cap-add SYS_ADMIN` and `--device /dev/fuse`.
    *   It prepends the `gcsfuse` mount command to the agent's startup command.
4.  **Runtime**:
    *   Container starts.
    *   `gcsfuse` mounts the bucket to the target directory.
    *   The agent process starts with access to the mounted data.
5.  **Sync (Optional)**:
    *   User runs `fabric sync`.
    *   Runtime detects GCS volume metadata.
    *   Runtime uses `rclone` (via `pkg/gcp`) to sync changes back to the bucket or pull new changes down.

## Future Considerations
- **Authentication**: Currently relies on Application Default Credentials (ADC) or injected keys. Ensure robust auth flow for `gcsfuse` in all environments.
- **Performance**: `gcsfuse` can have latency. Caching strategies might need tuning.
- **Kubernetes**: This implementation is primarily focused on the Docker runtime. Kubernetes support will require a different approach (e.g., CSI driver or sidecar).
