# Agent Resource Specification

## Summary

Introduce a `resources` block to the agent configuration that allows users to specify compute resource requirements (CPU, memory, disk) for agent containers. The syntax mirrors Kubernetes `resources.requests` and `resources.limits`, providing a familiar and well-understood model. This common configuration is translated to runtime-specific arguments depending on the target runtime (Apple Container, Docker, or Kubernetes).

## Motivation

Resource allocation is currently hardcoded per runtime:
- **Apple Container**: fixed at 2G memory, no CPU control (`apple_container.go:37`)
- **Docker**: no resource constraints applied at all (`common.go` / `docker.go`)
- **Kubernetes**: `K8sResources` struct exists in `api/types.go` but is never applied in `buildPod()` (`k8s_runtime.go:250-263`)

Users need the ability to control resource allocation to:
- Prevent agents from consuming excessive host resources
- Ensure agents have enough memory for large codebases or model operations
- Right-size Kubernetes pods for cost and scheduling efficiency
- Maintain consistent resource expectations across runtimes

## Configuration Schema

### Resource Block

A new `resources` field is added to `FabricConfig`:

```yaml
# In fabric-agent.yaml, settings.yaml profile, or template config
resources:
  requests:
    memory: "2Gi"
    cpu: "2"
  limits:
    memory: "8Gi"
    cpu: "4"
  disk: "20Gi"
```

### Semantics

The schema follows Kubernetes conventions:

| Field | Description |
|---|---|
| `resources.requests.memory` | Minimum memory the agent needs. Used as the guaranteed allocation on runtimes that don't distinguish requests from limits. |
| `resources.requests.cpu` | Minimum CPU cores. Whole numbers or decimal (e.g., `"0.5"`, `"2"`). |
| `resources.limits.memory` | Maximum memory the agent can consume. Hard cap enforced by the runtime. |
| `resources.limits.cpu` | Maximum CPU cores available. |
| `resources.disk` | Disk/volume size for the agent's primary working storage. Applied where the runtime supports it (e.g., Apple Container volume sizing). |

**Quantity format**: Values use Kubernetes-style quantity strings: plain integers, decimal, or with suffixes `Ki`, `Mi`, `Gi`, `Ti` (binary) and `K`, `M`, `G`, `T` (decimal). CPU values are in cores (e.g., `"4"`, `"0.5"`) — Kubernetes millicore notation (`500m`) is also accepted and normalized to decimal cores for non-Kubernetes runtimes.

### Defaults

When no `resources` block is provided, runtimes apply their own defaults:

| Runtime | Default Memory | Default CPU |
|---|---|---|
| Apple Container | `2G` (current behavior) | `4` (apple-container tool default) |
| Docker | Unlimited (Docker default) | Unlimited (Docker default) |
| Kubernetes | None (scheduler default) | None (scheduler default) |

## Go Types

```go
// In pkg/api/types.go

// ResourceSpec defines compute resource requirements for an agent container.
// It follows Kubernetes resource model conventions.
type ResourceSpec struct {
    Requests ResourceList `json:"requests,omitempty" yaml:"requests,omitempty"`
    Limits   ResourceList `json:"limits,omitempty" yaml:"limits,omitempty"`
    Disk     string       `json:"disk,omitempty" yaml:"disk,omitempty"`
}

// ResourceList is a set of resource name/quantity pairs.
type ResourceList struct {
    CPU    string `json:"cpu,omitempty" yaml:"cpu,omitempty"`
    Memory string `json:"memory,omitempty" yaml:"memory,omitempty"`
}
```

`FabricConfig` gains a new field:

```go
type FabricConfig struct {
    // ... existing fields ...
    Resources  *ResourceSpec     `json:"resources,omitempty" yaml:"resources,omitempty"`
}
```

The existing `K8sResources` struct (with `map[string]string` fields) is retained for backward compatibility in the `KubernetesConfig` block. When both `FabricConfig.Resources` and `KubernetesConfig.Resources` are set, the Kubernetes-specific config takes precedence for Kubernetes deployments (it may include GPU or other extended resources).

### RunConfig Propagation

`RunConfig` gains a `Resources` field so runtimes can access the resolved spec:

```go
type RunConfig struct {
    // ... existing fields ...
    Resources  *api.ResourceSpec
}
```

## Runtime Translation

### Apple Container (`container` CLI)

The `container run` command accepts:
- `-c, --cpus <cpus>` — number of CPUs (default: 4)
- `-m, --memory <memory>` — memory with K/M/G/T/P suffix (default: 1 GiB)

**Translation rules:**
- `limits.memory` → `-m` flag. If only `requests.memory` is set, use that instead.
- `limits.cpu` → `-c` flag. If only `requests.cpu` is set, use that instead.
- Apple Container does not distinguish requests from limits — the VM gets a fixed allocation. Use the *limit* when available (gives the agent the ceiling it may need), falling back to the *request*.
- `disk` → not directly applicable to `container run`, but could be used for volume creation sizing in the future.

**Example output:**
```
container run -d -t -m 8G -c 4 --name my-agent ...
```

**Current code change** (`apple_container.go:37`): Replace the hardcoded `"-m", "2G"` with values derived from `config.Resources`, falling back to `"-m", "2G"` when unset.

### Docker

Docker `run` accepts:
- `--memory` / `-m` — memory limit (e.g., `8g`, `512m`)
- `--cpus` — CPU limit as decimal (e.g., `4`, `0.5`)
- `--memory-reservation` — soft memory limit (maps to requests)

