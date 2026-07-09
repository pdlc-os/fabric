# Milestone 1 Walkthrough: Kubernetes Runtime Testing

This guide describes how to manually test the Milestone 1 implementation of the Kubernetes runtime using a GKE Autopilot cluster.

## Prerequisites

1.  **GKE Cluster**: Ensure you have created a cluster using the hack script:
    ```bash
    ./hack/create-cluster.sh
    ```
2.  **Kubectl Configured**: Verify that `kubectl` is pointing to the correct cluster:
    ```bash
    kubectl cluster-info
    ```
3.  **Built fabric**: Ensure you have a recent build of `fabric-agent` in your path (or use `go run main.go`).

## Configuration

To use the Kubernetes runtime, you can either update your global settings or use a local grove configuration.

### Option A: Global Settings
Update your global settings to use `kubernetes` as the default runtime:
```bash
fabric config set default_runtime kubernetes --global
```

### Option B: Local Grove Settings
In your current project directory (grove):
```bash
fabric config set default_runtime kubernetes
```

*(Optional)* Specify a namespace (defaults to `default`):
```bash
fabric config set kubernetes.default_namespace fabric-test
```
If you use a custom namespace, ensure it exists: `kubectl create namespace fabric-test`

## Testing Lifecycle Operations

### 1. Start an Agent
Launch a new agent pod in the cluster. This will sync your current workspace to the pod's `/workspace` directory.
```bash
fabric start --name k8s-test
```

**Verify via kubectl:**
```bash
kubectl get pods
```
You should see a pod named `k8s-test` (or similar) in the `Pending` then `Running` state.

### 2. List Agents
Verify the agent appears in the fabric list:
```bash
fabric list
```
The runtime should be identified as `kubernetes`.

### 3. Attach to the Agent
Open an interactive shell inside the agent pod:
```bash
fabric attach k8s-test
```
Inside the shell, verify:
- You are in `/workspace`.
- Your local files have been synced.
- Environment variables (like `GEMINI_API_KEY` if set) are available.

### 4. Check Logs
View the logs of the agent container:
```bash
fabric logs k8s-test
```

### 5. Stop/Delete the Agent
Remove the agent from the cluster:
```bash
fabric stop k8s-test
```

**Verify via kubectl:**
```bash
kubectl get pods
```
The pod should be terminated and removed.

## Troubleshooting

- **Image Pull Errors**: Milestone 1 uses a default image or whatever is specified in your template. Ensure the image is accessible from your GKE cluster. GKE Autopilot requires images to be in a registry it can access (like Artifact Registry).
- **Permissions**: Ensure your `gcloud` identity has sufficient permissions to create Pods in the cluster.
- **KUBECONFIG**: The runtime uses the `KUBECONFIG` environment variable or the default `~/.kube/config`.
