# Release Notes (March 19, 2026)

This release marks the completion of the GitHub App integration (Phases 3 and 4), introducing automated token refresh and comprehensive UI management. It also focuses on significantly improving developer experience and code quality through a unified CI pipeline and enhanced filtering in the Web UI.

## 🚀 Features
* **GitHub App Integration (Phase 3 & 4):**
    * **Automated Token Refresh:** Implemented a background refresh loop for installation tokens, ensuring long-running agents always have valid git credentials.
    * **Git Credential Helper:** Updated `fabrictool` to provide fresh tokens to git on-demand, replacing static environment variables.
    * **Admin Management UI:** A new "GitHub App" tab in the Admin Server Config allows for global monitoring of installations, rate limits, and status.
    * **Grove-Level Status:** Visual indicators and permission badges in Grove settings provide real-time feedback on GitHub integration health.
    * **Commit Attribution:** Added support for per-grove git identity configuration for correct commit authorship.
* **Unified CI Pipeline:**
    * **Local `make ci`:** Added a `make ci` target that mirrors the GitHub Actions pipeline, including `golangci-lint` and automated formatting.
    * **Auto-Formatting:** Replaced simple format checks with an automated `fmt` target to ensure consistent style across the codebase.
* **Enhanced Web UI Filtering:**
    * **"My Agents" & "My Groves":** Introduced server-side filters and dedicated UI sections to easily access resources owned by or shared with the current user.
    * **Grove List Improvements:** Replaced the "Last Updated" column with "Owner" for better context in multi-user environments.
* **Infrastructure & Monitoring:**
    * **Broker Relay Logs:** Implemented `fabric logs` via the broker relay, enabling log streaming in Hub-mode without direct agent connectivity.
    * **Rate Limit Monitoring:** Added proactive monitoring of GitHub API rate limits to prevent unexpected integration failures.
    * **Resource Scaling:** Added `n2-standard-16` (64GB) as a supported instance size for GCE demo provisioning.

## 🐛 Fixes
* **Linting & Quality:** Resolved over 50 `golangci-lint` warnings, including error check failures, deprecated imports, and unused struct fields.
* **Git Reliability:** Switched to HTTPS cloning for public repositories and improved repository detection using `git rev-parse`.
* **UI Polish:** Added missing page titles for admin routes and introduced visual dots for state indicators in settings.
* **Stability:** Fixed a regression where the runtime broker was not always enabled in the Hub systemd service.
* **Tooling:** Removed false `.gitignore` warnings and ensured `fabric init` is fully idempotent.
