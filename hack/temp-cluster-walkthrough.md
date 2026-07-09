# Kubernetes Runtime Walkthrough

This guide walks you through testing the Kubernetes runtime in `fabric-agent` using the `agent-sandbox` standard.

## Prerequisites
- `gcloud` CLI installed and authenticated.
- `kubectl` installed.
- A project where you have permission to create GKE clusters.
- The `agent-sandbox` repository cloned locally.

## Step 1: Create the GKE Cluster
Run the provided script to create a GKE Autopilot cluster:

```bash
./hack/create-cluster.sh
```

Wait for the cluster to be ready. This script also configures your local `kubectl` context.

## Step 2: Install Agent Sandbox CRDs and Controllers
We need to install the custom resources and the controller that manages sandboxes.

```bash
# Set the path to where you cloned the agent-sandbox repo
export SANDBOX_REPO="${HOME}/dev/agent-sandbox"

# Apply the CRDs
kubectl apply -f ${SANDBOX_REPO}/k8s/crds/

# Apply RBAC and Core Controller
kubectl apply -f ${SANDBOX_REPO}/k8s/rbac.generated.yaml
kubectl apply -f ${SANDBOX_REPO}/k8s/controller.yaml

# Apply Extensions RBAC
kubectl apply -f ${SANDBOX_REPO}/k8s/extensions-rbac.generated.yaml

# Patch the controller to use a real image and enable extensions
# (The manifests use a ko:// placeholder by default)
kubectl set image statefulset/agent-sandbox-controller \
  agent-sandbox-controller=registry.k8s.io/agent-sandbox/agent-sandbox-controller:v0.1.0 \
  -n agent-sandbox-system

kubectl patch statefulset agent-sandbox-controller \
  -n agent-sandbox-system \
  -p '{"spec": {"template": {"spec": {"containers": [{"name": "agent-sandbox-controller", "args": ["--extensions=true"]}]}}}}'
```

Verify the controllers are running:
```bash
kubectl get pods -n agent-sandbox-system
```

## Step 3: Create a Sandbox Template
<!-- TODO the template resource should prob be just called fabric -->
Agents need a template to define their environment. Create a file named `fabric-template.yaml`. We'll name it `gemini` to match the default fabric template.


```yaml
apiVersion: extensions.agents.x-k8s.io/v1alpha1
kind: SandboxTemplate
metadata:
  name: gemini
  namespace: default
spec:
  podTemplate:
    spec:
      containers:
      - name: agent
        image: python:3.11-slim
        # Ensure /workspace exists for context syncing
        command: ["/bin/sh", "-c", "mkdir -p /workspace && sleep 3600"]
        workingDir: /workspace
```

Apply it:
```bash
kubectl apply -f fabric-template.yaml
```

## Step 4: Initialize Fabric Project and Build
Before running agents, you need to initialize the fabric project to seed the templates.

```bash
# Build the fabric binary
go build -o fabric ./cmd/fabric

# Initialize the project (creates .fabric directory)
./fabric project init
```


## Step 5: Run Fabric with Kubernetes Runtime
Configure Fabric to use the Kubernetes runtime.

```bash
./fabric config set default_runtime kubernetes

# Run a new agent with a task
./fabric run my-k8s-agent "echo 'Hello from Kubernetes!'"
```

## Step 6: Verify the Resources
Check that Fabric created the `SandboxClaim` and that the controller provisioned the `Sandbox` and `Pod`.

```bash
# Check the claim
kubectl get sandboxclaims

# Check the provisioned pod
kubectl get pods -l fabric.agent=true
```

## Step 7: Test Logs and Attach
Test that you can see logs and interact with the agent:

```bash
# See logs
./fabric logs my-k8s-agent

# Attach interactively
./fabric attach my-k8s-agent
```

## Step 8: Cleanup
When finished, you can delete the agent and eventually the cluster.

```bash
# Delete the agent (removes SandboxClaim and local files)
./fabric delete my-k8s-agent

# Delete the cluster
gcloud container clusters delete fabric-agents --region us-central1
```
