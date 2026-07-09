# GCP Authentication for Agent Telemetry Export

**Status:** Draft — Reviewed
**Date:** 2026-03-03
**Parent Document:** [Hosted Fabric Metrics System Design](metrics-system.md)

---

## 1. Context

The [Metrics System Design](metrics-system.md) establishes Google Cloud Observability (Cloud Trace, Cloud Logging, Cloud Monitoring) as the primary telemetry backend (§11.1). Section 11.8 deferred credential injection as out-of-scope, noting that the system would assume the "application default credentials" (ADC) pattern and leave it to the Runtime Broker to make credentials available.

This document supersedes §11.8 by defining a concrete, GCP-specific credential injection strategy for agent telemetry. While the overall OTEL pipeline remains vendor-agnostic, authenticating to GCP endpoints requires a targeted solution that:

1. Provides a hub-managed, least-privilege service account key dedicated to telemetry writes.
2. Injects the key into agent containers during provisioning.
3. Uses a **fabric-specific environment variable** (not `GOOGLE_APPLICATION_CREDENTIALS`) so the credential is consumed exclusively by the fabrictool OTEL exporter and is not inadvertently discovered by other Google client libraries running in the container.

---

## 2. Design Principles

1. **Telemetry-Scoped Credential**: The injected key must only be usable for writing telemetry data. It must not grant access to any other GCP resources.
2. **Collector-Only Visibility**: The credential must not be surfaced through `GOOGLE_APPLICATION_CREDENTIALS` or placed in the ADC well-known location (`~/.config/gcloud/application_default_credentials.json`), preventing any other Google SDK in the container from discovering it.
3. **Hub as Credential Authority**: The Hub (or its backing secrets store) is the single source of truth for the telemetry service account key. Brokers do not manage their own keys.
4. **Leverage Existing Infrastructure**: File injection and environment variable mechanisms already exist in the provisioning pipeline. This design should use them rather than introduce new projection channels.

---

## 3. GCP IAM Configuration

### 3.1 Dedicated Service Account

A GCP service account is created specifically for telemetry ingestion:

```
fabric-telemetry-writer@<project-id>.iam.gserviceaccount.com
```

### 3.2 Minimum Required IAM Roles

The service account is granted only the following predefined roles at the project level:

| Role | Purpose |
|------|---------|
| `roles/cloudtrace.agent` | Write trace spans to Cloud Trace |
| `roles/logging.logWriter` | Write log entries to Cloud Logging |
| `roles/monitoring.metricWriter` | Write custom metrics to Cloud Monitoring |

No other roles should be granted. The account must not have `roles/editor`, `roles/owner`, or any data-read permissions.

### 3.3 Key Management

A JSON key file is generated for this service account and stored as a Hub-managed secret. Key rotation is handled manually by an administrator (see §7.1).

---

## 4. Credential Injection Flow

### 4.1 Overview

```
┌──────────────┐       ┌───────────────────┐       ┌───────────────────────────────┐
│   Fabric Hub  │       │  Runtime Broker    │       │     Agent Container           │
│              │       │                    │       │                               │
│  Stores SA   │──────▶│  Receives key in   │──────▶│  Key written to agent-home:   │
│  key as a    │ Agent │  CreateAgent       │ File  │  ~/.fabric/telemetry-gcp-     │
│  secret      │ Dispatch  dispatch payload │ Inject│  credentials.json            │
│              │       │                    │       │                               │
│  Attaches    │       │  Stages file in    │ Env   │  FABRIC_OTEL_GCP_CREDENTIALS  │
│  to grove    │       │  agent-home dir    │ Set   │  set to file path            │
│  config      │       │                    │       │                               │
└──────────────┘       └───────────────────┘       └───────────────────────────────┘
```

### 4.2 Hub: Secret Storage

The telemetry SA key is stored using the existing secrets system ([Secrets Design](secrets.md)) as a **file-type secret** scoped to the hub:

| Field | Value |
|-------|-------|
| **Name** | `fabric-telemetry-gcp-credentials` |
| **Type** | `file` |
| **Target** | `~/.fabric/telemetry-gcp-credentials.json` |
| **Scope** | `hub` |
| **Value** | Base64-encoded service account JSON key |

Hub-scoped secrets are resolved and included for all groves and brokers served by that hub instance. When telemetry is enabled for a grove with the `gcp` cloud backend, the Hub includes this secret in the `ResolvedSecrets` list within the `CreateAgent` dispatch payload sent to the broker.

### 4.3 Broker: File Staging

The Runtime Broker receives the secret through the standard `CreateAgent` dispatch flow and stages it using the existing `writeFileSecrets` mechanism (`pkg/runtime/common.go`):

1. The base64-encoded key content is decoded and written to the host-side staging directory: `<agent-dir>/secrets/fabric-telemetry-gcp-credentials`
2. The file is bind-mounted read-only into the container at `~/.fabric/telemetry-gcp-credentials.json`
3. File permissions are set to `0600` (owner-read only).

