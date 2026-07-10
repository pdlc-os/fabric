#!/usr/bin/env python3
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Claude Code container-side provisioner.

Runs inside the agent container during the pre-start lifecycle hook, invoked
by `fabrictool harness provision --manifest ...`. The host-side
ContainerScriptHarness has already:

  * Staged this script and config.yaml under $HOME/.fabric/harness/.
  * Written inputs/auth-candidates.json with the env-var names + paths to
    secret-value files under $HOME/.fabric/harness/secrets/<NAME>.
  * Mounted any auth file (e.g. ~/.claude/.credentials.json) at the declared
    container_path, when auth-file mode is in use.
  * Mounted ADC credentials when vertex-ai mode is in use.

This script's job:

  1. Determine which auth method Claude Code will use, honoring an explicit
     selection if present and otherwise applying the same precedence as the
     compiled harness:
         ANTHROPIC_API_KEY > CLAUDE_CODE_OAUTH_TOKEN > auth-file > bedrock
         > vertex-ai.
  2. For api-key auth, pre-approve the key by writing the last 20 chars of the
     API key as a fingerprint in .claude.json's customApiKeyResponses so
     Claude Code does not prompt for confirmation.
  3. Update .claude.json project paths to point at the container workspace.
  4. Translate universal MCP servers into Claude Code's native mcpServers
     format in .claude.json.
  5. Write outputs/resolved-auth.json describing the chosen method.
  6. Write outputs/env.json with env vars to project into the harness process
     (e.g. ANTHROPIC_API_KEY, CLAUDE_CODE_OAUTH_TOKEN, or Vertex AI vars).

