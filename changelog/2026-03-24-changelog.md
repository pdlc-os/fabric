# Release Notes (2026-03-24)

This update introduces a major overhaul of the agent visualization system, moving to a dynamic log-driven model, alongside the debut of the `fs-watcher` tool for granular filesystem activity tracking.

## 🚀 Features

* **New `fs-watcher` Tool:** Introduced a standalone Go tool for monitoring host directories using Linux `fanotify`. It attributes filesystem events (create, modify, delete, rename) to specific agents by correlating PIDs with Docker container labels. The tool supports grove-based auto-discovery, event debouncing, and `.gitignore`-style path filtering, outputting events in NDJSON format.
* **Log-Driven Agent Visualization:** Re-engineered the visualization engine to discover agents and files dynamically from log events (including `fabric-agents`, `fabric-messages`, and `fabric-server`) instead of relying on the filesystem. This allows for real-time, event-accurate playback and synthetic lifecycle event generation.
* **Enhanced Agent & File Animations:**
    * **Agent Lifecycle:** Added dynamic ring rebalancing with stable placement and optimal rotation.
    * **Message Ripples:** Implemented broadcast message ripple animations with content-based deduplication and improved coordinate alignment.
    * **File Events:** Introduced file reveal animations and "read prominence" particles that travel from files to agents for Read/Grep/Glob operations.
* **Workspace Visualization Context:** Added support for a `/workspace` root node as a tree anchor in the visualization graph. Files are now extracted from all tool events containing a `file_path`.
* **Activity Detection & Playback:** Added support for detecting agent deletion requests and implemented snapshot-based seeking for playback navigation.

## 🐛 Fixes

* **Visualization Refinements:**
    * **Beam Attribution:** Resolved issues with beam attribution for Gemini agents when using the `run_shell_command` tool.
    * **Canvas Stability:** Fixed a bug where the overlay canvas was destroyed by `force-graph` initialization.
    * **Coordination:** Improved coordination between beams and the agent ring using ref-counted freezing and spread targets.
    * **Data Integrity:** Improved message deduplication, agent name resolution, and added diagnostic logging for better troubleshooting.
* **Infrastructure & CI:** Configured git identity for `TestDetectDefaultBranch` to ensure stable CI execution.
* **Startup Logging:** Added guards against short agent IDs in startup log output to prevent formatting issues.
