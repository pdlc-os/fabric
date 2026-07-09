# Fabric Plugin Architecture Improvements

Issues identified during the first real message broker plugin implementation (OpenClaw) that require changes in the Fabric core project (`fabric/pkg/plugin/`, `fabric/pkg/hub/`, etc.).

---

## 1. Implement `POST /api/v1/broker/inbound` Hub Endpoint (High)

The plugin inbound delivery path depends on a hub API endpoint that does not yet exist. Both this plugin and the reference broker (`refbroker`) construct `POST /api/v1/broker/inbound` requests with a `{ "topic": "...", "message": {...} }` JSON body, but the hub has no handler for this route.

Without this endpoint, all inbound message delivery from any broker plugin is non-functional — messages are POSTed and rejected (or silently dropped if the hub returns a generic 404).

**What's needed**:
- A new HTTP handler in `pkg/hub/` that accepts the inbound payload
- Topic parsing to extract grove ID and agent slug from the topic string
- Dispatch to `DispatchAgentMessage()` (bypassing the broker to avoid circular delivery)
- Authentication via broker HMAC credentials
- The `X-Fabric-Plugin-Name` header should be logged for observability

**Reference**: `message-broker-plugins.md` §Inbound Message Routing, §Plugin Authentication for Hub API Callbacks

---

## 2. Add Runtime Health to Plugin Interface (Medium)

`PluginInfo` only has static metadata (`Name`, `Version`, `MinFabricVersion`, `Capabilities`). There is no way for the host to query whether a plugin is healthy at runtime.

The OpenClaw plugin cannot report:
- Whether its WebSocket connection is active or degraded
- Whether inbound messages are being buffered or dropped
- Last successful send/receive timestamp

**Options**:
- Add `Status string` and `StatusDetail string` fields to `PluginInfo` so `GetInfo()` can return dynamic state
- Add a separate `HealthCheck() (*HealthStatus, error)` method to `MessageBrokerPluginInterface`
- The second option is cleaner since it separates static metadata from runtime health, but requires a protocol version bump

**Reference**: `message-broker-plugins.md` §Plugin Behavior During Hub Unavailability — mentions health degradation reporting

---

## 3. `Subscribe(pattern)` Doesn't Generalize Beyond Pub/Sub (Medium)

The `MessageBrokerPluginInterface.Subscribe(pattern string)` method assumes a pub/sub model where the plugin can filter inbound messages by topic pattern. The OpenClaw plugin ignores the `pattern` parameter entirely — it starts a global event listener regardless of what pattern is passed.

This will be true for any integration that is not a topic-based pub/sub system (webhooks, polling APIs, WebSocket event streams, etc.).

**Options**:
1. **Document the convention**: Calling `Subscribe(">"` or `Subscribe("*")` means "start all inbound delivery." Plugins that don't support filtering should accept any pattern and start their global listener. This is the lowest-cost option.
2. **Split the interface**: Add `StartInbound() error` / `StopInbound() error` for non-pub/sub brokers, keeping `Subscribe`/`Unsubscribe` for brokers that support topic filtering. This is cleaner but requires updating the RPC wrappers and adapter.
3. **Ignore for now**: Accept that the pattern is a hint and plugins may ignore it. Document this in the plugin authoring guide.

Option 1 is recommended for v1 — it costs nothing beyond documentation.

---

## 4. `map[string]string` Config Limits Plugin Complexity (Low)

`Configure(config map[string]string)` forces all config values to be flat strings. The OpenClaw plugin encodes its route map as `"agent:coder=users/123,*=users/default"` — a comma-separated key=value string inside a string value. This is fragile (no support for values containing `=` or `,`) and doesn't scale to nested config.

This was an intentional v1 decision. For future plugin versions, consider:
- `ConfigureJSON(data []byte) error` — plugins receive raw JSON and unmarshal into their own config struct
- `map[string]interface{}` with gob serialization — more flexible but still untyped
- A `config_file` key convention — the host writes a temp JSON/YAML file and passes its path

The JSON approach is recommended — it's the simplest upgrade path and lets plugins define strongly-typed config structs.

**Note**: This would require a protocol version bump since it changes the RPC method signature. Could be offered as an optional alternative method alongside the existing `Configure()`.

---

## 5. No Host-to-Plugin Event Mechanism (Low)

The RPC interface is unidirectional: the host calls plugin methods. The plugin cannot receive push notifications from the host. This means:

- Plugins cannot learn about new agent creation/destruction dynamically
- No graceful pre-shutdown warning (only `Close()` which is immediate)
- Config changes require a full plugin restart

For `net/rpc` this is a fundamental limitation. A pragmatic solution without switching to gRPC streaming:
- Add a `Notify(event string, data map[string]string) error` method on the plugin interface
- The host calls it when relevant events occur (agent lifecycle, config update, pre-shutdown)
- Plugins that don't care can return nil

This is low priority — the current restart-on-config-change model works for now.

---

## 6. Plugin Logging Loses Structured Context (Low)

Plugins log to stderr, which go-plugin captures and forwards to the host. However:
- `slog` structured fields from the plugin are serialized as text and reparsed by go-plugin's log capture, losing structure
- Log levels from the plugin don't map cleanly to the host's log levels
- There's no correlation ID between host operations and the plugin logs they trigger

The `pkg/plugin/logger.go` adapter bridges go-hclog to slog for the host side, but plugin-side slog output on stderr doesn't get the reverse treatment.

**Recommendation**: Document the recommended logging pattern for plugin authors (use go-hclog directly, or use slog with text handler and accept the loss of structure). Consider providing a `pkg/plugin/pluginlog` helper package that plugin authors can import for consistent log formatting.

---

## Summary

| Priority | Issue | Effort |
|----------|-------|--------|
| **High** | Implement `/api/v1/broker/inbound` endpoint | Medium — new handler, auth, dispatch |
| **Medium** | Add runtime health to plugin interface | Small — add fields or method |
| **Medium** | Document or fix `Subscribe(pattern)` semantics | Small — documentation or interface change |
| **Low** | Upgrade config to support structured data | Medium — protocol version bump |
| **Low** | Add host→plugin notification mechanism | Medium — new RPC method |
| **Low** | Plugin logging guidance | Small — documentation |