No new file-staging code is required; the existing secrets projection pipeline handles this.

### 4.4 Broker: Environment Variable Injection

In addition to staging the file, the broker sets a fabric-specific environment variable to inform fabrictool where to find the credential:

```
FABRIC_OTEL_GCP_CREDENTIALS=/home/<user>/.fabric/telemetry-gcp-credentials.json
```
This variable is set alongside the existing telemetry env vars emitted by `TelemetryConfigToEnv()` in `pkg/config/telemetry_convert.go`.

**Why not `GOOGLE_APPLICATION_CREDENTIALS`?**

Setting `GOOGLE_APPLICATION_CREDENTIALS` would cause every Google Cloud client library in the container to discover and use this credential. Agent containers may run tools (e.g., `gcloud`, `gsutil`, GCS FUSE, or application code using Google SDKs) that should authenticate with the agent's own identity — not the telemetry writer. A fabric-namespaced variable ensures only the fabrictool exporter consumes this credential.

---

## 5. Fabrictool Credential Consumption

### 5.1 OTEL Exporter Configuration

The fabrictool telemetry pipeline (`pkg/fabrictool/telemetry/pipeline.go`) must be updated to:

1. Check for `FABRIC_OTEL_GCP_CREDENTIALS` at startup.
2. If present, read the JSON key file and construct a `google.Credentials` object from it using `google.CredentialsFromJSON()`.
3. Pass the credential explicitly to the GCP OTEL exporter via `option.WithCredentials()`, rather than relying on ADC auto-discovery.

```go
// Pseudocode for credential loading
if credPath := os.Getenv("FABRIC_OTEL_GCP_CREDENTIALS"); credPath != "" {
    keyBytes, err := os.ReadFile(credPath)
    if err != nil {
        return fmt.Errorf("failed to read GCP telemetry credentials: %w", err)
    }

    creds, err := google.CredentialsFromJSON(ctx, keyBytes,
        "https://www.googleapis.com/auth/trace.append",
        "https://www.googleapis.com/auth/logging.write",
        "https://www.googleapis.com/auth/monitoring.write",
    )
    if err != nil {
        return fmt.Errorf("failed to parse GCP telemetry credentials: %w", err)
    }

    // Pass creds to each GCP exporter
    traceExporter, _ := cloudtrace.New(cloudtrace.WithTraceClientOptions(
        option.WithCredentials(creds),
    ))
    // ... similarly for logging and monitoring exporters
}
```

### 5.2 Fallback Behavior

If `FABRIC_OTEL_GCP_CREDENTIALS` is not set, fabrictool falls back to the standard ADC chain. This preserves backward compatibility for environments where Workload Identity, metadata server credentials, or a pre-existing `GOOGLE_APPLICATION_CREDENTIALS` is available (e.g., GKE pods with Workload Identity Federation).

### 5.3 Credential Caching

The `google.Credentials` object handles token refresh internally. Fabrictool should create the credential once at startup and reuse it for all exporters (traces, logs, metrics). No additional caching is needed.

---

## 6. Configuration Surface

### 6.1 TelemetryCloudConfig Extension

The `TelemetryCloudConfig` struct (`pkg/api/types.go`) gains an optional field to signal that GCP-specific authentication is configured:

```go
type TelemetryCloudConfig struct {
    Enabled  *bool             `json:"enabled,omitempty" yaml:"enabled,omitempty"`
    Endpoint string            `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
    Protocol string            `json:"protocol,omitempty" yaml:"protocol,omitempty"`
    Headers  map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
    TLS      *TelemetryTLS     `json:"tls,omitempty" yaml:"tls,omitempty"`
    Batch    *TelemetryBatch   `json:"batch,omitempty" yaml:"batch,omitempty"`
    // Provider identifies the cloud backend type (e.g., "gcp", "generic").
    // When set to "gcp", the provisioning pipeline will inject the
    // telemetry SA key if available.
    Provider string            `json:"provider,omitempty" yaml:"provider,omitempty"`
}
```

### 6.2 Environment Variable Emission

`TelemetryConfigToEnv()` is extended to emit:

| Condition | Variable | Value |
|-----------|----------|-------|
| `Cloud.Provider == "gcp"` | `FABRIC_TELEMETRY_CLOUD_PROVIDER` | `gcp` |

The `FABRIC_OTEL_GCP_CREDENTIALS` variable itself is not emitted by `TelemetryConfigToEnv()` — it is set by the broker's secret-injection path based on the presence of the resolved secret. This keeps the telemetry config conversion stateless (it doesn't need to know about file paths).

### 6.3 Grove Settings Example

```yaml
telemetry:
  enabled: true
  cloud:
    enabled: true
    provider: gcp
  hub:
    enabled: true
