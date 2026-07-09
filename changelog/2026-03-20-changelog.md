# Release Notes (March 20, 2026)

This release focuses on refining the Kubernetes (K8s) runtime integration, hardening GitHub App security and reliability, and enhancing the Web UI for better observability and persistent user preferences.

## 🚀 Features
* **Kubernetes (K8s) Runtime Refinements:**
    * **Native PTY Support:** Switched to the K8s Go client API for `exec` and `attach`, replacing reliance on the `kubectl` binary and improving terminal stability.
    * **Autopilot Compatibility:** Added auto-detection for GKE Autopilot clusters to apply correct scheduling tolerances and default resource requests.
    * **Enhanced Security:** Agents now run as the `fabric` user within K8s pods instead of `root`, following principle of least privilege.
* **Web UI & UX Enhancements:**
    * **Persistent Filters:** The "My Groves/Agents" filter toggle is now persisted in `localStorage`, maintaining your view across sessions.
    * **Identity & Navigation:** Added grove links to the agent list and improved the display of git remote URLs during grove creation.
    * **Admin Improvements:** Added pagination to the admin users list and improved the rendering of GitHub App configuration.
    * **Notification UX:** Overhauled subscription workflows for better clarity and responsiveness.
* **Core Runtime & Images:**
    * **Bun Support:** Added the Bun runtime to the `core-base` image to support modern JavaScript/TypeScript workflows.
    * **GKE Auth:** Improved GKE authentication by adding required environment variables to systemd unit files and implementing early validation.

## 🐛 Fixes
* **GitHub App Integration:**
    * **Token Priority:** Fixed logic to ensure GitHub App tokens take precedence over legacy environment variables when both are present.
    * **Installation Verification:** The "Check Status" button now performs an active verification against the GitHub API.
    * **Reliability:** Resolved infinite loops in discovery and fixed settings persistence on the admin page.
* **Git Operations:**
    * **URL Normalization:** Normalized schemeless clone URLs to prevent failures during agent provisioning.
    * **Branch Auto-Detection:** The system now automatically falls back to the default remote branch if the specifically configured branch is missing.
    * **Ignore Logic:** Switched to `git check-ignore` for more consistent and accurate `.gitignore` detection.
* **Stability & Security:**
    * **Auth Fallback:** Implemented auto-fallback to GCE metadata authentication when K8s-specific exec plugins fail.
    * **SSE Filtering:** Fixed a bug where real-time SSE updates could bypass active UI filters.
    * **Message Delivery:** Ensured the CLI flushes message buffers before exiting to guarantee local delivery.
* **UI Polish:**
    * Resolved dark theme styling issues for list items and owner-only groves.
    * Fixed missing Shoelace icons and increased the size of the GitHub logo on grove detail pages.
    * Surfaced descriptive API error messages directly in the scheduling UI.
