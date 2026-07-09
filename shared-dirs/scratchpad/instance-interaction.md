You are an AI agent whose primary role is to manage and interact with a GCP VM via `gcloud compute ssh --zone "us-central1-a" "fabric-aiopm" --project "deploy-demo-test"`

If you do not have the ssh command already installed in your environment, you will need to install it with apt. You have sudo in this environment, and on the fabric-aiopm GCE VM.

Note: this note was adapted and is re-used from an earlier project about building an A2A bridge - some leftover notes may still be in here and can be deleted for flagged for cleanup.

## VM Details

- **Instance**: `fabric-aiopm`
- **Zone**: `us-central1-a`
- **Project**: `deploy-demo-test`
- **SSH user**: Logs in as a service account (`sa_*`), not as `fabric`. Use `sudo -u fabric bash -c '...'` to run commands as the fabric user, or `sudo` for root-level operations.

## Repository Configuration

The fabric repo is checked out at `/home/fabric/fabric` on the VM.

- **Remote**: `https://github.com/ptone/fabric.git` (origin)
- **Branch**: `fabric/a2a-bridge`
- **Purpose**: This VM is configured for integration testing of the `fabric/a2a-bridge` branch. Changes are pushed from the development workspace to the remote, then pulled down onto the VM.

## Hub Service

- **Service**: `fabric-hub` (systemd)
- **Config directory**: `/home/fabric/.fabric/`
- **Environment file**: `/home/fabric/.fabric/hub.env`
- **Settings**: `/home/fabric/.fabric/settings.yaml`
- **Database**: `/home/fabric/.fabric/hub.db`
- **Service file**: `/etc/systemd/system/fabric-hub.service`
- **Binary**: `/usr/local/bin/fabric`
- **Web UI / API port**: 8080 (behind Caddy reverse proxy)
- **Public URL**: `https://aiopm.projects.fabric-ai.dev`
- **Caddy config**: `/etc/caddy/Caddyfile` (serves `aiopm.projects.fabric-ai.dev`)

### Key hub.env settings
- `FABRIC_MAINTENANCE_REPO_PATH="/home/fabric/fabric"` — points rebuild operations at the local checkout
- `FABRIC_MAINTENANCE_REPO_BRANCH=fabric/chat-tee` — pins rebuilds to this branch

## Common Operations

### Check service status
```bash
gcloud compute ssh --zone "us-central1-a" "fabric-aiopm" --project "deploy-demo-test" --command "sudo systemctl status fabric-hub"
```

### Pull latest code on VM
```bash
gcloud compute ssh --zone "us-central1-a" "fabric-aiopm" --project "deploy-demo-test" --command "sudo -u fabric bash -c 'cd /home/fabric/fabric && git pull origin fabric/chat-tee'"
```

### Rebuild and restart hub
```bash
gcloud compute ssh --zone "us-central1-a" "fabric-aiopm" --project "deploy-demo-test" --command "
sudo -u fabric bash -c 'cd /home/fabric/fabric && git pull origin fabric/a2a-bridge && make web && /usr/local/go/bin/go build -o fabric ./cmd/fabric'
sudo systemctl stop fabric-hub
sudo mv /home/fabric/fabric/fabric /usr/local/bin/fabric
sudo chmod +x /usr/local/bin/fabric
sudo systemctl start fabric-hub
"
```

### View recent logs
```bash
gcloud compute ssh --zone "us-central1-a" "fabric-aiopm" --project "deploy-demo-test" --command "sudo journalctl -u fabric-hub -n 50 --no-pager"
```

### Health check
```bash
gcloud compute ssh --zone "us-central1-a" "fabric-aiopm" --project "deploy-demo-test" --command "curl -s http://localhost:8080/healthz"
```

## Integration Testing Workflow

1. Make changes in the development workspace on branch `fabric/a2a-bridge`
2. Push to remote: `git push origin fabric/a2a-bridge`
3. Pull on VM and rebuild (see commands above), or trigger a rebuild via the hub's admin maintenance UI
4. Test against `https://integration.projects.fabric-ai.dev`


## SSH Notes

- **Do NOT use `--tunnel-through-iap`** — the VM has an external IP (35.232.118.211) and OS Login. Direct SSH works fine.
- The previous instance `fabric-integration` is not in use — always use `fabric-aiopm`
- `integration.projects.fabric-ai.dev` (136.111.240.153) is the OLD VM — do not use
- `aiopm.projects.fabric-ai.dev` (35.232.118.211) is THIS VM
- The hub can also self-rebuild via its admin maintenance page (rebuild-server / rebuild-web tasks), which respect the `FABRIC_MAINTENANCE_REPO_BRANCH` setting


