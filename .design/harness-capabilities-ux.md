# Harness Capabilities for Advanced Config UX

## Goal
Provide a harness-aware capability map for advanced agent configuration so the web Configure page can disable unsupported options, show explanatory tooltips, and prevent invalid saves.

The user experience target is:
- Do not hide fields.
- Disable unsupported fields for the current harness.
- Show a hover explanation for each disabled field.
- Keep capability logic source-of-truth in harness implementation code.

## What Was Reviewed
- Harness interface and implementations:
  - `pkg/api/harness.go`
  - `pkg/harness/gemini_cli.go`
  - `pkg/harness/claude_code.go`
  - `pkg/harness/opencode.go`
  - `pkg/harness/codex.go`
  - `pkg/harness/generic.go`
- Hook processing and dialect support:
  - `cmd/fabrictool/commands/hook.go`
  - `pkg/fabrictool/hooks/dialects/claude.go`
  - `pkg/fabrictool/hooks/dialects/gemini.go`
  - `pkg/fabrictool/hooks/handlers/limits.go`
- Runtime env injection and telemetry wiring:
  - `pkg/agent/run.go`
  - `pkg/runtime/common.go`
- Current advanced config UI:
  - `web/src/components/pages/agent-configure.ts`
- Hub update behavior:
  - `pkg/hub/handlers.go` (`updateAgent`)

## Findings

### 1) Advanced UI currently has no harness capability gating
`web/src/components/pages/agent-configure.ts` renders all advanced fields as editable and does not resolve harness type/capabilities before save.

### 2) Hook-based features only work for harnesses with supported hook dialects
`fabrictool hook` only registers `claude` and `gemini` dialect parsers. There are no `codex` or `opencode` dialects in `pkg/fabrictool/hooks/dialects`.

This matters because limits and status detail updates are driven by normalized hook events.

### 3) Limits capability is uneven by harness
`LimitsHandler` enforces:
- `max_turns` on `agent-end` events.
- `max_model_calls` on `model-end` events.

Event mapping:
- Gemini maps `AfterAgent` -> `agent-end` and `AfterModel` -> `model-end`.
- Claude maps `Stop/SubagentStop` -> `agent-end`, but has no `model-end` mapping.
- OpenCode/Codex have no hook dialect wiring.

Result:
- `max_turns`: effectively supported for Gemini and Claude.
- `max_model_calls`: effectively supported only for Gemini.
- `max_duration`: enforced by fabrictool init (harness-agnostic), supported for all harnesses.

### 4) Telemetry has two layers of support
Layer A: Fabric telemetry pipeline (`FABRIC_TELEMETRY_*`, `FABRIC_OTEL_*`) is harness-agnostic, driven by `pkg/agent/run.go` + `cmd/fabrictool/commands/init.go`.

Layer B: harness-native telemetry forwarding is harness-specific via `Harness.GetTelemetryEnv()` and only injected when `TelemetryEnabled` is true (`pkg/runtime/common.go`).

Current harness-native telemetry support:
- Gemini: yes (`GEMINI_TELEMETRY_*`).
- Claude: yes (`CLAUDE_CODE_ENABLE_TELEMETRY` + OTEL exporter envs).
- Codex: returns nil with comment "uses TOML config file" but no active injection path currently.
- OpenCode: returns nil (deferred).
- Generic: returns nil.

So telemetry checkbox semantics need to be explicit:
- Fabric telemetry config itself is broadly available.
- Native harness telemetry emission is currently only first-class for Gemini/Claude.

### 5) System prompt support differs by harness
- Gemini: native file injection to `~/.gemini/system_prompt.md`.
- Claude: native file + CLI `--system-prompt` usage.
- OpenCode: downgraded by prepending to `AGENTS.md`.
- Codex: TODO/no-op currently in `InjectSystemPrompt`.
- Generic: writes `.fabric/system_prompt.md` (no harness-native behavior).

This should be represented as capability quality, not just boolean.

### 6) Auth method support differs by harness
From `ResolveAuth` implementations + `RequiredAuthEnvKeys` mapping:
- Gemini: `api-key`, `auth-file`, `vertex-ai`.
- Claude: `api-key`, `vertex-ai`.
- OpenCode: `api-key`, `auth-file`.
- Codex: `api-key`, `auth-file`.
- Generic: passthrough (effectively permissive, not strict mode-specific).

The current UI auth dropdown always offers all options; this should be capability-gated.

### 7) Hub update path does not currently validate unsupported config by harness
`updateAgent` in `pkg/hub/handlers.go` persists `updates.Config` into `AppliedConfig.InlineConfig` for `created` agents without harness capability validation.

This means unsupported values can be saved and only fail later (or silently do nothing).

## Current Capability Matrix (from code behavior)

Legend:
- `Y` supported.
- `P` partial/degraded semantics.
- `N` unsupported.

