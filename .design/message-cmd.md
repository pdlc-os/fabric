# Message command

## Status: Implemented

**Implementation:** `cmd/message.go` (216 lines)

---

## Design

`fabric message` (msg for short)

Sends a message to harnesses which enqueues new messages to an agent.

Uses tmux `send-keys` command combined with the `exec` command of the runtime. Sends the message plus the "Enter" special tmux key.

### Flags

- `-i` / `--interrupt` — First uses send-keys to send either 'Escape' or 'C-c' depending on the harness (harness-specific, 'C-c' for generic)
- `-b` / `--broadcast` — Sends the message to all running agents in the grove
- `--all` — Cross-grove broadcast to all running agents

### Implementation Notes

- Supports both Hub-based and local runtime messaging
- Single agent message send and broadcast modes
- Alias: `msg`