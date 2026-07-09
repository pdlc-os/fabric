# Release Notes (March 28, 2026)

This release marks a major milestone with the full rollout of bidirectional messaging between humans and agents, the introduction of a new hybrid workspace model for git-backed groves, and a revamped permission system that simplifies multi-agent orchestration.

## 🚀 Features
* **Bidirectional Human-Agent Messaging:**
    * **Integrated Inbox:** Agents can now send direct, persistent messages to humans. These are stored in a new Hub-managed message store, independent of external logging providers.
    * **Web Inbox Tray:** A new notification tray in the web UI provides real-time access to agent messages with unread counts and one-click acknowledgement.
    * **Human Inbox CLI:** Introduced the `fabric messages` command group (aliases: `msgs`, `inbox`) to list, read, and manage agent communications from the terminal.
    * **Broker Integration:** Messages are now routed through the existing `MessageBrokerProxy`, enabling real-time delivery via SSE and external channels like Slack or webhooks.
    * **Dual-Action `ask_user`:** The `fabrictool status ask_user` command now simultaneously updates agent state and sends an explicit message to the human inbox, ensuring questions are never missed.
* **Git-Workspace Hybrid Groves:**
    * **Hybrid Infrastructure:** Introduced a new grove type that combines the persistence of a remote git repository with a high-performance local workspace on the host.
    * **Automated Provisioning:** Implemented a full multi-phase lifecycle for hybrid groves, including host-side cloning, automated agent provisioning, and standardized credential management.
* **Transitive Access Control (Ancestry Chain):**
    * **Ancestry-Based Permissions:** Agents now track their "ancestry"—the chain of creator IDs from the root user down to the parent agent. 
    * **Simplified Orchestration:** Any principal in an agent's ancestry chain now has transitive access to that agent, eliminating the need for complex, manual permission grants in multi-agent swarms.
* **Agent Visualizer & FS-Watcher:**
    * **Filesystem Telemetry:** Introduced the `fs-watcher` tool (utilizing `FAN_ACCESS`) to capture and visualize agent filesystem activity in real-time.
    * **Enhanced Animations:** Added sophisticated new animations for agent lifecycle events, file creation, and "message ripples" to better illustrate agent coordination.
    * **Visualizer Polish:** Added `--max-depth` for graph complexity control and improved the stability of the force-graph layout during rapid updates.
* **Web Terminal & PTY Enhancements:**
    * **Extended Key Support:** Standardized the forwarding of `CSI u` sequences (e.g., `Shift+Enter`), ensuring full compatibility with modern CLI applications and editors running within the web terminal.
    * **Improved Interactivity:** Optimized mouse selection and shift-drag behavior for macOS terminal users and standardized `TERM=xterm-256color` for all web sessions.
* **Documentation & Branding:**
    * **README Overhaul:** Refreshed the project documentation with a focus on "deep agents" and included new visual proof points of agent coordination.

## 🐛 Fixes
* **Rootless Podman Support:** Resolved critical initialization and execution issues when running Fabric in rootless Podman environments, including UID/GID mapping and user propagation.
* **Agent Identity Security:** Scoped agent identities strictly by grove to prevent ID collisions and strengthened environment variable persistence in Gemini and other harnesses.
* **Auth & Recovery:** Fixed a regression in agent auth token propagation, ensuring agents remain authenticated across harness process recoveries and restarts.
* **UI Stability:** Resolved issues with missing Shoelace icons, improved dark theme consistency for list items, and fixed a bug where force-graph re-initialization could destroy active canvas overlays.
