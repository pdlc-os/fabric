# Fabric Chat App

A standalone service that bridges Google Chat (and future Slack) with the Fabric Hub, enabling users to manage agents and receive notifications directly from their chat workspace. Built as a Google Workspace Add-on (HTTP Service), it runs as both a message broker plugin for real-time agent communication and an API proxy for operational commands.

## Features

- Bidirectional messaging between chat users and Fabric agents
- Agent management via slash commands (`/fabric list`, `/fabric start`, etc.)
- Automatic user identity mapping (chat user to Hub account)
- Space-to-grove linking for scoped interactions
- Real-time notification cards for agent status changes (`COMPLETED`, `ERROR`, `WAITING_FOR_INPUT`, etc.)
- Interactive `ask_user` response flow with inline reply fields
- Per-user notification subscriptions with activity-type filtering

## Prerequisites

- Go 1.25+
- A running Fabric Hub instance
- A GCP project with the Google Chat API enabled
- A GCP service account with:
  - Google Chat API permissions (for sending/receiving messages)
  - Access to the Hub's signing key in GCP Secret Manager (for user impersonation)
  - On a GCE VM, the instance's attached service account can be used via Application Default Credentials (ADC) — no key file needed
- A Hub admin user account for the chat app to authenticate as

## GCP Setup

### 1. Create a GCP Project (or use existing)

```bash
gcloud projects create my-fabric-chat --name="Fabric Chat App"
gcloud config set project my-fabric-chat
```

### 2. Enable Required APIs

```bash
gcloud services enable chat.googleapis.com
gcloud services enable secretmanager.googleapis.com
gcloud services enable artifactregistry.googleapis.com  # if deploying to Cloud Run
gcloud services enable run.googleapis.com                # if deploying to Cloud Run
```

### 3. Create a Service Account

```bash
# Create the service account
gcloud iam service-accounts create fabric-chat-app \
  --display-name="Fabric Chat App"

# Grant access to the Hub's signing key in Secret Manager
gcloud secrets add-iam-policy-binding <HUB_SIGNING_KEY_SECRET> \
  --member="serviceAccount:fabric-chat-app@my-fabric-chat.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"

# Create and download a key file (for local development)
gcloud iam service-accounts keys create chat-sa-key.json \
  --iam-account=fabric-chat-app@my-fabric-chat.iam.gserviceaccount.com
```

### 4. Register as a Workspace Add-on