| Advanced Field / Feature | Gemini | Claude | OpenCode | Codex | Generic | Notes |
|---|---|---|---|---|---|---|
| `max_turns` | Y | Y | N | N | N | Requires hook `agent-end` events. |
| `max_model_calls` | Y | N | N | N | N | Requires hook `model-end`; only Gemini maps it. |
| `max_duration` | N | N | N | N | N | Env is set but no enforcement implementation found. |
| `telemetry.enabled` (Fabric pipeline config) | Y | Y | Y | Y | Y | Harness-agnostic pipeline config.
| Native harness telemetry forwarding | Y | Y | N | P | N | Codex has comment/TOML intent but no active wiring path.
| `system_prompt` native behavior | Y | Y | P | N | P | OpenCode/generic are downgrade paths.
| `agent_instructions` | Y | Y | Y | Y | Y | Implemented for all harnesses.
| `auth_selectedType=api-key` | Y | Y | Y | Y | Y | |
| `auth_selectedType=auth-file` | Y | N | Y | Y | Y | Claude does not support auth-file mode.
| `auth_selectedType=vertex-ai` | Y | Y | N | N | Y | Generic passthrough is permissive.

## Proposed Capability Model

Add a harness-owned capabilities structure that shadows advanced config semantics.

### Shape
Define in API package (example):

```go
type SupportLevel string

const (
    SupportNo       SupportLevel = "no"
    SupportPartial  SupportLevel = "partial"
    SupportYes      SupportLevel = "yes"
)

type CapabilityField struct {
    Support SupportLevel `json:"support"`
    Reason  string       `json:"reason,omitempty"`
}

type HarnessAdvancedCapabilities struct {
    Harness string `json:"harness"`

    Limits struct {
        MaxTurns      CapabilityField `json:"max_turns"`
        MaxModelCalls CapabilityField `json:"max_model_calls"`
        MaxDuration   CapabilityField `json:"max_duration"`
    } `json:"limits"`

    Telemetry struct {
        EnabledConfig CapabilityField `json:"enabled"`
        NativeEmitter CapabilityField `json:"native_emitter"`
    } `json:"telemetry"`

    Prompts struct {
        SystemPrompt CapabilityField `json:"system_prompt"`
        AgentInstructions CapabilityField `json:"agent_instructions"`
    } `json:"prompts"`

    Auth struct {
        ApiKey   CapabilityField `json:"api_key"`
        AuthFile CapabilityField `json:"auth_file"`
        VertexAI CapabilityField `json:"vertex_ai"`
    } `json:"auth"`
}
```

### Ownership
Each harness implementation should expose this via a new interface method, for example:

```go
type CapabilityProvider interface {
    AdvancedCapabilities() api.HarnessAdvancedCapabilities
}
```

This keeps capability truth in `pkg/harness/*` as requested.

## Proposed Implementation Plan

### Phase 1: Backend capability source of truth
1. Add capability structs in `pkg/api`.
2. Extend harness interface (or optional capability interface) and implement in:
   - `GeminiCLI`, `ClaudeCode`, `OpenCode`, `Codex`, `Generic`.
3. Add unit tests per harness capability map.

### Phase 2: Resolve effective harness for an agent
1. Add a helper in Hub to resolve harness type from the agent’s effective harness-config.
2. Recommended resolution order:
   - `agent.AppliedConfig.HarnessConfig` (name) -> resolve harness config record/type.
   - Fallback: inline config `harness_config`.
   - Fallback: conservative default (`generic`) with explicit warning reason.
3. Expose resolved harness type + capability object in `GET /api/v1/agents/{id}` response.

### Phase 3: UI capability-driven field disabling
In `web/src/components/pages/agent-configure.ts`:
1. Load capabilities from agent detail payload.
2. For each advanced field:
   - Keep visible.
   - Set `disabled` when unsupported.
   - Add `sl-tooltip` reason text sourced from capability map.
3. Specific UI behavior:
   - Disable `Max Model Calls` for non-Gemini harnesses.
   - Disable `Max Turns` where hooks/agent-end not supported.
   - Disable `Max Duration` for all harnesses until enforcement exists.
   - Disable unsupported auth options in the auth dropdown.
   - For `system_prompt` partial support, keep enabled but show warning hint.

### Phase 4: Save-time validation in Hub
1. In `updateAgent`, validate incoming `updates.Config` against resolved harness capabilities.
2. For unsupported non-empty fields, return `400 ValidationError` with field-level reasons.
3. Keep this server-side even with UI disabling to prevent API misuse/regressions.

### Phase 5: Tests
1. Harness capability unit tests in `pkg/harness/*_test.go`.
2. Hub tests for validation in `pkg/hub/*_test.go`.
3. Frontend tests for disabled state + tooltip copy.
4. Integration tests for:
   - Gemini allows `max_model_calls`.
   - Claude rejects `max_model_calls`.
   - Any harness rejects `max_duration` until implemented.

## Notes and Follow-up Work
- `max_duration` currently appears designed but not implemented. Either:
  - Implement enforcement in `fabrictool` init/supervisor and mark supported where applicable, or
  - Keep disabled with explicit tooltip: "Not implemented yet".
- Codex telemetry likely needs explicit implementation if we want to advertise native telemetry support (e.g., mutate `~/.codex/config.toml` at start/provision).
- Existing docs include at least one likely stale claim (Codex OpenTelemetry support) relative to current code path; docs should be updated after implementation.

## Recommended Initial Capability Defaults (safe first pass)
- Gemini: full support except `max_duration`.
- Claude: support `max_turns`, no `max_model_calls`, no `max_duration`.
- OpenCode/Codex/Generic: no hook-derived limits; no `max_duration`; telemetry config allowed but native telemetry limited.

This conservative baseline prevents overpromising and aligns with current implementation evidence.
