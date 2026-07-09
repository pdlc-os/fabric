# Release Notes (March 30, 2026)

This release focuses on strengthening Hub-scoped secret security through instance-based namespacing, improving visibility for cloud-stored secrets, and refining GitHub App error diagnostics for more efficient troubleshooting.

## ⚠️ BREAKING CHANGES
* **Secret Management Namespacing:** Hub-scoped secrets are now uniquely namespaced by Hub Instance ID to prevent name collisions when multiple Hub instances share a single GCP project for Secret Manager.
    * **Action Required:** For environments where multiple Hub instances are in use, use the new `--hub-id` flag with the `fabric hub secret migrate` command to transition existing secrets to the namespaced format.

## 🚀 Features
* **Secret Management & Observability:**
    * **Hub-Instance Namespacing:** Introduced unique namespacing for Hub-scoped secrets using a per-Hub instance ID (derived from hostname or configuration). Secrets in GCP Secret Manager are now labeled with `fabric-hub-hostname` for easier filtering in the console.
    * **Resource URI Visibility:** The GCP Secret Manager resource URI (`SecretRef`) is now included in API responses and the CLI output for `fabric hub secret get`, providing direct visibility into underlying cloud resources.
* **Workspace Management:**
    * **Nested Grove Support:** Relaxed the restriction on nested grove initialization. The `init` command now detects existing parent groves and provides informational guidance instead of erroring, enabling more flexible and hierarchical workspace structures.

## 🐛 Fixes
* **GitHub App Integration:**
    * **Enhanced Diagnostics:** Improved error messages for GitHub App authentication failures with actionable guidance for JWT and private key mismatches. Added key fingerprint logging to assist in identifying configuration errors.
    * **API Accuracy:** Refined HTTP status codes for token refresh endpoints to distinguish between upstream authentication failures (502), revoked installations (422), and permission issues (403).
    * **UI Polish:** Formatted internal error codes as human-readable labels within the grove settings UI for better clarity.
