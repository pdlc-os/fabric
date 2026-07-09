# Issue: Apple Native Container Does Not Support Attach

## Description
The Apple native `container` CLI (Virtualization.framework backend) does not support a persistent `attach` command that allows re-connecting to the standard input/output of a running container's initial process. It currently only supports `exec` to run new processes in a running container.

This prevents `fabric` from implementing `fabric attach` to let users interact with a running agent session.

## Proposed Solution: Tmux-based Persistence
Since we cannot re-attach to the container's PID 1 directly, we can run the primary application (Gemini CLI) inside a `tmux` session within the container.

### Implementation Details
1.  **Template Update:** Ensure `tmux` is installed in the base image or template used for agents.
2.  **Agent Startup:**
    - Modify the agent's entrypoint or startup command to launch a named `tmux` session:
      ```bash
      tmux new-session -d -s gemini-session 'gemini'
      ```
3.  **Attach Logic:**
    - When a user runs `fabric attach <agent>`, the runtime should use `exec` to attach to the existing `tmux` session:
      ```bash
      container exec -it <container-id> tmux attach-session -t gemini-session
      ```
4.  **Runtime Abstraction:**
    - Update the `Runtime` interface and `apple_container.go` implementation to support this `Attach` flow.
    - If a container isn't running `tmux`, the `Attach` command might need a fallback or to report an error.

## Benefits
- Provides a consistent "attach" experience across runtimes.
- Session persistence: if the TTY is disconnected, the Gemini CLI process continues running inside `tmux`.
- Multi-user/multi-attach support if needed.

## Risks/Challenges
- **Dependencies:** Requires `tmux` to be present in the container.
- **Complexity:** Managing `tmux` sessions (start/stop/cleanup) adds another layer to the agent lifecycle.
- **Escape Sequences:** Ensuring that detaching from `tmux` (Ctrl-B, D) doesn't kill the container or the CLI.

## Related Files
- `pkg/runtime/runtime.go`
- `pkg/runtime/apple_container.go`
- `cmd/attach.go` (to be implemented)
- `.fabric/templates/default/.bashrc` (could be used for auto-attach or setup)
