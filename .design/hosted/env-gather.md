# Environment Gather: CLI Fallback for Remote Agent Environment Variables

## Status
**MVP Implemented** — The core env-gather flow is implemented end-to-end across CLI, Hub, and Broker. See §7 for implementation details.

## 1. Goal

When a remote agent is started via the Hub, environment variables required by the agent's template may not be fully satisfiable by the Hub and Broker alone. The env-gather flow allows the system to fall back to the CLI (the user's local machine) to supply missing variables before the agent container launches.

Today, this fallback does not exist. The Hub dispatches agents directly to the Broker with whatever environment it has resolved. If variables are missing, the agent starts without them and may fail at runtime.

The goal is to implement an optimistic dispatch with broker-initiated fallback:
1. The Hub resolves what it can from its scoped env storage (global, grove, user) and dispatches to the Broker.
2. The Broker merges Hub-provided env with its own local environment and evaluates whether all required keys are satisfied.
3. If all required keys have values, the Broker starts the agent immediately (fast path).
4. If required keys remain unsatisfied ("needed"), the Broker initiates a gather request back to the Hub over WebSocket, which relays it to the CLI.
5. The CLI gathers from `os.Getenv()`, prompts the user for confirmation, and submits the values back.
6. If the CLI cannot satisfy the needed keys, it is an error — the agent is not started.

This is intentionally the reverse of local mode, where the CLI environment is used as a short-circuit (values are projected directly into the container). In hosted mode, the CLI is the last resort in the resolution chain.

The detailed protocol is specified in [`remote-env-gather.md`](remote-env-gather.md). This document captures design decisions and remaining open questions.

## 2. Current State of Implementation

The MVP env-gather flow is fully implemented. The previous gaps (no broker-initiated gather, no required-vs-needed analysis, no CLI gathering UX, no env submission endpoint) have all been addressed. See §7 for the complete implementation record.

## 3. Design Decisions

### 3.1 How required environment variables are declared

A key is **required** if it is declared in either a settings profile (`HarnessConfigEntry.Env`) or a template config file. Both sources contribute to the set of required keys for the final agent environment.

If a source declares a key with a non-empty value (e.g., `LOG_LEVEL: "info"`), the value is provided and the key is satisfied at config time.

If a source declares a key with an empty string value (e.g., `GEMINI_API_KEY: ""`), the key is required but has no value yet. It must be satisfied somewhere in the resolution chain — Hub env storage, Broker local environment, or CLI fallback — before the agent can start.

### 3.2 Required vs. Needed

These are distinct concepts at different points in the lifecycle:

- **Required** is a config-time property. A key is required if it appears in any settings file or template config. The set of required keys is known before the agent is created.

- **Needed** is a provisioning-time condition. A required key is "needed" if it still has no value after the Hub and Broker have both contributed what they can. A needed key is the trigger for the CLI fallback.

If the Broker cannot fill all required keys from Hub-provided env + its own local env, it must not start the agent. Instead, it initiates the gather request.

### 3.3 Optimistic dispatch with broker-initiated fallback

The Hub does not have the full picture — the Broker's own local environment can fill gaps that the Hub cannot see. Therefore, the Hub dispatches optimistically, and the Broker is responsible for determining whether env is complete.

The flow:

1. **Hub dispatches to Broker** with `ResolvedEnv` (everything the Hub could resolve) as it does today.
2. **Broker merges** Hub-provided env with its own local env and `config.Env` overrides.
3. **Broker evaluates completeness.** For each required key, check if the merged result has a non-empty value.
   - **All satisfied:** Broker starts the agent immediately (fast path, no change from today's behavior).
   - **Some needed:** Broker sends a gather request back to the Hub over the existing WebSocket control channel, listing the needed keys. The agent remains in its current pending/provisioning state.
4. **Hub relays to CLI.** The Hub forwards the needed-keys list to the CLI (which is still waiting on the create response).
5. **CLI gathers and submits.** The CLI checks `os.Getenv()` for each needed key and prompts the user (see 3.4). If satisfied, submits values back to the Hub.
6. **Hub forwards to Broker.** Hub sends the gathered env to the Broker, which merges and starts the agent.

This avoids a dedicated pre-flight round-trip. In the common case where the Hub and Broker together satisfy everything, the agent starts with zero additional latency.

### 3.4 CLI prompt behavior

**Interactive mode:** The CLI checks `os.Getenv()` for each needed key. It then displays a summary of the keys found (not values) and prompts the user to confirm it is OK to send them. If the user confirms, the values are submitted.

If the CLI cannot find all needed keys in its local environment, it is an **error**. The CLI reports which required keys were not available at any point during provisioning and instructs the user to set them — in the Hub (`fabric hub env set`), on the Broker, or in the local shell environment — and try again.

**Non-interactive mode (`--non-interactive`):** If needed keys are not available in the CLI environment, it is an immediate error. No prompting occurs; the CLI exits with an error message listing the unsatisfied keys.

### 3.5 Communication channel

The Broker initiates the gather request back to the Hub over the existing WebSocket control channel. The Broker must never be required to be directly reachable by the Hub (NAT, firewalls, tunnels). The WS channel is already established by the Broker and is the correct path for broker-to-hub communication.

### 3.6 Agent state during env gather

No new agent state is introduced. The agent uses the existing `pending` or `provisioning` states during the gather process. Logging should clearly indicate what is happening (e.g., "waiting for CLI env gather", "needed keys: GEMINI_API_KEY, GITHUB_TOKEN").

If the gather process cannot be satisfied — the CLI fails to provide the needed keys, disconnects, or times out — it is an error. The agent creation fails, and the user must re-issue the `fabric start` command after establishing the required env vars in the system or locally.

There is no resumability or deferred env submission. The gather is synchronous within the scope of the `fabric start` command.

## 4. MVP Scope

The minimum implementation that demonstrates the flow end-to-end:

1. A harness config (settings profile) declares a required env key with an empty value, e.g., `GEMINI_API_KEY: ""`.
2. The Hub dispatches to the Broker with its resolved env (which does not include a value for this key).
3. The Broker detects that `GEMINI_API_KEY` is required but has no value after merging Hub env + local env.
4. The Broker sends a gather request over WS listing `["GEMINI_API_KEY"]` as needed.
5. The Hub relays the needed list back to the CLI's pending create request.
6. The CLI checks `os.Getenv("GEMINI_API_KEY")`, finds the value, and prompts the user to confirm sending it.
7. The CLI submits the gathered env back to the Hub.
8. The Hub forwards to the Broker, which merges and starts the agent.

**Out of scope for MVP:** TTL/GC for stalled gathers, `--force` flag, `--no-env-gather` flag, scope annotations in the gather response, `fabric env submit` recovery command, `${VAR}` reference expansion in template values.

## 5. Open Questions

### 5.1 Template env declarations vs. settings-only

The decision in 3.1 covers settings profiles declaring required keys. Should template YAML files (`fabric-agent.yaml`) also support an `env:` section that declares required keys?

If so, the template and settings declarations would be merged (union of keys). This would allow template authors to declare env requirements that are visible and version-controlled alongside the template, rather than relying on users to have the right settings profile.

Not blocking MVP — settings-only is sufficient to demonstrate the flow.

### 5.2 Gather request timeout

How long should the Hub/Broker wait for the CLI to respond with gathered env? The CLI gather is synchronous within `fabric start`, so it should be fast (user confirms a prompt). But network delays or a slow user could extend this.

**Options:**
- Fixed timeout (e.g., 60 seconds) with error on expiry.
- No explicit timeout; rely on the existing WS/HTTP connection timeouts.
- Configurable via Hub settings.

### 5.3 Multiple brokers and env availability

If a grove has multiple brokers with different local environments, a user might prefer a broker based on what env vars it already has. Should `fabric start` provide visibility into which broker can satisfy which keys before the user selects one?

**Recommendation:** Defer. User selects broker first (existing flow), then env is gathered. Broker selection based on env availability is a future optimization.

## 6. Reference

- Full protocol specification: [remote-env-gather.md](remote-env-gather.md)
- Hub env storage API: `pkg/hubclient/env.go`
- Hub dispatch with ResolvedEnv: `pkg/hub/httpdispatcher.go`
- Broker env merging: `pkg/runtimebroker/handlers.go`
- CLI agent start (Hub path): `cmd/common.go`
- Settings/profile env config: `pkg/config/settings_v1.go`
- Template embeds: `pkg/config/embeds/templates/`

## 7. Implementation Record

The MVP was implemented across 5 commits on the `env-gather` branch, with 2 follow-up commits for bug fixes and enhancements.

### 7.1 Shared Types and Interface Methods (`30ced2c`)

Added the `GatherEnv bool` field to create-agent request types at every layer (CLI, Hub, Broker) so the flag propagates end-to-end. Defined response types for the gather protocol:

**Broker types** (`pkg/runtimebroker/types.go`):
- `EnvRequirementsResponse` — returned by the Broker on HTTP 202: lists `AgentID`, `Required`, `HubHas`, `BrokerHas`, and `Needs` keys.
- `FinalizeEnvRequest` — carries gathered `Env map[string]string` from the Hub to the Broker.

**Hub types** (`pkg/hub/handlers.go`, `pkg/hub/server.go`):
- `RemoteEnvRequirementsResponse` — Hub-side mirror of the Broker's env requirements.
- `EnvGatherResponse` / `EnvSource` — enriched response sent to the CLI with scope annotations (which source provided which key).
- `SubmitEnvRequest` — CLI submits gathered env to the Hub.
- Added `EnvGather *EnvGatherResponse` to `CreateAgentResponse` (present only on 202).

**Hub client types** (`pkg/hubclient/agents.go`):
- `EnvGatherResponse`, `EnvSource`, `SubmitEnvRequest` — client-facing equivalents.
- Added `SubmitEnv()` method to the agent service interface.

**Interface extensions**:
- `RuntimeBrokerClient` (`pkg/hub/server.go`) gained `CreateAgentWithGather()` and `FinalizeEnv()`.
- `AgentDispatcher` (`pkg/hub/server.go`) gained `DispatchAgentCreateWithGather()` and `DispatchFinalizeEnv()`.
- Implemented on all client variants: `HTTPRuntimeBrokerClient`, `ControlChannelBrokerClient`, `HybridBrokerClient`, and `AuthenticatedBrokerClient`.

### 7.2 Broker Env Evaluation and Finalize Endpoint (`d756fdf`)

**Env completeness check** (`pkg/runtimebroker/handlers.go`): When `req.GatherEnv` is true, the Broker's `createAgent()` handler evaluates env completeness after the existing merge logic. It extracts required keys from the settings profile via a new `extractRequiredEnvKeys()` method (loads `settings.yaml` from `req.GrovePath` using `config.LoadEffectiveSettings()`). Keys with empty values in `HarnessConfigEntry.Env` or `V1ProfileConfig.Env` are considered required.

Each required key is categorized:
- `hubHas` — value was in `req.ResolvedEnv` (provided by Hub)
- `brokerHas` — value was found in `os.Getenv()` or `config.Env`
- `needs` — no value found anywhere

If `needs` is empty, the agent starts immediately (HTTP 201, existing fast path). If `needs` is non-empty, the Broker returns HTTP 202 with `EnvRequirementsResponse` and stores the pending agent state in memory.

**Pending state** (`pkg/runtimebroker/server.go`): Added `pendingEnvGather map[string]*pendingAgentState` (guarded by mutex) to the Broker `Server`. Each entry holds the original `CreateAgentRequest`, the merged env so far, and a creation timestamp. No TTL/GC is implemented yet (noted as out-of-scope for MVP in §4).

**Finalize endpoint** (`pkg/runtimebroker/handlers.go`): New `finalizeEnv()` handler at `POST /api/v1/agents/{id}/finalize-env`. Retrieves the pending state, merges the gathered env, and starts the agent. Returns HTTP 201 with the standard agent response. Registered via the existing `handleAgentAction` routing in `pkg/runtimebroker/server.go`.

### 7.3 Hub 202 Handling and Submit Endpoint (`b80e3b0`)

**Hub env resolution** (`pkg/hub/httpdispatcher.go`): New `resolveEnvFromStorage()` method on `HTTPAgentDispatcher` queries the store for env vars across `grove` and `user` scopes, merging them with grove-scope precedence. New `buildEnvSources()` creates a scope-tracking map so the CLI can see where each key was resolved from.

**Dispatch with gather** (`pkg/hub/httpdispatcher.go`): `DispatchAgentCreateWithGather()` resolves env from storage, sets `GatherEnv=true` and `EnvSources` on the request, then calls the broker client's `CreateAgentWithGather()`. If the broker returns env requirements (non-nil), they are passed back to the handler.

**Handler changes** (`pkg/hub/handlers.go`): The `createAgent()` handler checks `req.GatherEnv`. When true, it uses `DispatchAgentCreateWithGather` instead of `DispatchAgentCreate`. If env requirements come back, the handler sets the agent status to `provisioning`, builds an enriched `EnvGatherResponse` via `buildEnvGatherResponse()` (annotating each `hubHas` key with its scope), and returns HTTP 202.

**Submit endpoint** (`pkg/hub/handlers.go`): New `submitAgentEnv()` handler at `POST /api/v1/groves/{groveId}/agents/{agentId}/env`. Validates the agent is in `provisioning` state (returns 409 Conflict otherwise), calls `DispatchFinalizeEnv()` to forward the gathered env to the Broker, and updates the agent status to `running`.

**Control channel passthrough** (`pkg/hub/controlchannel_client.go`): Added `doRequestRaw()` that does not reject HTTP 202 as an error. `CreateAgentWithGather()` uses this to detect 202 responses and parse `RemoteEnvRequirementsResponse` from the body, while 201 responses follow the existing `RemoteAgentResponse` path.

### 7.4 CLI Env-Gather Flow (`412e1fe`)

**Trigger** (`cmd/common.go`): The CLI sets `GatherEnv: true` on all Hub-mode create requests. After receiving the response from `createAgentWithBrokerResolution()`, it checks `resp.EnvGather`. If non-nil (indicating a 202 from the Hub), it calls `gatherAndSubmitEnv()`.

**Gather logic** (`cmd/common.go`, `gatherAndSubmitEnv()`):
1. For each key in `resp.EnvGather.Needs`, checks `os.Getenv(key)`.
2. Separates keys into "found locally" and "still missing".
3. If any keys are still missing, prints an error listing unsatisfied keys with guidance to set them via `fabric hub env set`, broker env, or local shell, and returns an error.
4. In interactive mode (`util.IsTerminal()`), prompts the user to confirm sending the gathered values (displays key names, not values).
5. Submits via `hubCtx.Client.GroveAgents(groveID).SubmitEnv()`.
6. On success, returns the agent info from the finalized response so the normal startup flow continues.

### 7.5 Tests (`9420899`)

**Broker tests** (`pkg/runtimebroker/handlers_envgather_test.go`, 8 tests):
- `TestEnvGather_AllSatisfied` — all required keys provided by Hub; agent starts immediately (201).
- `TestEnvGather_NeedsKeys` — Hub provides one key, another is missing; Broker returns 202 with categorized keys.
- `TestEnvGather_BrokerHasKey` — required key exists in Broker's own `os.Getenv()`; agent starts (201).
- `TestEnvGather_FinalizeEnv` — two-phase flow: 202, then finalize-env with gathered values; agent starts (201).
- `TestEnvGather_NoGatherFlag` — `GatherEnv=false` skips env evaluation entirely; agent starts (201).
- `TestEnvGather_HarnessAware` — harness-config directory on disk without settings `harness_configs.env`; broker detects `ANTHROPIC_API_KEY` as needed via harness `RequiredEnvKeys()` (202).
- `TestEnvGather_GeminiAuthType` — gemini harness with `auth_selected_type: vertex-ai` requires `GOOGLE_CLOUD_PROJECT` instead of `GEMINI_API_KEY` (202).
- `TestEnvGather_SettingsAuthTypeOverride` — settings profile `harness_overrides` overrides on-disk `auth_selected_type` from `gemini-api-key` to `oauth-personal`; no env keys required, agent starts (201).

**Hub tests** (`pkg/hub/envgather_test.go`, 8 tests):
- `TestEnvGather_HubDispatch_AllSatisfied` — dispatcher with all env satisfied returns nil requirements.
- `TestEnvGather_HubDispatch_NeedsGather` — dispatcher relays broker's env requirements correctly.
- `TestEnvGather_HubDispatch_FinalizeEnv` — dispatcher forwards gathered env to broker.
- `TestEnvGather_HubHandler_202Response` — full handler test: `GatherEnv=true` request produces HTTP 202 with `EnvGather` in response body.
- `TestEnvGather_HubHandler_GroveRoute_202Response` — env-gather via the grove-scoped route (`/api/v1/groves/{groveId}/agents`); verifies `DispatchAgentCreateWithGather` is called and 202 response includes `EnvGather`.
- `TestEnvGather_HubHandler_SubmitEnv` — submit endpoint accepts gathered env, forwards to broker, updates agent status to running.
- `TestEnvGather_HubHandler_SubmitEnv_InvalidState` — submit rejected with 409 when agent is not in provisioning state.
- `TestEnvGather_HubEnvResolution` — Hub resolves grove-scoped env vars from storage and includes them in `ResolvedEnv` sent to broker.

**Harness unit tests** (per-harness `*_test.go` files):
- `TestClaudeRequiredEnvKeys` — returns `[ANTHROPIC_API_KEY]` regardless of auth type.
- `TestGeminiRequiredEnvKeys` — returns `[GEMINI_API_KEY]` for default/api-key, `[GOOGLE_CLOUD_PROJECT]` for vertex-ai, `nil` for oauth-personal and compute-default-credentials.
- `TestCodexRequiredEnvKeys` — returns `nil`.
- `TestGenericRequiredEnvKeys` — returns `nil`.
- `TestOpenCodeRequiredEnvKeys` — returns `[ANTHROPIC_API_KEY]`.

### 7.6 Grove-Scoped Agent Create Handler (`edba6ec`)

The env-gather flow was only wired into `createAgent()` (the `/api/v1/agents` route used by the Broker control channel), but not into `createGroveAgent()` (the `/api/v1/groves/{groveId}/agents` route the CLI actually hits). This meant `GatherEnv=true` from the CLI was decoded correctly but never acted upon — the handler always called `DispatchAgentCreate` instead of `DispatchAgentCreateWithGather`.

**Handler fix** (`pkg/hub/handlers.go`): Added the same `req.GatherEnv` routing logic to `createGroveAgent()`. When `GatherEnv` is true, the handler calls `DispatchAgentCreateWithGather` and handles the 202 path (setting agent status to `provisioning`, building the enriched `EnvGatherResponse`, returning HTTP 202 with `CreateAgentResponse.EnvGather` populated). The non-gather path remains unchanged.

### 7.7 Harness-Aware Env Key Extraction (`e1dc8c7`)

The broker's `extractRequiredEnvKeys()` previously only detected env requirements from settings `harness_configs[*].env` empty-value entries. When no `harness_configs` section existed in settings (common case), it returned 0 required keys, making env-gather ineffective in practice.

**Harness interface** (`pkg/api/harness.go`): Added `RequiredEnvKeys(authSelectedType string) []string` to the `Harness` interface. Each harness declares its intrinsic env requirements:
- `ClaudeCode` → `[ANTHROPIC_API_KEY]`
- `GeminiCLI` → varies by auth type: `[GEMINI_API_KEY]` for default/api-key, `[GOOGLE_CLOUD_PROJECT]` for vertex-ai, `nil` for oauth-personal/compute-default-credentials
- `OpenCode` → `[ANTHROPIC_API_KEY]`
- `Codex`, `Generic` → `nil`

**Broker extraction rewrite** (`pkg/runtimebroker/handlers.go`): `extractRequiredEnvKeys()` now uses a two-phase approach:
1. **Phase 1 (harness-aware):** Resolves the active harness-config name via `resolveHarnessConfigName()`, loads the on-disk harness-config directory to determine the harness type and `auth_selected_type`, then calls the harness's `RequiredEnvKeys()` to get intrinsic requirements.
2. **Phase 2 (settings-based):** Preserved from the original implementation — extracts keys with empty values from settings `harness_configs[*].env` and `profiles[*].env`, allowing users to declare custom env requirements beyond what the harness itself needs.

**Harness-config resolution** (`pkg/runtimebroker/handlers.go`): Two new methods support Phase 1:
- `resolveHarnessConfigName()` — determines which harness-config to use via a resolution chain: explicit `req.Config.Harness` → template name matching on-disk directory → template name matching settings entry → profile's `DefaultHarnessConfig` → settings' `DefaultHarnessConfig`.
- `resolveHarnessIdentity()` — loads the on-disk harness-config directory and applies settings overrides (via `ResolveHarnessConfig`) to determine the final `harnessName` and `authSelectedType`. Settings profile `harness_overrides` for `auth_selected_type` take precedence over on-disk values.

### 7.8 Implementation Deviations from Design

**HTTP status codes instead of WebSocket messages.** The design (§3.5) specifies the gather request flowing over the existing WebSocket control channel. The implementation uses HTTP status codes (201 vs 202) tunneled through the existing WebSocket `RequestEnvelope`/`ResponseEnvelope` infrastructure instead of adding new WebSocket message types. The effect is the same — the Broker signals back to the Hub, which relays to the CLI — but the mechanism is HTTP-level rather than a distinct WS message type.

**Scope annotations included in MVP.** The design listed scope annotations as out-of-scope for MVP (§4). The implementation includes them: the Hub enriches the `EnvGatherResponse.HubHas` entries with the scope that provided each key (grove, user, etc.), allowing the CLI to display where keys were resolved from.

**No explicit timeout on pending state.** The Broker stores pending agent state in memory with a `CreatedAt` timestamp but does not currently enforce a TTL or garbage-collect stale entries. This matches the design's open question (§5.2) and is deferred for a follow-up.

### 7.9 Remaining Work (Post-MVP)

The following items from §4 and §5 remain unimplemented:
- TTL/GC for stalled pending gathers on the Broker
- `--force` flag to skip env-gather and start with missing keys
- `--no-env-gather` flag to disable the flow entirely
- `fabric env submit` recovery command for re-submitting env after a failed gather
- `${VAR}` reference expansion in template env values
- Template-level env declarations (§5.1) — currently settings-only
- Configurable gather timeout (§5.2)
- Broker selection based on env availability (§5.3)
