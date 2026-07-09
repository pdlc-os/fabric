# Release Notes (March 18, 2026)

This release focuses on a major modernization of authentication via User Access Tokens (UATs) and the initial rollout of native GitHub App integration. It also significantly improves infrastructure robustness with idempotent setup scripts and enhanced GCP identity management.

## ⚠️ BREAKING CHANGES
* **Legacy API Key Removal:** The legacy API key system (`sk_live_*`) has been completely removed. Users must migrate to the new User Access Tokens (`fabric_pat_*`) for all programmatic and API access.
* **Metadata Server Security:** "Block mode" now strictly enforces the restriction of direct metadata server access for agents. Any agents that were previously bypassing this due to loose enforcement will now be correctly restricted.

## 🚀 Features
* **User Access Tokens (UATs):** Implemented a complete end-to-end token system (Phases 1-3) replacing the legacy API keys.
    * **Granular Scoping:** New `fabric_pat_*` tokens support grove-level scoping and specific action permissions.
    * **CLI Management:** Added `fabric auth tokens` commands for lifecycle management (create, list, revoke).
    * **Web UI:** Dedicated management interface in the user profile for token administration.
    * **Observability:** Added `auth-type` logging to all requests to differentiate between session and UAT-based access.
* **GitHub App Integration:** Completed Phases 1 and 2 of the GitHub App integration for agent git authentication.
    * **Native Auth:** Support for JWT-based authentication and automated installation token minting.
    * **Grove Association:** Infrastructure to link groves with specific GitHub App installations.
    * **Webhooks:** Initial support for GitHub webhooks to handle installation and status updates.
* **GCP Identity & Security:**
    * **Automated Token Minting:** Integrated `GCPTokenGenerator` for seamless service account token acquisition.
    * **Identity Visibility:** Added GCP service account details to the Identity card in the Web UI.
    * **Restricted Passthrough:** Tightened security by restricting GCP identity passthrough to broker owners only.
* **Infrastructure & Setup:**
    * **Flexible Deployments:** Parameterized `starter-hub` scripts to make GKE optional, allowing for easier standalone GCE deployments.
    * **Implicit Hub Mode:** The system now automatically enables Hub mode when valid credentials and endpoints are detected in the configuration.
* **UI/UX Enhancements:**
    * **Layout Refactoring:** Overhauled the Grove detail and settings pages for better organization and navigation.
    * **Workspace Table:** Added a height-limited, scrollable workspace files table to the Grove detail view.

## 🐛 Fixes
* **Script Idempotency:** Made all GCE provisioning, bootstrap, and setup scripts fully idempotent, ensuring safe re-runs without side effects.
* **Provisioning Fixes:** Resolved cloud-init syntax errors (`#cloud-config` header requirement) and fixed permission issues for certificates during deployment.
* **Environment Setup:** Fixed missing `make` dependency in hub build steps and resolved JSON key mismatches during GCP service account creation.
* **UI Consistency:** Standardized button styles on the Current State card and fixed missing icons in the profile navigation menu.
