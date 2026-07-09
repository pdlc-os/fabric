# Codex Harness Bundle

Fabric harness configuration for [Codex](https://github.com/openai/codex),
OpenAI's coding agent CLI.

## Install

From a repository checkout:

```sh
fabric harness-config install harnesses/codex
```

Or directly from GitHub:

```sh
fabric harness-config install github.com/pdlc-os/fabric/tree/main/harnesses/codex
```

## Auth Modes

| Mode | Env / File | Notes |
|------|-----------|-------|
| `api-key` (default) | `CODEX_API_KEY` or `OPENAI_API_KEY` | Codex key takes precedence |
| `auth-file` | `~/.codex/auth.json` | Codex native auth file |

## Bundle Layout

```
codex/
  config.yaml       # Harness configuration (provisioner, capabilities, auth)
  provision.py       # Container-side provisioner (pre-start hook)
  Dockerfile         # Image build (FROM fabric-base)
  cloudbuild.yaml    # Cloud Build configuration
  home/
    .bashrc                    # Shell config with fabric env sourcing
    .codex/config.toml         # Codex client settings (model, otel, etc.)
    .codex/fabric_notify.sh     # Notification hook script
```

## Build the Image

```sh
# Local Docker build
docker build --build-arg BASE_IMAGE=fabric-base:latest -t fabric-codex:latest -f Dockerfile .

# Cloud Build
gcloud builds submit --config cloudbuild.yaml .
```
