# Release Notes (March 21, 2026)

This release focuses on stabilizing Kubernetes agent operations, enhancing GitHub App integration, and providing more robust management tools for brokers and runtimes.

## ⚠️ BREAKING CHANGES
* **Agent CLI:** The `fabric` CLI now explicitly blocks resource management commands (e.g., `create`, `delete`, `message`) when executed inside an agent container that lacks a reachable Hub endpoint. This prevents silent failures and guides users to configure `FABRIC_HUB_ENDPOINT` correctly. Informational commands like `version`, `help`, and `doctor` remain available.

## 🚀 Features
* **Broker & Runtime Management:**
    * **Runtime Brokers:** Introduced a new tab in Grove settings for managing auxiliary runtimes and broker plugins.
    * **Broker Health:** Implemented inbound endpoints and health checks for broker plugins to improve reliability and monitoring.
    * **Activity States:** Added a dedicated **'blocked'** state for agents to clearly communicate intentional wait states (e.g., waiting for external triggers).
* **GitHub App Integration:**
    * **Direct Access:** Added "Configure GitHub App" links to the user profile sidebar and access tokens page.
    * **Git Connectivity:** Enhanced Grove views with GitHub remote links and tooltips for GitHub-connected resources.
    * **Token Sync:** Implemented GitHub App token minting directly from source Groves to streamline template synchronization.
* **Template Sync & External Repos:**
    * **External Git Support:** Added support for loading templates from external Git repositories for non-Git based Groves.
    * **Input Flexibility:** Improved the template sync UI to accept bare host/org/repo URLs and automatically refresh lists upon success.
* **Enhanced File Viewer:**
    * **Shared Directories:** Added support for shared directory tabs, allowing users to browse multiple shared volumes within the same view.
    * **UI Polish:** Introduced a manual Refresh button and improved multi-broker warnings for clarity.
* **UI/UX Refinement:**
    * **Navigation:** Reordered the terminal toolbar and added a direct "back-to-grove" link.
    * **Visual Cues:** Standardized Grove icons and added link badges to distinguish linked Groves in the dashboard.

## 🐛 Fixes
* **Kubernetes Agent Reliability:** 
    * **API Transition:** Switched to the Kubernetes Go client API for PTY execution, replacing the `kubectl` binary for better performance and stability.
    * **Privilege Management:** Ensured all critical K8s operations (exec, attach, tmux readiness) run as the `fabric` user.
    * **Startup Sequencing:** Implemented a startup gate to guarantee directory synchronization is complete before an agent process launches.
    * **Cleanup Logic:** Improved K8s runtime cleanup by handling stale resources and "pod not found" errors gracefully.
* **Observability & Logging:** Standardized `grove_id` and `agent_id` labels across all components to ensure they promote correctly to Cloud Logging.
* **Web UI Consistency:** Migrated all web components to a standardized `extractApiError()` utility for consistent and helpful error messaging.
* **Environment Precedence:** Fixed a critical issue where `FABRIC_GROVE_ID` from the environment was being overridden by local settings files, ensuring correct authoritative Grove association.
* **General Stability:** 
    * Fixed an issue where explicit prompt arguments would error instead of overwriting `prompt.md`.
    * Resolved dark theme styling regressions and UI navigation bugs after agent deletion.