1. Go to the [Google Cloud Console](https://console.cloud.google.com) > **APIs & Services** > **Google Chat API** > **Configuration**
2. Set **App name** and **Avatar URL** (e.g., "Fabric")
3. Under **Functionality**, enable:
   - Receive 1:1 messages
   - Join spaces and group conversations
4. Under **Connection settings**, select **HTTP Service** as the app type and enter your endpoint URL:
   ```
   https://<YOUR_CHAT_APP_URL>/chat/events
   ```
5. Under **Slash commands**, add:
   | Command ID | Command        | Description                |
   |------------|----------------|----------------------------|
   | 1          | `/fabric`       | Message agents             |
   | 2          | `/fabricAdmin`  | Agent & space admin        |

   Note the numeric **Command ID** assigned by the console — you'll need it for the `command_id_map` configuration.
6. Set the **Card interaction URL** to the same endpoint URL (for backward compatibility with pre-migration cards)
7. Under **Permissions**, configure which users/OUs can install the app
8. Note the **service account email** shown on the configuration page (used for request verification)

## Configuration

Create a YAML configuration file (e.g., `fabric-chat-app.yaml`):

```yaml
hub:
  # Fabric Hub endpoint
  endpoint: "https://hub.example.com"
  # Hub admin user for system-level operations
  user: "chat-app@example.com"
  # Path to the Hub's user signing key file (base64-encoded, 32 bytes).
  # The chat app uses this to mint impersonation tokens for hub users.
  signing_key: "/path/to/user-signing-key"

# Broker plugin RPC server settings.
# The Hub connects to this address as a self-managed plugin.
plugin:
  listen_address: "localhost:9090"

platforms:
  google_chat:
    enabled: true
    # GCP project ID where the Chat app is registered
    project_id: "my-fabric-chat"
    # Optional: path to a service account key file for Google Chat API calls.
    # If omitted, Application Default Credentials (ADC) are used — on a GCE VM
    # this is the instance's attached service account.
    # credentials: "/path/to/chat-sa-key.json"
    # HTTP endpoint for receiving Google Chat events
    listen_address: ":8443"
    # Public URL of this endpoint (used for action URLs in cards and token audience verification)
    external_url: "https://fabric-chat-app-xxxxx.run.app/chat/events"
    # Per-project service account email for request verification (from Chat API config page)
    service_account_email: "chat@my-fabric-chat.iam.gserviceaccount.com"
    # Mapping of numeric command IDs (assigned in Console) to command names
    command_id_map:
      "1": "fabric"
      "2": "fabricAdmin"

  slack:
    enabled: false
    # bot_token: "${SLACK_BOT_TOKEN}"
    # signing_secret: "${SLACK_SIGNING_SECRET}"
    # listen_address: ":8444"

state:
  # SQLite database for user mappings, space links, and subscriptions
  database: "/var/lib/fabric-chat-app/state.db"

notifications:
  # Which agent activities to relay to chat spaces
  trigger_activities:
    - COMPLETED
    - WAITING_FOR_INPUT
    - ERROR
    - STALLED
    - LIMITS_EXCEEDED

logging:
  level: "info"    # debug, info, warn, error
  format: "json"   # json or text
```

Environment variables in the form `${VAR}` or `$VAR` are expanded in the config file before parsing.

### Hub-Side Plugin Configuration

Register the chat app as a self-managed broker plugin in the Hub's settings:

```yaml
# In Hub settings.yaml (added automatically by make install)
plugins:
  broker:
    googlechat:
      self_managed: true
      address: "localhost:9090"
```

## Local Development

```bash
cd extras/fabric-chat-app

# Download dependencies
go mod download

# Create a minimal config for local development
cat > dev-config.yaml <<'EOF'
hub:
  endpoint: "http://localhost:8080"
plugin:
  listen_address: "localhost:9090"
platforms:
  google_chat:
    enabled: true
    project_id: "my-gcp-project"
    # credentials: "./chat-sa-key.json"  # optional if using ADC
    listen_address: ":8443"
    external_url: "https://<YOUR_TUNNEL_URL>/chat/events"
    service_account_email: "chat@my-gcp-project.iam.gserviceaccount.com"
    command_id_map:
      "1": "fabric"
      "2": "fabricAdmin"
state:
  database: "./fabric-chat-app.db"
logging:
  level: "debug"
  format: "text"
EOF

# Run the server
go run ./cmd/fabric-chat-app/ --config dev-config.yaml
```

The app starts two servers:
- **Port 8443** - Google Chat HTTP event endpoint (receives events from Google Chat)
- **Port 9090** - Broker plugin RPC server (receives messages from the Hub)

### Testing Locally with a Tunnel

Google Chat sends events to the configured HTTP endpoint. For local development, use a tunnel service (e.g., `ngrok`, `cloudflared`) to expose port 8443:

```bash
ngrok http 8443
```

Then update both the **HTTP endpoint URL** and the **Card interaction URL** in the Chat API configuration page to use the tunnel URL (e.g., `https://abc123.ngrok.io/chat/events`). Also set `external_url` in your dev config to match.

## Testing

```bash
cd extras/fabric-chat-app
go test ./...
```

## Install on a Provisioned Hub VM

If the Hub was deployed via `scripts/starter-hub/`, the chat app can be installed alongside it with `make install`. This builds the binary, installs a systemd unit, generates the runtime config, patches the Caddyfile for path-based routing, and adds the broker plugin entry to the Hub's `settings.yaml`.

The install is idempotent — re-run it after any hub update (`gce-start-hub.sh --full`) to re-apply patches to files the hub scripts may have overwritten.

### Remote install via `gcloud compute ssh`

The install script requires sudo (for systemd, Caddy, and `/usr/local/bin`). On a starter-hub VM, the SSH user has passwordless sudo while the `fabric` user does not. Run the install remotely:

```bash
# Replace INSTANCE and ZONE with your hub VM values.
# Instance name is "fabric-${HUB_NAME}" (e.g., fabric-gteam).
gcloud compute ssh INSTANCE --zone=ZONE --command '
  sudo -u fabric sh -c "cd /home/fabric/fabric/extras/fabric-chat-app && make build"
  sudo /home/fabric/fabric/extras/fabric-chat-app/install.sh
'
```

### First-time setup

Before running `make install`, create the chat-app env file on the VM:

```bash
gcloud compute ssh INSTANCE --zone=ZONE --command '
  sudo install -m 600 -o fabric -g fabric \
    /home/fabric/fabric/extras/fabric-chat-app/chat-app.env.sample \
    /home/fabric/.fabric/chat-app.env
'
# Then SSH in and edit with your values (project ID, SA email, hub user):
gcloud compute ssh INSTANCE --zone=ZONE
sudo -u fabric nano /home/fabric/.fabric/chat-app.env
```

> **Note:** `CHAT_APP_CREDENTIALS` is optional. On a GCE VM the app uses Application Default Credentials (ADC) from the instance's attached service account. If the service account lacks Chat API permissions, the app prints remediation steps including the required `gcloud` commands at startup.

### On-VM install (if you have sudo)

```bash
cd ~/fabric/extras/fabric-chat-app
make install
```

After install, restart the Hub to pick up the new plugin config:

```bash
sudo systemctl restart fabric-hub
```

Check status:

```bash
sudo systemctl status fabric-chat-app
journalctl -u fabric-chat-app -f
```

## Docker Build

The Dockerfile uses a multi-stage build. It must be built from the repo root because the chat app module has a `replace` directive pointing to the parent Fabric module:

```bash
docker build -t fabric-chat-app -f extras/fabric-chat-app/Dockerfile .
```

Run the container:

```bash
docker run -p 8443:8443 -p 9090:9090 \
  -v /path/to/config.yaml:/etc/fabric-chat-app/config.yaml \
  -v /path/to/chat-sa-key.json:/etc/fabric-chat-app/chat-sa-key.json \
  fabric-chat-app
```

## Deploy to Cloud Run

The included `cloudbuild.yaml` builds, pushes, and deploys the app to Cloud Run.

```bash
gcloud builds submit \
  --config=extras/fabric-chat-app/cloudbuild.yaml \
  --substitutions=_GIT_SHA=$(git rev-parse --short HEAD)
```

Override defaults with substitutions:
- `_REGISTRY` - Artifact Registry path (default: `us-central1-docker.pkg.dev/$PROJECT_ID/fabric`)
- `_SERVICE_NAME` - Cloud Run service name (default: `fabric-chat-app`)
- `_REGION` - Deployment region (default: `us-central1`)

The deployment configures:
- 512 MiB memory, 1 vCPU
- Min 1 / max 3 instances (keeps at least one warm for webhook responsiveness)
- 300s request timeout
- Authentication required (configure IAM for Google Chat to invoke)

### Cloud Run Configuration

After deploying, mount the config file and service account key as secrets or volumes:

```bash
# Store config as a secret
gcloud secrets create fabric-chat-app-config \
  --data-file=config.yaml

# Update the Cloud Run service to mount it
gcloud run services update fabric-chat-app \
  --region=us-central1 \
  --update-secrets=/etc/fabric-chat-app/config.yaml=fabric-chat-app-config:latest
```

Update the **HTTP endpoint URL** and **Card interaction URL** in the Chat API configuration page to point to the Cloud Run service URL (e.g., `https://fabric-chat-app-xxxxx.run.app/chat/events`). Use the same URL as the `external_url` in your config.

### Co-hosting with the Hub behind Caddy

The chat app uses the `/chat/` path prefix so it can share a domain with the Fabric Hub via a reverse proxy. `make install` patches the Caddyfile automatically, but if you're configuring manually, the resulting Caddyfile looks like:

```
fabric.example.com {
    # Chat app (Google Workspace Add-on endpoint)
    handle /chat/* {
        reverse_proxy localhost:8443
    }

    # Hub API and Web UI
    handle {
        reverse_proxy localhost:8080
    }

    tls /etc/letsencrypt/live/fabric.example.com/fullchain.pem /etc/letsencrypt/live/fabric.example.com/privkey.pem
}
```

In this setup, set `external_url` to `https://fabric.example.com/chat/events` and register that as the HTTP endpoint URL in the Chat API configuration.

## Slash Commands

Once the app is running and connected, users interact via `/fabric` in Google Chat:

| Command | Description |
|---------|-------------|
| `/fabric help` | Show available commands |
| `/fabric register` | Link your chat account to your Hub user (auto-matches by email, falls back to device auth) |
| `/fabric unregister` | Remove your chat-to-Hub account link |
| `/fabric link <grove-slug>` | Link the current space to a grove (admin only) |
| `/fabric unlink` | Unlink the current space from its grove (admin only) |
| `/fabric list` | List agents in the linked grove |
| `/fabric status <agent>` | Show agent status card with action buttons |
| `/fabric create <agent>` | Create a new agent |
| `/fabric start <agent>` | Start an agent |
| `/fabric stop <agent>` | Stop an agent |
| `/fabric delete <agent>` | Delete an agent (with confirmation) |
| `/fabric logs <agent>` | Show recent agent logs |
| `/fabric message <agent> <text>` | Send a message to an agent (supports `--thread <id>`) |
| `/fabric subscribe <agent>` | Subscribe to agent notifications (with activity filter dialog) |
| `/fabric unsubscribe <agent>` | Unsubscribe from agent notifications |

You can also @mention the bot to send messages to agents:

```
@Fabric tell deploy-agent to check the staging cluster
```

## Architecture

```
Google Chat ──HTTP events──> fabric-chat-app ──Hub API──> Fabric Hub
                                  │  │                       │
                   sync responses─┘  │◄──broker plugin (RPC)─┘
                                     │
                                 SQLite (local state)
```

The app uses the **Workspace Add-on HTTP Service** model. Google Chat sends events as nested `EventObject` payloads (with `commonEventObject` and `chat` sub-objects). Interactive features like dialogs and card updates use **synchronous JSON responses** in the HTTP response body, while background notifications continue to use the async Chat REST API.

The chat app operates under three identity contexts:

1. **Hub admin user** - System-level Hub operations (notification subscriptions, grove lookups)
2. **GCP service account** - Infrastructure access (Secret Manager for signing keys, Google Chat API)
3. **Impersonated chat users** - User-initiated commands are executed as the linked Hub user via short-lived scoped tokens

## Ports

| Port | Purpose |
|------|---------|
| 8443 | Google Chat HTTP event endpoint |
| 9090 | Broker plugin RPC server |

## Health Check

The app exposes a `/chat/healthz` endpoint on the event server (port 8443) that checks Hub API reachability, broker plugin connection, and database accessibility.
