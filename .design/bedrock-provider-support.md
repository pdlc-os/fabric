# PRD — AWS Bedrock Provider Support for Fabric Harnesses

> **Status:** Draft — ready for review
> **Date:** 2026-07-09
> **Provenance:** Merged from two independent analyses of the auth pipeline, both
> verified against source at commit `1f8d5047` (main). The Claude Code Bedrock
> environment contract has been **confirmed against official Claude Code docs**
> ([Amazon Bedrock](https://code.claude.com/docs/en/amazon-bedrock.md),
> [Environment variables](https://code.claude.com/docs/en/env-vars.md)) — the
> former draft's ⚠️ VERIFY items are resolved and inlined in §7.

---

## 1. Summary

Add **AWS Bedrock** as a first-class authentication type for Fabric agents,
alongside the currently supported providers (Anthropic API, Google Vertex AI,
OpenAI, Gemini/Google AI, GitHub Copilot). A Fabric agent running the **Claude
Code harness** (primary target) can be pointed at Bedrock as its model backend
by selecting a `bedrock` auth type and supplying AWS credentials — mirroring how
the existing `vertex-ai` auth type points Claude Code at Google Vertex.

This unblocks all-AWS deployments where models are consumed through Bedrock
(data residency, IAM-governed access, consolidated billing) rather than a
vendor's public API.

Bedrock is a new **auth type on the existing `claude` harness**, not a new
harness or provider subsystem. The plumbing is bounded and additive; the
substantive work is AWS credential handling, not the "use Bedrock" flag.

---

## 2. Background — how auth works today (verified in source)

An "auth type" in Fabric is **declarative config + a shared resolver + an env
overlay**, with a host-side **gather stage** feeding it. Four moving parts:

1. **Gather (host side)** — `GatherAuthWithEnv()` (`pkg/harness/auth.go:50-132`)
   discovers credentials into `api.AuthConfig` (`pkg/api/types.go:500-532`):
   env vars (`ANTHROPIC_API_KEY`, `CLAUDE_CODE_OAUTH_TOKEN`, `GEMINI_API_KEY`,
   `OPENAI_API_KEY`, `GOOGLE_CLOUD_PROJECT`/`_REGION`, … at auth.go:63-80) and
   well-known files (gcloud ADC, `~/.claude/.credentials.json`,
   `~/.codex/auth.json`, … at auth.go:84-119). In broker mode
   (`localSources=false`) host env/filesystem fallback is disabled so operator
   credentials never leak into hub-dispatched agents.
   **Config-driven extras:** `gatherConfigEnvVars()` (auth.go:136-159) forwards
   any env key declared in a harness config's `auth.types[*].required_env` —
   new *env-var* credentials need **no Go change** to be gathered.
2. **Declaration** — `harnesses/<name>/config.yaml` under `auth.types.<name>`
   lists `required_env` / `required_files` plus an `autodetect` map
   (claude: `harnesses/claude/config.yaml:85-120`; schema:
   `pkg/api/harness_auth_metadata.go:18-61`).
3. **Resolution** — `harnesses/fabric_harness.py` (`AuthMethod` / `AuthSpec` /
   `select_auth`; no-auth fallback at fabric_harness.py:495-511). Go-side
   detection in `pkg/harness/auth.go`: `DetectAuthTypeFromFileSecrets` /
   `FromEnvVars` / `FromGCPIdentity` (auth.go:369-443), each shadowed by a
   config-driven `*FromConfig` twin (auth.go:535-651) that the broker/hub
   prefer (`pkg/runtimebroker/handlers.go:2069-2168`,
   `pkg/hub/handlers_agent_create_helpers.go:799-843`).
4. **Env overlay (container side)** — the harness's `provision.py` maps the
   resolved method to the env the underlying CLI needs.
   `harnesses/claude/provision.py:178-191` (`_build_env_overlay`) is where
   `vertex-ai` becomes `CLAUDE_CODE_USE_VERTEX=1` + project/region. Method
   precedence: api-key > oauth-token > auth-file > vertex-ai
   (provision.py:30-33).

File-based credentials additionally flow through the **file-secret switches**:
`OverlayFileSecrets` (auth.go:174-190), `setAuthConfigFieldByTargetSuffix`
(auth.go:253-265), the target→path map in `pkg/agent/run.go:1302-1310`, and the
container mount mapping in `ContainerScriptHarness.ResolveAuth`
(`pkg/harness/container_script_harness.go:210-285`).

### 2.1 Providers/auth types supported today

Canonical auth types: **`api-key`**, **`oauth-token`**, **`auth-file`**,
**`vertex-ai`**. Per harness: `claude` (Anthropic API, Claude subscription,
Vertex), `codex` (OpenAI), `gemini-cli` (Google AI, Vertex), `opencode`
(Anthropic, OpenAI, Vertex), `copilot` (GitHub tokens), `hermes`, `antigravity`
(Google).

### 2.2 The gap

- **No `bedrock` auth type exists** on any harness. `bedrock` /
  `CLAUDE_CODE_USE_BEDROCK` / `AWS_*` appear nowhere in `pkg/`, `cmd/`, or
  `harnesses/` (verified by grep; the only mention is an illustrative example
  in `.design/decouple-templates.md:240`).
- **`vertex-ai` is the sole precedent** for a managed-cloud gateway backend.
  Bedrock is the AWS analog and follows the same shape.
- Fabric has credential resolvers for Anthropic API keys and Google ADC /
  service-account identity (`DetectAuthTypeFromGCPIdentity`), but **no AWS
  credential resolver**. That — not the toggle — is the substantive gap.

---

## 3. Goals / Non-Goals

### Goals
- **G1.** Add a `bedrock` auth type to the `claude` harness so a Fabric Claude
  Code agent can use Bedrock.
- **G2.** Recognize `bedrock` as a first-class type across resolution and
  detection (`fabric_harness.py`, `pkg/harness/auth.go`,
  `pkg/api/harness_auth_metadata.go`), at parity with `vertex-ai`.
- **G3.** Define a secure path for AWS credentials into the agent container
  (Bedrock API key / static keys in v1; profile file in v1.5; IAM role as
  stretch — see §6.4).
- **G4.** Keep the pattern generalizable to other harnesses whose CLI supports
  Bedrock. *(Scoped down from the earlier draft: Gemini CLI and Copilot CLI
  have no native Bedrock support; OpenCode is unconfirmed — see OQ6.)*
- **G5.** Additive only; no regression to api-key / oauth-token / auth-file /
  vertex-ai.

### Non-Goals
- **NG1.** A general AWS STS / role-assumption service inside Fabric.
- **NG2.** Bedrock for harnesses whose CLI does not support it.
- **NG3.** AWS as a Fabric runtime/deployment backend (separate concern).
- **NG4.** Cost controls / Bedrock-specific rate limiting.

---

## 4. Users & Use Cases

- **U1 — All-AWS shop:** runs Fabric Hub/Brokers on EC2/EKS; models must come
  through Bedrock for residency/procurement/billing; cannot use the Anthropic
  public API or GCP Vertex.
- **U2 — IAM-governed access:** wants model access mediated by IAM roles
  rather than long-lived vendor API keys.
- **U3 — Multi-provider portability:** selects provider per agent/template
  (Anthropic vs. Vertex vs. Bedrock) with no harness-code changes.

---

## 5. Requirements

### Functional
- **FR1.** `fabric create` with the `claude` harness MUST accept a `bedrock`
  auth selection — explicitly (`--harness-auth bedrock` /
  `auth_selectedType`) and via autodetect when strong Bedrock signals are
  present (`AWS_BEARER_TOKEN_BEDROCK`, `CLAUDE_CODE_USE_BEDROCK`).
  Bare `AWS_REGION` MUST NOT trigger autodetection (false-positive trap —
  many environments set it for unrelated reasons).
- **FR2.** On provision, the container MUST receive the Claude Code Bedrock
  env contract (§7): `CLAUDE_CODE_USE_BEDROCK=1`, region, credentials, and
  optional model pins.
- **FR3.** AWS credentials MUST be injectable via Fabric's secret mechanism.
  Env-var secrets already flow by name (`fabric hub secret set
  AWS_BEARER_TOKEN_BEDROCK …` works once the config declares the key); a
  credentials-file secret needs a new well-known name (§6.3).
- **FR4.** Detection precedence MUST be deterministic and documented:
  explicit selection > api-key > oauth-token > bedrock > vertex-ai.
- **FR5.** Selecting `bedrock` MUST NOT require `ANTHROPIC_API_KEY`; its
  absence MUST NOT cause the `no_auth` drop-to-shell fallback
  (fabric_harness.py:495-511) when Bedrock credentials are present.

### Non-Functional
- **NFR1.** Additive & backward compatible (G5).
- **NFR2.** Follow the `vertex-ai` implementation shape for reviewability.
- **NFR3.** Secrets never logged; AWS creds handled like all other secrets.
- **NFR4.** Vendored `fabric_harness.py` copies stay in sync via
  `harnesses/gen/main.go` (`go generate`) and pass
  `harnesses/vendored_lib_test.go`. Edit sources under `harnesses/`, never
  `.fabric/`.

---

## 6. Proposed Design

Mirror `vertex-ai` end-to-end. All env-var names below are confirmed (§7).

### 6.1 Harness config — `harnesses/claude/config.yaml`

Add under `auth.types` (modeled on the `vertex-ai` block at config.yaml:85-120):

```yaml
    bedrock:
      required_env:
        - any_of: ["AWS_REGION", "AWS_DEFAULT_REGION"]
        - any_of:
            - "AWS_BEARER_TOKEN_BEDROCK"   # Bedrock API key (simplest)
            - "AWS_ACCESS_KEY_ID"          # + AWS_SECRET_ACCESS_KEY (+ AWS_SESSION_TOKEN)
            - "AWS_PROFILE"                # requires the file secret below
      # v1.5: optional AWS credentials file mounted as a file secret
      required_files:
        - name: aws-credentials
          type: file
          description: "AWS shared credentials file (profile-based auth)"
          field: AwsCredentialsFile        # new AuthConfig field — see §6.3
          alternative_env_keys: ["AWS_SHARED_CREDENTIALS_FILE"]
          required: false
  autodetect:
    env:
      # strong signals only — never bare AWS_REGION (FR1)
      AWS_BEARER_TOKEN_BEDROCK: bedrock
      CLAUDE_CODE_USE_BEDROCK: bedrock
```

Also advertise the capability in the `capabilities.auth` block
(config.yaml:68-72, next to `vertex_ai: { support: "yes" }`) and the auth-type
table in `harnesses/claude/README.md`.

### 6.2 Env overlay — `harnesses/claude/provision.py`

Add a `bedrock` method to the `AUTH` spec (provision.py:68-81) with precedence
after `vertex-ai`, and a branch in `_build_env_overlay()` (provision.py:178-191)
parallel to the vertex-ai branch:

```python
    if auth.method == "bedrock":
        overlay = {"CLAUDE_CODE_USE_BEDROCK": "1"}
        for k in ("AWS_REGION", "AWS_DEFAULT_REGION",
                  "AWS_BEARER_TOKEN_BEDROCK",
                  "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
                  "AWS_SESSION_TOKEN", "AWS_PROFILE",
                  "ANTHROPIC_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL",
                  "ANTHROPIC_DEFAULT_SONNET_MODEL",
                  "ANTHROPIC_DEFAULT_HAIKU_MODEL",
                  "ANTHROPIC_BEDROCK_BASE_URL"):
            v = _resolve(ctx, k)
            if v:
                overlay[k] = v
        return overlay
```

Then `go generate` (harnesses/gen/main.go) to re-vendor `fabric_harness.py`
copies (NFR4).

### 6.3 Go plumbing — gather, file secrets, fallback tables

**Env-var-only v1 needs no gather changes**: keys declared in `required_env`
flow through `gatherConfigEnvVars` (auth.go:136-159), Hub secret injection
(secrets are stored by env-var name), and broker/hub preflight
(handlers.go:2069-2168 / handlers_agent_create_helpers.go:799-843) — all
config-driven.

**The credentials-file path (v1.5) is real Go work** — every switch that knows
file secrets needs a new case:
- `AwsCredentialsFile` field on `api.AuthConfig` (`pkg/api/types.go:500-532`).
- Well-known file-secret name `AWS_CREDENTIALS` → container path
  `~/.aws/credentials`: cases in `OverlayFileSecrets` (auth.go:174-190),
  `setAuthConfigFieldByTargetSuffix` (auth.go:253-265), the target→field map in
  `pkg/agent/run.go:1302-1310`, and the mount mapping in
  `ContainerScriptHarness.ResolveAuth`
  (container_script_harness.go:250-279). Set `AWS_SHARED_CREDENTIALS_FILE` in
  the container to the mounted path.
- Host-side discovery of `~/.aws/credentials` in the well-known-files block
  (auth.go:84-119), local mode only.

**Compiled fallback tables** (parity with vertex-ai; used when a harness
config lacks the declarative `auth:` block): `case "bedrock"` arms in
`RequiredAuthEnvKeys` (auth.go:451-493), `RequiredAuthSecrets`
(auth.go:336-357), and `DetectAuthTypeFromEnvVars` (auth.go:407-417) —
matching only the strong signals per FR1. The `*FromConfig` twins need no
change. `ValidateAuth` (auth.go:295-327) is generic — no new entry needed.

### 6.4 AWS credential models, phased

- **v1 (env: bearer token / static keys):** credentials as Fabric env secrets;
  provisioner injects them. No Go changes beyond §6.3's fallback-table arms.
- **v1.5 (profile/file):** `AWS_CREDENTIALS` file secret mounted to
  `~/.aws/credentials` for `AWS_PROFILE` use (§6.3 plumbing).
- **Stretch (role/identity):** AWS analog of `DetectAuthTypeFromGCPIdentity` —
  detect ambient IAM role / instance profile / IRSA and use the default AWS
  credential chain with no static secret. Needs a
  `DetectAuthTypeFromAWSIdentity` and a `skipped_when_aws_role_assigned`
  metadata flag mirroring `skipped_when_gcp_service_account_assigned`
  (harness_auth_metadata.go:52, config.yaml:110). Confirmed viable: Claude
  Code honors the default credential chain (§7). Highest value for U2.
- **STS expiry caveat:** session tokens can expire while an agent container is
  still running. Claude Code's doc-sanctioned answer is the `awsAuthRefresh` /
  `awsCredentialExport` settings (settings.json, not env). Out of scope for
  v1; revisit with the stretch goal (OQ2).

### 6.5 Preflight & metadata

There is no provider-specific check in `fabric doctor` — preflight
completeness checks live in the broker/hub create paths (see §6.3) and return
HTTP 202 `MissingEnvVars` (`pkg/hub/errors.go:251`). The declarative config
drives them, so no new preflight code is needed beyond the config itself.
Schema already supports everything used here except the stretch-goal flag
(`pkg/api/harness_auth_metadata.go:18-61`).

---

## 7. Claude Code Bedrock env contract (CONFIRMED)

Verified against official docs
([amazon-bedrock.md](https://code.claude.com/docs/en/amazon-bedrock.md),
[env-vars.md](https://code.claude.com/docs/en/env-vars.md)):

- **Toggle:** `CLAUDE_CODE_USE_BEDROCK=1`.
- **Region:** `AWS_REGION` wins, then `AWS_DEFAULT_REGION`, then the active
  AWS profile's region, then `us-east-1` (fallback chain since v2.1.172;
  older versions require explicit `AWS_REGION`).
- **Credential styles (all supported):** static keys
  (`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`/optional `AWS_SESSION_TOKEN`),
  `AWS_PROFILE` (shared credentials/config files), AWS SSO, ambient IAM
  instance roles / IRSA via the default credential chain, and Bedrock API
  keys via `AWS_BEARER_TOKEN_BEDROCK`.
- **Model selection:** `ANTHROPIC_MODEL` (default is a `us.anthropic.…`
  inference-profile ID), plus `ANTHROPIC_DEFAULT_OPUS_MODEL` /
  `ANTHROPIC_DEFAULT_SONNET_MODEL` / `ANTHROPIC_DEFAULT_HAIKU_MODEL`.
- **Also relevant:** `ANTHROPIC_BEDROCK_BASE_URL` (custom endpoints/gateways),
  `ANTHROPIC_SMALL_FAST_MODEL_AWS_REGION`, `ANTHROPIC_BEDROCK_SERVICE_TIER`
  (`default`/`flex`/`priority`), `AWS_SHARED_CREDENTIALS_FILE` /
  `AWS_CONFIG_FILE` (non-default file locations), `awsAuthRefresh` /
  `awsCredentialExport` (settings.json — credential refresh for SSO/expiring
  creds).
- **Other CLIs:** Gemini CLI and GitHub Copilot CLI have no native Bedrock
  support; Codex/OpenAI is a different ecosystem. OpenCode support is
  unconfirmed (OQ6).

---

## 8. Open Questions / Risks

- **OQ1. v1 credential scope** — bearer token + static keys only, or include
  the file secret (v1.5) in the first PR? (Recommendation: env-only v1; the
  file plumbing in §6.3 is the bulk of the Go diff.)
- **OQ2. STS/session expiry** — document as a limitation in v1, or wire
  `awsAuthRefresh`/`awsCredentialExport` into templates? (Recommendation:
  document; revisit with the IAM stretch.)
- **OQ3. Autodetect strength** — locked to `AWS_BEARER_TOKEN_BEDROCK` /
  `CLAUDE_CODE_USE_BEDROCK` per FR1; is `AWS_PROFILE` too weak to include?
- **OQ4. Region/model matrix** — Bedrock model availability is
  region-specific; validate at create time or document only?
- **OQ5. IAM role detection** — in this effort or a follow-up?
- **OQ6. OpenCode** — confirm whether its CLI supports Bedrock before scoping
  it into G4.
- **Risk:** treating this as "just set a flag" — credential handling (§6.3,
  §6.4) is where the effort and the security review belong.

---

## 9. Rollout & Testing

- **Unit:** extend `pkg/harness/auth_test.go` (detection, required-env);
  add a `bedrock` peer to the bundle-contract fixtures at
  `pkg/harness/testdata/bundle_contract/claude/` (alongside `vertex_ai/`
  `input.json` + `want.json`).
- **Vendoring:** `go generate` + `harnesses/vendored_lib_test.go` green.
- **Integration:** provision a `claude` agent with `bedrock` selected; assert
  the container env overlay, and that Claude Code reaches a Bedrock model.
- **Regression:** api-key / oauth / auth-file / vertex-ai paths unchanged;
  FR5 (no `no_auth` drop-to-shell) covered by a test.
- **Docs:** `docs-site/src/content/docs/local/agent-credentials.md` (the
  provider/auth page), `harnesses/claude/README.md` auth table, and the
  hub-secret examples.

---

## 10. Estimated Effort

- **v1** (config + provision overlay + detection/table arms + tests + docs):
  **small** (~½–1 day) — the contract is confirmed, so no research remains.
- **v1.5** (credentials-file secret plumbing across the five Go switches):
  **small–medium**.
- **Stretch** (IAM identity detection + skip flag): **medium**.
- **OpenCode** (if OQ6 confirms): **small**.

The gating decisions are OQ1/OQ2 (credential scope), not the plumbing.
