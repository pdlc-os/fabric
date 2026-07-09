# Claude Harness Bundle

Fabric harness configuration for [Claude Code](https://claude.ai/code),
Anthropic's coding agent CLI.

## Install

From a repository checkout:

```sh
fabric harness-config install harnesses/claude
```

Or directly from GitHub:

```sh
fabric harness-config install github.com/pdlc-os/fabric/tree/main/harnesses/claude
```

## Auth Modes

| Mode | Env / File | Notes |
|------|-----------|-------|
| `api-key` (default) | `ANTHROPIC_API_KEY` | Direct API access |
| `oauth-token` | `CLAUDE_CODE_OAUTH_TOKEN` | Generate with `claude setup-token` |
| `auth-file` | `~/.claude/.credentials.json` | Claude native credentials file |
| `vertex-ai` | `GOOGLE_CLOUD_PROJECT` + `GOOGLE_CLOUD_REGION` | Vertex AI with ADC or service account |
| `bedrock` | `AWS_REGION` + one of `AWS_BEARER_TOKEN_BEDROCK` / `AWS_ACCESS_KEY_ID`+`AWS_SECRET_ACCESS_KEY` / `AWS_PROFILE` (with `~/.aws` mounted) | Amazon Bedrock; sets `CLAUDE_CODE_USE_BEDROCK=1` |

## Bundle Layout

```
claude/
  config.yaml        # Harness configuration (provisioner, capabilities, auth)
  provision.py        # Container-side provisioner (pre-start hook)
  capture_auth.py     # Interactive auth capture script
  Dockerfile          # Image build (FROM fabric-base)
  init-firewall.sh    # Network firewall setup for the container
  cloudbuild.yaml     # Cloud Build configuration
  home/
    .bashrc                     # Shell config with fabric env sourcing
    .claude.json                # Claude Code settings template
    .claude/settings.json       # Claude Code settings (hooks, permissions)
```

## Build the Image

```sh
# Local Docker build
docker build --build-arg BASE_IMAGE=fabric-base:latest -t fabric-claude:latest -f Dockerfile .

# Cloud Build
gcloud builds submit --config cloudbuild.yaml .
```