**Translation rules:**
- `limits.memory` → `--memory`
- `requests.memory` → `--memory-reservation`
- `limits.cpu` → `--cpus`
- `requests.cpu` → not directly mapped (Docker doesn't have a CPU reservation equivalent in simple form; could use `--cpu-shares` but that adds complexity for minimal gain). Omit for now.
- `disk` → not applicable to Docker (volumes inherit host filesystem).

**Example output:**
```
docker run -d -t --memory 8g --memory-reservation 2g --cpus 4 --name my-agent ...
```

**Code change**: Add resource args in `buildCommonRunArgs()` or in the Docker/Apple runtime `Run()` methods after constructing base args. Placing it in the runtime-specific `Run()` method is cleaner since the flag formats differ slightly between Docker and Apple Container.

### Kubernetes

Kubernetes natively supports the requests/limits model on `container.resources`.

**Translation rules:**
- `requests.memory` → `container.resources.requests["memory"]`
- `requests.cpu` → `container.resources.requests["cpu"]`
- `limits.memory` → `container.resources.limits["memory"]`
- `limits.cpu` → `container.resources.limits["cpu"]`
- `disk` → `container.resources.requests["ephemeral-storage"]` (if the cluster supports it)

If `KubernetesConfig.Resources` is also set (the existing `K8sResources` map), those values are merged on top, allowing Kubernetes-specific resources like `nvidia.com/gpu` to be specified without polluting the common schema.

**Code change** (`k8s_runtime.go`, `buildPod()`): Set `corev1.Container.Resources` from the resolved resource spec.

## Configuration Hierarchy and Merging

Resources participate in the existing configuration merge hierarchy:

1. **Template** `fabric-agent.yaml` — base defaults per harness type
2. **Settings profile** — environment-specific overrides
3. **Agent-level** `fabric-agent.yaml` — per-agent overrides
4. **CLI flags** (future) — ad-hoc override at start time

Merging is field-level: a higher-priority layer setting `limits.memory` overrides only that field, not the entire `resources` block.

### Settings Integration

Resources can be set at the profile level or as harness overrides:

```yaml
# settings.yaml
profiles:
  local:
    runtime: container
    resources:
      requests:
        memory: "4Gi"
        cpu: "2"
      limits:
        memory: "8Gi"
        cpu: "4"
  remote:
    runtime: kubernetes
    resources:
      requests:
        memory: "2Gi"
        cpu: "1"
      limits:
        memory: "4Gi"
        cpu: "2"
    harness_overrides:
      claude:
        resources:
          limits:
            memory: "16Gi"
```

This requires adding a `Resources *api.ResourceSpec` field to `ProfileConfig` and `HarnessOverride`.

## CLI Flags (Future)

For ad-hoc overrides at start time:

```
fabric start my-agent --memory 8Gi --cpus 4
```

These map to `limits.memory` and `limits.cpu` respectively, since they represent the user asking for a specific allocation cap. This is a future enhancement and not required for the initial implementation.

## Quantity Parsing

A utility function normalizes resource quantities across runtimes:

```go
// pkg/util/resources.go

// ParseMemory parses a Kubernetes-style memory quantity and returns bytes.
// Accepts: "512Mi", "2Gi", "1G", "2048M", "1073741824"
func ParseMemory(s string) (int64, error) { ... }

// FormatMemoryForDocker formats bytes as a Docker-compatible memory string.
// Returns values like "512m", "2g", "1073741824"
func FormatMemoryForDocker(bytes int64) string { ... }

// FormatMemoryForApple formats bytes as an Apple Container-compatible memory string.
// Returns values like "512M", "2G"
func FormatMemoryForApple(bytes int64) string { ... }

// ParseCPU parses a CPU quantity. Accepts "4", "0.5", "500m".
// Returns the value as a float64 number of cores.
func ParseCPU(s string) (float64, error) { ... }
```

## Implementation Scope

### Phase 1 (This PR)
1. Add `ResourceSpec` and `ResourceList` types to `pkg/api/types.go`
2. Add `Resources` field to `FabricConfig` and `RunConfig`
3. Add quantity parsing utilities in `pkg/util/resources.go`
4. Apply resources in Apple Container runtime (replace hardcoded `2G`)
5. Apply resources in Docker runtime (`--memory`, `--cpus`)
6. Apply resources in Kubernetes runtime (`buildPod` container resources)
7. Add `Resources` field to `ProfileConfig` and `HarnessOverride` for settings-level configuration
8. Implement merge logic in settings/config merging
9. Unit tests for quantity parsing and runtime argument generation

### Phase 2 (Future)
- CLI flags (`--memory`, `--cpus`) on `fabric start`
- `disk` field support for Apple Container volume sizing
- Validation warnings (e.g., request > limit, unreasonably small values)
- Hub API support for resource specs in hosted mode
- Dashboard display of resource allocation and utilization

## Files Modified

| File | Change |
|---|---|
| `pkg/api/types.go` | Add `ResourceSpec`, `ResourceList`; add `Resources` to `FabricConfig` |
| `pkg/runtime/interface.go` | Add `Resources` to `RunConfig` |
| `pkg/runtime/apple_container.go` | Use `Resources` for `-m` and `-c` flags, fall back to current defaults |
| `pkg/runtime/docker.go` | Add `--memory`, `--memory-reservation`, `--cpus` from `Resources` |
| `pkg/runtime/k8s_runtime.go` | Set `corev1.Container.Resources` in `buildPod()` |
| `pkg/config/settings.go` | Add `Resources` to `ProfileConfig`, `HarnessOverride`; merge logic |
| `pkg/agent/run.go` | Pass `Resources` from `FabricConfig` into `RunConfig` |
| `pkg/util/resources.go` | New file: quantity parsing and formatting utilities |
| `pkg/util/resources_test.go` | New file: tests for quantity parsing |
