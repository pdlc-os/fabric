# Release Notes (2026-03-26)

This release focuses on a significant enhancement to the security model with the introduction of transitive access control via agent ancestry, alongside improved terminal input handling and new workflow templates.

## 🚀 Features
* **Transitive Agent Access (Ancestry):** Implemented an ancestry chain for agents (`root` → `parent` → `child`). This enables transitive access, allowing any principal (user or agent) in an agent’s creation chain to access it without complex recursive queries. This includes a new SQLite migration (`V37`) and updated filtering capabilities for descendant queries.
* **Repository Workflow Templates:** Added a suite of `.fabric/templates` to streamline common tasks. These include templates for generating `release-notes` and a `web-dev` environment complete with pre-configured harnesses for Claude and Gemini, custom `.bashrc`/`.tmux.conf` setups, and specialized skills.
* **Enhanced PTY Debugging:** Added comprehensive escape sequence logging across the PTY pipeline to improve diagnostics for terminal-based input and output issues.

## 🐛 Fixes
* **Web Terminal Shift+Enter:** Resolved an issue where Shift+Enter was not correctly handled in the web terminal by ensuring the proper CSI u sequence is transmitted.
* **Authorization Consistency:** Unified authorization checks across all API handlers and capability computations to consistently respect the new ancestry-based access model.
* **Environment Persistence (Gemini):** Fixed a bug where environment variables could be lost during harness execution by explicitly setting `GEMINI_CLI_NO_RELAUNCH` and refining `SANDBOX_ENV` protections.
* **Legacy Agent Compatibility:** Added handling for `NULL` ancestry columns to ensure compatibility for agents created prior to the `V37` migration.

## 📝 Documentation & Chores
* Finalized the `agent-auth-refactor` design specification.
* Updated internal `fabric` agent instructions for better alignment with current capabilities.
* Updated `picomatch` dependencies in `/web` and `/extras/agent-viz/web`.
