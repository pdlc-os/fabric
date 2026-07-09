# Release Notes (2026-03-22)

This release introduces a new 3D agent activity visualizer, implements a more robust template inheritance model, and overhauls the messaging and logging infrastructure for better scale and reliability across the Hub and Grove environments.

## ⚠️ BREAKING CHANGES
* **Template Inheritance:** All custom templates now automatically inherit from the `default` template as a base layer. The default template's `home/` directory (containing standard `.tmux.conf`, `.zshrc`, `.gitconfig`, etc.) is now copied first, with custom template files overlaid on top. Custom templates no longer need to manually include these common configuration files.
* **UUID-based Message Logging:** Message log entries have transitioned from using human-readable slugs to UUID-based `sender_id` and `recipient_id` for filtering and identification. This change prevents message leakage between agents with identical names and may require updates to external log parsing tools.
* **Database Schema:** The unique constraint on `group.grove_id` has been removed to allow multiple groups per grove, enabling more flexible permission and organizational structures.

## 🚀 Features
* **Agent Visualizer:** Introduced a new tool for GCP log replay and 3D graph-based activity visualization. This includes a dedicated web interface for replaying agent interactions and a new README with usage instructions.
* **Grove-Level Messaging:** Added a new grove-level message viewer and a broadcast compose box, allowing for better visibility and communication across multiple agents within a grove.
* **Enhanced Telemetry Controls:** Replaced the binary telemetry toggle with a tri-state select dropdown (None, Basic, Full) and implemented automatic GCP credential detection at well-known paths.
* **UI/UX Improvements:** 
    * Added a realtime agent status badge to the terminal page.
    * Implemented log severity filtering in the agent detail view.
    * Added column sorting to the grove detail file viewer.
* **Management Tools:** Supported the `--stopped` flag for the `delete` command over Hub, allowing for cleaner removal of inactive agents.

## 🐛 Fixes
* **Message Delivery:** Improved reliability of message delivery to terminal sessions using `tmux` bracketed paste and the `send-keys -l` (literal) flag.
* **Log Hygiene:** Resolved several issues causing duplicate message logging in `fabric-server` and `fabric-messages` and reduced log noise from notification dispatchers.
* **API Stability:** The message dispatcher now returns a `503 Service Unavailable` with a `Retry-After` header when it or the broker is not ready, improving client-side error handling.
* **Component Fixes:**
    * Fixed agent instruction resolution logic across the new template inheritance chain.
    * Corrected model hooks for Claude and enabled native Gemini telemetry.
    * Added missing icons (broadcast-pin, chevron-down, box-arrow-up-right) to the Shoelace icon set.
    * Resolved an issue where message recipient fields incorrectly used UUIDs instead of slugs in certain UI contexts.
