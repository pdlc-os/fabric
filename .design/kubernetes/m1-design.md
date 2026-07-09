# Milestone 1: Technical Design - Basic Runtime Connectivity

## Objective
Implement direct Kubernetes Pod management in `KubernetesRuntime` to ensure immediate support for custom images, environment variables, and command propagation. This bypasses the current `SandboxClaim` CRD limitations for this milestone.

## Component Changes

### 1. `pkg/runtime/kubernetes/runtime.go`

**Refactor `Run` Method:**
The current `Run` method creates a `SandboxClaim`. We will branch logic (or replace it entirely for now) to create a `corev1.Pod`.

```go
func (r *KubernetesRuntime) Run(ctx context.Context, config api.RunConfig) (string, error) {
    // 1. Resolve Namespace
    namespace := r.resolveNamespace(config.Labels)

    // 2. Generate Pod Spec
    pod := r.buildPod(namespace, config)

    // 3. Create Pod
    createdPod, err := r.Client.Clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
    if err != nil {
        return "", fmt.Errorf("failed to create pod: %w", err)
    }

    // 4. Wait for Ready
    if err := r.waitForPodReady(ctx, namespace, createdPod.Name); err != nil {
        return createdPod.Name, err
    }

    // 5. Sync Context (Existing logic, maybe tweaked)
    if config.Workspace != "" {
        // ...
    }

    return createdPod.Name, nil
}
```

**New Helper: `buildPod`**

This function transforms `api.RunConfig` into a `*corev1.Pod`.

```go
func (r *KubernetesRuntime) buildPod(namespace string, config api.RunConfig) *corev1.Pod {
    // Command Resolution
    cmd := config.Harness.GetCommand(config.Task, config.Resume)
    
    // Env Resolution
    envVars := []corev1.EnvVar{}
    for _, e := range config.Env {
        // Parse "KEY=VALUE"
        parts := strings.SplitN(e, "=", 2)
        if len(parts) == 2 {
            envVars = append(envVars, corev1.EnvVar{Name: parts[0], Value: parts[1]})
        }
    }

    // Auth Injection (Temporary M1 Solution)
    if config.Auth.GeminiAPIKey != "" {
        envVars = append(envVars, corev1.EnvVar{Name: "GEMINI_API_KEY", Value: config.Auth.GeminiAPIKey})
    }
    // ... handle other keys (Anthropic, Google, etc.)

    return &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      config.Name,
            Namespace: namespace,
            Labels:    config.Labels,
        },
        Spec: corev1.PodSpec{
            Containers: []corev1.Container{
                {
                    Name:            "agent",
                    Image:           config.Image,
                    Command:         cmd, // Harness command overrides entrypoint
                    Env:             envVars,
                    ImagePullPolicy: corev1.PullIfNotPresent, // Or Always
                    WorkingDir:      "/workspace", // Default for now
                    VolumeMounts: []corev1.VolumeMount{
                         // For M1, we might need a /workspace emptyDir if not implicit
                         {Name: "workspace", MountPath: "/workspace"},
                    },
                },
            },
            Volumes: []corev1.Volume{
                {
                    Name: "workspace",
                    VolumeSource: corev1.VolumeSource{
                        EmptyDir: &corev1.EmptyDirVolumeSource{},
                    },
                },
            },
            RestartPolicy: corev1.RestartPolicyNever,
        },
    }
}
```

**New Helper: `waitForPodReady`**
Replaces `waitForReady` (which watched SandboxClaims) with a watcher for Pod status.

### 2. `pkg/k8s/client.go`

No major changes required as `Clientset` is already exposed. We might add a helper wrapper for Pod operations if we want to keep `runtime.go` clean, but direct `Clientset` usage is fine for this stage.

## Verification Steps (QA)

1.  **Setup:**
    *   Set `GEMINI_API_KEY` in local env.
    *   Configure `~/.fabric/settings.json` or flag to use `kubernetes`.

2.  **Test Case 1: Environment Propagation**
    *   Command: `fabric run --runtime kubernetes --image alpine:latest --env MY_VAR=hello "echo $MY_VAR"`
    *   Expectation: Pod starts, logs contain "hello", Pod terminates with Succeeded.

3.  **Test Case 2: Custom Image**
    *   Command: `fabric run --runtime kubernetes --image python:3.9 "python --version"`
    *   Expectation: Logs show Python 3.9 version.

4.  **Test Case 3: Harness Start (Gemini)**
    *   Command: `fabric start my-agent` (assuming Gemini harness).
    *   Action: Inspect Pod YAML.
    *   Expectation: `GEMINI_API_KEY` is present in `spec.containers[0].env`.