```

When `provider: gcp` is set and the `fabric-telemetry-gcp-credentials` secret exists at hub scope, the full injection chain activates automatically during agent provisioning.

---

## 7. Design Decisions

### 7.1 Key Rotation Strategy

**Decision:** Manual rotation. An administrator generates a new key, updates the Hub secret, and restarts affected agents. Automated rotation can be added as a secrets-system enhancement in the future without changing this design.

### 7.2 Per-Grove vs. Shared Key

**Decision:** A single shared key per Hub instance. The key is scoped at whatever level the secret resolves to (initially `hub` scope). The GCP resource labels attached to telemetry data (grove name, agent name) already provide per-grove attribution. Per-grove keys can be introduced later by changing the secret scope without modifying the injection mechanism.

### 7.3 Secret Scope

**Decision:** `hub` scope. The telemetry credential is initially stored as a hub-scoped secret, making it available to all groves and brokers served by that hub. Hub-scoped secrets are being implemented separately as a general secrets-system feature. The injection mechanism is scope-agnostic — if the secret scope is later narrowed to `grove` or `runtime_broker`, no changes to the injection pipeline are required.

### 7.4 Workload Identity Federation as Alternative

**Decision:** Key-file injection is the universal baseline. For GKE or Cloud Run environments where Workload Identity Federation (WIF) is available, the fabrictool fallback-to-ADC behavior (§5.2) already provides a working path — no key injection is needed when ambient credentials are present. A dedicated `provider: gcp-wif` variant can be added in the future if explicit opt-in is desired.

### 7.5 Key File Path Convention

**Decision:** `~/.fabric/telemetry-gcp-credentials.json`. This path is outside `~/.config/gcloud/` (avoiding ADC discovery), within the `.fabric` namespace for consistency, and does not collide with existing fabric-managed files.

### 7.6 Multiple Cloud Providers

The `FABRIC_OTEL_GCP_CREDENTIALS` variable is GCP-specific by name and only consumed by the GCP exporter codepath. A second provider (e.g., AWS) would use its own namespaced variable (e.g., `FABRIC_OTEL_AWS_CREDENTIALS`) and its own injected credential, without conflict.

### 7.7 Credential File Permissions in Apple Runtime — Open

**Status:** Open — requires verification.

The Apple Virtualization Framework runtime uses a file-copy mechanism (`secret-map.json`) rather than bind-mounts. The `writeSecretMap` path in `pkg/runtime/common.go` writes a manifest that the in-VM agent copies from a shared volume. It must be verified that the copy step enforces `0600` permissions on the credential file inside the VM, or an explicit `chmod 0600` should be added.

---

## 8. Security Considerations

### 8.1 Blast Radius

The SA key can only write telemetry data. Even if compromised, an attacker can only:
- Write spurious trace spans, log entries, or metrics to the project.
- They cannot read existing telemetry, access other GCP resources, or escalate privileges.

Volume-based abuse (writing excessive data) can be mitigated with GCP quotas and billing alerts.

### 8.2 Key Exposure Surface

The key exists in three locations:
1. **Hub secrets store** (encrypted at rest via the secrets backend, e.g., GCP Secret Manager).
2. **Broker staging directory** (ephemeral, per-agent, filesystem permissions restricted).
3. **Agent container filesystem** (bind-mounted read-only or copied with `0600` perms).

The key is never transmitted to the agent process as an environment variable value — only the *path* to the file is set in the environment.

### 8.3 Container Escape

If an agent process escapes the container, it could read the key file from the host staging directory. This is an existing risk for all file-type secrets and is mitigated by:
- Running containers with minimal capabilities.
- Using rootless container runtimes where available.
- The key's limited permissions (telemetry-write only) bounding the impact.

---

## 9. Implementation Checklist

1. **GCP Setup**: Create `fabric-telemetry-writer` service account with roles from §3.2. Generate JSON key.
2. **Hub Secret**: Store the key as a `file`-type secret named `fabric-telemetry-gcp-credentials` at hub scope.
3. **API Types**: Add `Provider` field to `TelemetryCloudConfig` (§6.1).
4. **Config Conversion**: Emit `FABRIC_TELEMETRY_CLOUD_PROVIDER` from `TelemetryConfigToEnv()` (§6.2).
5. **Provisioning**: Ensure `CreateAgent` dispatch includes the telemetry secret when `provider: gcp` and the secret exists. Set `FABRIC_OTEL_GCP_CREDENTIALS` env var pointing to the mounted path.
6. **Fabrictool**: Update OTEL provider setup to load explicit credentials from `FABRIC_OTEL_GCP_CREDENTIALS` (§5.1).
7. **Tests**: Unit tests for credential loading, env var emission, and fallback-to-ADC behavior.
8. **Documentation**: Update operator runbook with GCP setup instructions and key rotation procedure.
9. **Demo Scripts**: Update the demo-hub GCE provisioning scripts in `.hack/` to include telemetry SA key setup and injection configuration.