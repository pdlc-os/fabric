# OpenCode Harness Bundle

Fabric harness configuration for [OpenCode](https://opencode.ai), an open-source
AI coding assistant.

## Install

From a repository checkout:

```sh
fabric harness-config install harnesses/opencode
```

Or directly from GitHub:

```sh
fabric harness-config install github.com/pdlc-os/fabric/tree/main/harnesses/opencode
```

## Auth Modes

| Mode | Env / File | Notes |
|------|-----------|-------|
| `api-key` (default) | `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` | Anthropic key takes precedence |
| `auth-file` | `~/.local/share/opencode/auth.json` | OpenCode native auth file |

## Bundle Layout

```
opencode/
  config.yaml       # Harness configuration (provisioner, capabilities, auth)
  provision.py       # Container-side provisioner (pre-start hook)
  Dockerfile         # Image build (FROM fabric-base)
  cloudbuild.yaml    # Cloud Build configuration
  home/
    .config/opencode/opencode.json   # OpenCode client settings
```

## Build the Image

```sh
# Local Docker build
docker build --build-arg BASE_IMAGE=fabric-base:latest -t fabric-opencode:latest -f Dockerfile .

# Cloud Build
gcloud builds submit --config cloudbuild.yaml .
```
