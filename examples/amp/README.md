# Amp Harness Example

An example [Amp](https://ampcode.com) harness for fabric, delivered entirely
through the `examples/` directory — no Go code changes required. This serves as
both a working Amp integration and a reference implementation for community
harness authors following the same pattern.

## Prerequisites

- Docker (or a compatible container runtime)
- An Anthropic API key (`ANTHROPIC_API_KEY`) or Amp API key (`AMP_API_KEY`)
- A built `fabric-base` image

## Quick Start

```bash
# 1. Build the container image
docker build -t fabric-amp examples/amp/

# 2. Install the harness-config (grove-level or global)
cp -r examples/amp .fabric/harness-configs/amp
# OR install globally:
# cp -r examples/amp ~/.fabric/harness-configs/amp

# 3. Copy the template into your grove's .fabric directory
cp -r examples/amp/templates/amp .fabric/templates/amp

# 4. Start an agent
AMP_API_KEY=sk-ant-... fabric start my-researcher --template amp

# With an initial task:
fabric start my-researcher --template amp --task "Review the auth module for security issues"

# Resume a stopped agent:
fabric start my-researcher

# Attach to a running agent:
fabric attach my-researcher
```

## Auth

Auth is resolved by the `provision.py` script at container start time, in
precedence order:

| Priority | Variable | Notes |
|---|---|---|
| 1 | `AMP_API_KEY` | Amp-native key or Sourcegraph access token |
| 2 | `ANTHROPIC_API_KEY` | Normalized to `AMP_API_KEY` before launch |

The provisioner reads the secret value from the fabric-managed secret staging
area and injects it as `AMP_API_KEY` into the agent's environment regardless of
which source variable was supplied.

## Directory Structure

`examples/amp/` is the harness-config artifact — the entire directory can be
installed directly (or via a future `fabric harness-config install <url>`).

```
examples/amp/
├── README.md                    # This file
├── Dockerfile                   # Container image (based on fabric-base)
├── config.yaml                  # Declarative harness metadata
├── provision.py                 # Container-side provisioning script
├── home/                        # Home directory overlay
│   ├── .bashrc                  # PATH setup
│   └── .config/
│       └── amp/
│           └── settings.json    # Amp defaults
└── templates/
    └── amp/
        ├── fabric-agent.yaml     # Template definition
        └── agents.md            # Agent instructions (fabrictool lifecycle)
```

## How It Works

1. `fabric start` resolves the `amp` template → finds `default_harness_config: amp`
   → loads `harness-configs/amp/config.yaml`.
2. The harness config specifies `provisioner.type: container-script` → the
   generic `ContainerScriptHarness` handles provisioning. No Amp-specific Go
   code exists.
3. At container start, `fabrictool init` runs the `pre-start` lifecycle hook,
   which invokes `provision.py`:
   - Resolves auth from `inputs/auth-candidates.json`
   - Reads the API key from the staged secret file
   - Projects `AMP_API_KEY` into `outputs/env.json`
   - Reconciles `~/.config/amp/settings.json` with required defaults
4. `fabrictool init` loads `outputs/env.json` into the child environment and
   launches `amp -x "<task>"` (or `amp threads continue` on resume).

## Capabilities

| Feature | Support |
|---|---|
| Task delivery (`--task`) | Yes |
| Resume (`fabric start` without task) | Yes — `amp threads continue` |
| Attach (`fabric attach`) | Yes — tmux session |
| System prompt | Partial — prepended to `AGENT.md` |
| Agent instructions | Yes — written to `AGENT.md` |
| Turn limits | No — no hook dialect for turn events |
| Model call limits | No — no hook dialect for model events |
| Duration limits | Yes |
| Telemetry (OTEL) | No — Amp has no native OTEL integration |

## Notes

- `amp.dangerouslyAllowAll: true` is set in `settings.json` to permit
  non-interactive file and shell operations inside the container.
- Amp's OS keychain (`@napi-rs/keyring`) is not available in containers;
  the provisioner uses the `AMP_API_KEY` env var exclusively.
- To use `--stream-json` for structured event logging, add it to
  `command.base` in `config.yaml` (e.g. `["amp", "--stream-json"]`).
