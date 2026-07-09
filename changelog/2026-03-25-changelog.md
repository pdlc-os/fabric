# Release Notes (2026-03-25)

This period focused on completing the multi-phase rollout of the Git-Workspace Hybrid system, enabling seamless integration between local git workspaces and containerized agent environments.

## 🚀 Features

* **Git-Workspace Hybrid Groves:** Completed the full 5-phase implementation of hybrid groves. This major architectural update includes:
    * **Automated Provisioning:** Host-side cloning and agent environment setup are now unified.
    * **Credential Standardization:** Unified `fabrictool credential-helper` for GitHub App token management across all agent types.
    * **Web UI Integration:** Full support for managing hybrid workspaces and viewing structured git error guidance (Auth, Network, NotFound) in the dashboard.
* **Agent Visualizer (agent-viz) Enhancements:**
    * Added `--fs-log` flag to use the filesystem watcher as a real-time event source.
    * Introduced `--max-depth` flag to limit file graph complexity.
    * Now captures file-read events via `FAN_ACCESS` for more granular activity tracking.
* **Raw Message Support:** Added a new raw message type for literal tmux `send-keys` delivery, enabling direct, uninterpreted terminal control for automation scripts.

## 🐛 Fixes

* **Agent Isolation:** Scoped agent identities by grove ID to prevent cross-grove name collisions. This ensures that agents with the same name (e.g., "orchestrator") in different groves do not interfere with each other's containers or logs.
* **Agent Reliability & Recovery:**
    * **State Persistence:** Agent environment variables are now written to a persistent file to ensure reliable recovery after a harness process restart.
    * **Auth Propagation:** Fixed a regression where agent auth tokens were not correctly propagated upon restart.
    * **Shell Stability:** Prevented environment variable loss by managing `GEMINI_CLI_NO_RELAUNCH` state more effectively.
* **GitHub Integration:** Auto-associates GitHub App installations at grove creation time, streamlining the authentication flow for private repositories.
* **Workspace Discovery:**
    * Improved workspace mount detection during grove discovery.
    * Suppressed duplicate "added watch" logs for shared workspace directories.
    * Fixed a bug where the system would exit prematurely if a grove had no running agents.
* **Visualizer Polish:** Reduced file label clutter in the graph and added a show/hide toggle for better navigation. Fixed "phantom beams" caused by duplicate slug-based agent entries.