The script is intentionally stdlib-only so it works on any container image
that ships python3 (declared in config.yaml's required_image_tools).
"""

from __future__ import annotations

import json
import os
import sys
from typing import Any

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import fabric_harness

assert fabric_harness.INTERFACE_VERSION >= 2, (
    "fabric_harness.py INTERFACE_VERSION is too old "
    f"(got {fabric_harness.INTERFACE_VERSION}, need >= 2); "
    "this is a staging bug — the host should have staged a compatible library"
)

CLAUDE_JSON_FILE = "~/.claude.json"
CLAUDE_AUTH_FILE = "~/.claude/.credentials.json"

AUTH = fabric_harness.AuthSpec(
    harness="claude",
    methods=[
        fabric_harness.env_method("api-key", any_of=["ANTHROPIC_API_KEY"]),
        fabric_harness.env_method("oauth-token", any_of=["CLAUDE_CODE_OAUTH_TOKEN"],
                                 hint="set CLAUDE_CODE_OAUTH_TOKEN (generate with `claude setup-token`)"),
        fabric_harness.file_method("auth-file", path=CLAUDE_AUTH_FILE, secret_key="CLAUDE_AUTH"),
        # env_fallback so credentials that arrive as plain container env
        # (settings env maps, ambient-role marker planted by the host) can
        # satisfy the match; staged secrets still win via two-pass matching.
        # CLAUDE_CODE_USE_BEDROCK is the ambient-role marker: with an IAM
        # execution role the default credential chain needs no material at
        # all, so the toggle itself is the only signal present.
        fabric_harness.env_method("bedrock",
                                 any_of=["AWS_BEARER_TOKEN_BEDROCK", "AWS_ACCESS_KEY_ID", "AWS_PROFILE",
                                         "CLAUDE_CODE_USE_BEDROCK"],
                                 env_fallback=True,
                                 hint="provide AWS credentials for Amazon Bedrock: AWS_BEARER_TOKEN_BEDROCK "
                                      "(Bedrock API key), AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY, "
                                      "AWS_PROFILE with a mounted ~/.aws, or an ambient IAM role "
                                      "(CLAUDE_CODE_USE_BEDROCK=1) — plus AWS_REGION"),
        fabric_harness.env_method("vertex-ai",
                                 all_of=["GOOGLE_CLOUD_PROJECT"],
                                 any_of=["GOOGLE_CLOUD_LOCATION", "GOOGLE_CLOUD_REGION"],
                                 hint="provide GOOGLE_CLOUD_PROJECT + GOOGLE_CLOUD_LOCATION/GOOGLE_CLOUD_REGION "
                                      "(with ADC or GCP service account) for Vertex AI"),
    ],
)

CLAUDE_MCP_MAPPING = {
    "transport_field": "type",
    "transport_map": {
        "stdio": "stdio",
        "sse": "sse",
        "streamable-http": "streamable-http",
    },
    "global_config_file": fabric_harness.expand_path(CLAUDE_JSON_FILE),
    "global_config_path": "mcpServers",
    "project_config_file": fabric_harness.expand_path(CLAUDE_JSON_FILE),
    "project_config_path": "projects.{workspace}.mcpServers",
}


def _resolve(ctx: fabric_harness.ProvisionContext, name: str) -> str:
    """Read a secret value, falling back to a ${VAR} placeholder."""
    val = ctx.read_secret(name)
    return val if val else "${" + name + "}"


def _apply_api_key_approval(ctx: fabric_harness.ProvisionContext, api_key: str) -> None:
    """Pre-approve the API key in .claude.json.

    Writes the last 20 characters of the key as a fingerprint in
    customApiKeyResponses.approved so Claude Code does not prompt for
    confirmation. Mirrors ClaudeCode.ApplyAuthSettings in claude_code.go.
    """
    if not api_key:
        return

    fingerprint = api_key[-20:] if len(api_key) > 20 else api_key
    claude_json_path = fabric_harness.expand_path(CLAUDE_JSON_FILE)

    cfg: dict[str, Any] = {}
    if os.path.isfile(claude_json_path):
        try:
            cfg = fabric_harness.load_json(claude_json_path) or {}
        except (OSError, json.JSONDecodeError):
            cfg = {}
    if not isinstance(cfg, dict):
        cfg = {}

    cfg["customApiKeyResponses"] = {
        "approved": [fingerprint],
        "rejected": [],
    }

    fabric_harness.atomic_write_json(claude_json_path, cfg)


def _update_project_paths(ctx: fabric_harness.ProvisionContext) -> None:
    """Update .claude.json project paths to point at the container workspace.

    Mirrors ClaudeCode.provisionClaudeJSON in claude_code.go. Takes the first
    existing project entry's settings and re-keys it to the container workspace
    path. If no project entries exist, creates a default settings map.
    """
    claude_json_path = fabric_harness.expand_path(CLAUDE_JSON_FILE)

    cfg: dict[str, Any] = {}
    if os.path.isfile(claude_json_path):
        try:
            cfg = fabric_harness.load_json(claude_json_path) or {}
        except (OSError, json.JSONDecodeError):
            cfg = {}
    if not isinstance(cfg, dict):
        cfg = {}

    projects = cfg.get("projects")
    if not isinstance(projects, dict):
        projects = {}

    project_settings: Any = None
    for v in projects.values():
        project_settings = v
        break

    if project_settings is None:
        project_settings = {
            "allowedTools": [],
            "mcpContextUris": [],
            "mcpServers": {},
            "enabledMcpjsonServers": [],
            "disabledMcpjsonServers": [],
            "hasTrustDialogAccepted": True,
            "projectOnboardingSeenCount": 1,
            "hasClaudeMdExternalIncludesApproved": False,
            "hasClaudeMdExternalIncludesWarningShown": False,
            "exampleFiles": [],
        }

    cfg["projects"] = {ctx.workspace: project_settings}
    fabric_harness.atomic_write_json(claude_json_path, cfg)


def _build_env_overlay(ctx: fabric_harness.ProvisionContext, auth: fabric_harness.ResolvedAuth) -> dict[str, str]:
    """Build the env vars overlay for outputs/env.json."""
    if auth.method == "api-key" and auth.env_key:
        return {auth.env_key: _resolve(ctx, auth.env_key)}
    if auth.method == "oauth-token":
        return {"CLAUDE_CODE_OAUTH_TOKEN": _resolve(ctx, "CLAUDE_CODE_OAUTH_TOKEN")}
    if auth.method == "vertex-ai":
        region_key = auth.env_key or "GOOGLE_CLOUD_REGION"
        return {
            "CLAUDE_CODE_USE_VERTEX": "1",
            "ANTHROPIC_VERTEX_PROJECT_ID": _resolve(ctx, "GOOGLE_CLOUD_PROJECT"),
            "CLOUD_ML_REGION": _resolve(ctx, region_key),
        }
    if auth.method == "bedrock":
        # Pass through whichever staged AWS material is present; absent keys
        # are skipped (no placeholder) since each credential style uses a
        # different subset. Model pins (ANTHROPIC_MODEL and friends) are not
        # auth material — they flow via the template env, not this overlay.
        overlay = {"CLAUDE_CODE_USE_BEDROCK": "1"}
        for key in ("AWS_REGION", "AWS_DEFAULT_REGION",
                    "AWS_BEARER_TOKEN_BEDROCK",
                    "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
                    "AWS_SESSION_TOKEN", "AWS_PROFILE"):
            val = ctx.read_secret(key)
            if val:
                overlay[key] = val
        return overlay
    return {}


def provision(ctx: fabric_harness.ProvisionContext) -> None:
    auth = ctx.select_auth(AUTH)

    try:
        _update_project_paths(ctx)
    except OSError as exc:
        raise fabric_harness.ProvisionError(f"failed to update project paths: {exc}") from exc

    if auth.method == "api-key" and auth.env_key:
        api_key_value = ctx.read_secret(auth.env_key)
        if api_key_value:
            try:
                _apply_api_key_approval(ctx, api_key_value)
            except OSError as exc:
                ctx.warn(f"failed to write API key approval: {exc}")

    env = _build_env_overlay(ctx, auth)
    extra: dict[str, Any] | None = None
    if auth.method == "vertex-ai":
        extra = {"vertex_ai": True}
    elif auth.method == "bedrock":
        extra = {"bedrock": True}
    ctx.write_outputs(auth, env=env, extra=extra)

    mcp_mapping = dict(CLAUDE_MCP_MAPPING)
    mcp_mapping["project_config_path"] = f"projects.{ctx.workspace}.mcpServers"
    try:
        count = fabric_harness.apply_mcp_servers_simple(ctx.bundle_dir, mcp_mapping, ctx.workspace)
    except (OSError, ValueError) as exc:
        ctx.warn(f"mcp merge failed: {exc}")
        count = 0
    if count > 0:
        ctx.info(f"applied {count} mcp server(s)")


if __name__ == "__main__":
    fabric_harness.run("claude", provision)
