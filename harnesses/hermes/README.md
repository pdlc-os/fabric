# Hermes Harness Bundle

Fabric harness configuration for [Hermes Agent](https://github.com/nousresearch/hermes-agent),
Nous Research's AI coding agent (MIT license).

## Install

From a repository checkout:

```sh
fabric harness-config install harnesses/hermes
```

Or directly from GitHub:

```sh
fabric harness-config install github.com/pdlc-os/fabric/tree/main/harnesses/hermes
```

## Auth Modes

| Mode | Env Var | Notes |
|------|---------|-------|
| `api-key` (default) | `ANTHROPIC_API_KEY` | Anthropic key (highest precedence) |
| `api-key` | `OPENAI_API_KEY` | OpenAI key |
| `api-key` | `GOOGLE_API_KEY` | Google AI Studio key |

## Bundle Layout

```
hermes/
  config.yaml       # Harness configuration (provisioner, capabilities, auth)
  provision.py       # Container-side provisioner (pre-start hook)
  capture_auth.py    # Credential capture for no-auth flow
  Dockerfile         # Image build (FROM fabric-base)
  cloudbuild.yaml    # Cloud Build configuration
```

## Build the Image

```sh
# Local Docker build
docker build --build-arg BASE_IMAGE=fabric-base:latest -t fabric-hermes:latest -f Dockerfile .

# Cloud Build
gcloud builds submit --config cloudbuild.yaml .
```
