# Release Notes (2026-03-27)

This release marks a major milestone with the completion of the bidirectional messaging system and significant improvements to terminal interactions and rootless Podman compatibility.

## 🚀 Features

* **Bidirectional Messaging System:** Completed all five phases of the messaging evolution. This includes a robust message store, hub persistence, and a new human inbox available via both CLI and API. Agent outbound messaging and broker integration are fully operational.
* **Web Frontend Inbox Tray:** Introduced a new real-time inbox tray in the web header. It features an envelope icon with an unread badge, a popover panel for quick message review, and support for mark-read/mark-all-read actions powered by SSE updates.
* **Enhanced Admin UI:** The user list now includes direct administrative actions, allowing admins to promote, suspend, or delete users directly from the management interface.
* **Improved Terminal Window Switcher:** Redesigned the agent/shell window switcher buttons to be wider and rectangular (44x32px) with larger, centered icons for better visibility and accessibility.

## 🐛 Fixes

* **Terminal Selection & Mouse Behavior:**
    * Implemented support for shift-drag selection on macOS terminal.
    * Disabled copy-mode drag selection highlights in tmux to prevent confusion with browser-native selection.
    * Optimized overall web terminal mouse behavior for a more native feel.
* **Terminal Key Handling:** Resolved an issue where Shift+Enter caused double newlines; the terminal now correctly sends `ESC CR` instead of `CSI u`.
* **Rootless Podman Compatibility:** 
    * Fixed UID/GID mapping issues during `fabrictool init` for rootless environments.
    * Ensured `ExecUser` is correctly threaded through PTY handlers.
    * Mandated the use of the root user for `podman exec` when running in rootless mode to ensure proper permissions.
* **Terminal Environment:** PTY sessions now automatically set `TERM=xterm-256color` to ensure consistent color and feature support across different shells.
* **Web Assets:** Added a fallback for bootstrap-icons in the Shoelace icon copy script to ensure UI consistency during build processes.
* **Fabrictool Initialization:** Improved `fabrictool` resilience by falling back to direct `/etc/passwd` edits when `usermod` fails.

## 🧹 Chore & Refactoring

* **Template Architecture Cleanup:** Removed legacy `SeedCommonFiles` code and the `embeds/common/` directory. All common environment files (.tmux.conf, .zshrc, .gitconfig) are now seeded exclusively through the default agnostic template base layer, simplifying the provisioning logic.
* **Documentation:** Updated the `messages-evolution` design specification to reflect the successful completion of all phases.
