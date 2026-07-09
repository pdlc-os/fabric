# Bug: Missing `image` field in FabricConfig

## Issue Description
The `fabric-agent.json` configuration file, which allows defining agent-specific configuration, includes an `image` field in some templates (e.g., `pkg/config/embeds/opencode/fabric-agent.json`). However, the corresponding Go struct `api.FabricConfig` (defined in `pkg/api/types.go`) does not define an `Image` field.

## Consequence
When `fabric-agent.json` is loaded and unmarshaled into `api.FabricConfig` via `LoadConfig` (in `pkg/config/templates.go`), the `image` field in the JSON is ignored/discarded by the Go JSON decoder.

As a result:
1. The container image cannot be defined or overridden at the agent level via `fabric-agent.json`.
2. The agent image resolution falls back to:
   - CLI flags (`--image`).
   - Harness defaults defined in `settings.json` (via `ResolveHarness`).
   - Hardcoded defaults (e.g., "gemini-cli-sandbox").

## Location
- **File**: `pkg/api/types.go`
- **Struct**: `FabricConfig`

## Reproduction
1. Create a `fabric-agent.json` with `"image": "custom-image:latest"`.
2. Start the agent.
3. Observe that `custom-image:latest` is NOT used; the default harness image is used instead.

## Proposed Fix (Deferred)
To support per-agent image configuration via `fabric-agent.json`:
1. Add `Image string` field to `FabricConfig` struct in `pkg/api/types.go` with `json:"image,omitempty"`.
2. Update `MergeFabricConfig` in `pkg/config/templates.go` to handle merging of the `Image` field.
3. Update `AgentManager.Start` in `pkg/agent/run.go` to respect `finalFabricCfg.Image` when resolving the image, giving it precedence over defaults but lower precedence than CLI overrides.
