# Release Notes (2026-04-02)

This release focuses on a significant architectural overhaul of the template import system, moving from a slow container-based bootstrap process to a direct, high-performance server-side implementation. It also introduces a new specialized agent skill for team creation and a sophisticated multi-agent poker simulation example.

## ⚠️ BREAKING CHANGES
* **API Removal:** The `POST /api/v1/groves/{groveId}/sync-templates` endpoint has been removed. It is replaced by `POST /api/v1/groves/{groveId}/import-templates`.
* **Synchronous UI Flow:** The "Load Templates" button in the Web UI now performs a synchronous server-side import instead of spawning a background agent. Any external scripts polling for a sync agent will need to be updated.

## 🚀 Features
* **Direct Server-Side Template Import:** Replaced the heavyweight bootstrap container agent with a direct Hub-side import mechanism. This significantly reduces template loading times and provides immediate feedback in the Web UI.
* **Enhanced Template Discovery & Deep Path Support:**
    * **Deep GitHub Paths:** Support for importing from specific subdirectories (e.g., `/tree/main/path/to/templates`) rather than just the repository root.
    * **Expanded Protocol Support:** Added support for importing templates from archives (`.zip`, `.tar.gz`) and rclone URIs (`:gcs:bucket/path`).
    * **Native Fabric Templates:** Support for direct "copy" imports of native fabric templates containing `fabric-agent.yaml`, skipping the conversion process.
* **New Agent Skill: `team-creation`:** A specialized skill for generating coordinated multi-agent template sets. It simplifies the creation of orchestrator-worker patterns and provides best-practice guidance for agent-to-agent communication.
* **Multi-Agent Example: `agent-poker`:** A complete Texas Hold'em poker simulation featuring:
    * **Dealer Role:** Manages game state, deals cards using a persistent Python-based deck script, and enforces rules.
    * **Player Role:** Implements private strategies and allows for (detectable) cheating attempts.
    * **Auditor Role:** Independently validates the integrity of the game by tracking all dealt cards and monitoring broadcast actions for violations.

## 🐛 Fixes
* **Agent Skill Refinement:** Improved guidance on the `--notify` flag and added missing documentation for the `fabric messages` CLI command.
* **Template Examples:** Resolved broken URLs in example templates and updated them to the new import workflow.
