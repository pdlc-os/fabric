# Web Terminal Copy/Paste: Options & Test Matrix

## Problem Statement

With the current `.tmux.conf` and web PTY setup, users cannot copy text from the
web-based terminal. The `set -g mouse on` setting causes tmux to intercept all
mouse events, preventing native browser text selection. The intended fallback —
OSC 52 clipboard relay — is not working end-to-end.

## Current Architecture

```
Browser (xterm.js + ClipboardAddon)
  ↕ WebSocket (JSON { type:"data", data: base64 })
Go PTY server (pty_handlers.go)
  ↕ PTY fd (raw bytes, no filtering)
docker exec -it <container> tmux attach-session -t fabric
  ↕ container TTY
tmux (inside container)
```

### Current .tmux.conf (relevant lines)
```
set -g mouse on                          # tmux captures all mouse events
set -g allow-passthrough on              # let sequences pass through to outer terminal
set -s set-clipboard on                  # emit OSC 52 on copy
bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-no-clear
bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-selection-no-clear
```

### Current terminal.ts (relevant parts)
- `@xterm/addon-clipboard` loaded (handles OSC 52 → `navigator.clipboard`)
- Custom key handler: Ctrl/Cmd+C copies xterm.js selection, Ctrl/Cmd+V pastes
- No special mouse configuration on the xterm.js Terminal instance

## Failure Analysis

The OSC 52 clipboard relay chain has multiple potential break points:

1. **tmux may not emit OSC 52** — `set-clipboard on` requires tmux to believe
   the outer terminal supports it. The `TERM` inside the container and the
   terminal-overrides/features must align. If tmux thinks the outer terminal
   doesn't support OSC 52, it silently drops it.

2. **Nested PTY may strip OSC 52** — The chain is: tmux → container TTY →
   `docker exec -it` PTY → broker-side PTY → WebSocket. The intermediate PTYs
   (especially docker's) may filter or truncate escape sequences.

3. **ClipboardAddon may silently fail** — `navigator.clipboard.writeText()`
   requires a secure context (HTTPS or localhost) AND the document must be
   focused. If the page is served over plain HTTP on a non-localhost URL, the
   clipboard API is unavailable.

4. **xterm.js selection is invisible** — Even though xterm.js renders the
   terminal buffer and could provide its own selection, `mouse on` in tmux
   enables mouse reporting escape sequences, so xterm.js forwards all mouse
   events to the PTY instead of creating a local selection. The user sees
   tmux's copy-mode highlight (rendered as terminal output) but never gets a
   browser-side selection they can Ctrl+C.

---

## Options to Test

### Option 1: Shift-Click Bypass (No Code Changes)

**Concept:** Most terminal emulators (including xterm.js) bypass mouse reporting
when the user holds **Shift**. This means Shift+click-drag creates a native
xterm.js selection even when tmux has `mouse on`.

**Changes:** None — this is already supported by xterm.js out of the box.

**Test procedure:**
1. Open web terminal to an agent
2. Hold Shift and click-drag to select text
3. Press Ctrl+C (or Cmd+C) — the existing key handler should copy the xterm.js selection
4. Paste elsewhere to verify

**Trade-offs:**
- (+) Zero code changes, works immediately
- (+) Keeps tmux mouse mode (scroll, pane click, etc.)
- (-) Non-discoverable — users won't know to hold Shift
- (-) Selection is xterm.js-native (character-based), not tmux copy-mode

**If this works**, we could add a small hint in the toolbar ("Hold Shift to select text").

---

### Option 2: Disable tmux Mouse Mode Entirely

**Concept:** Remove `set -g mouse on` so tmux never captures mouse events.
xterm.js handles all mouse interaction natively — click-drag selects text,
Ctrl+C copies it.

**Changes to `.tmux.conf`:**
```diff
-set -g mouse on
+set -g mouse off

-# On mouse-drag-end, copy the selection...
-bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-no-clear
-bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-selection-no-clear
```

**Trade-offs:**
- (+) Native browser selection just works
- (+) Ctrl+C/Cmd+C copy works via existing key handler
- (-) Loses mouse scroll (must use tmux copy-mode via `Ctrl-B [` then arrow/PgUp)
- (-) Loses mouse pane selection and resize
- (-) Feels less "modern" — many users expect mouse scroll

---

### Option 3: Mouse Mode ON + Debug/Fix OSC 52 Chain

**Concept:** Keep the current approach but systematically fix the OSC 52 relay.

**Sub-tests to isolate the break:**

#### 3a. Verify tmux is emitting OSC 52
Add temporary logging in `readFromPTY()` in `pty_handlers.go` to detect OSC 52:
```go
// In readFromPTY, after reading from ptySlave:
if bytes.Contains(buf[:n], []byte("\x1b]52;")) {
    slog.Info("OSC 52 detected in PTY output", "agentID", s.agentID, "len", n)
}
```

#### 3b. Verify tmux terminal type supports clipboard
Inside the container, check:
```bash
tmux show -s set-clipboard      # should say "on"
tmux show -g allow-passthrough   # should say "on"
echo $TERM                       # should be xterm-256color or similar
```

#### 3c. Force tmux to emit OSC 52 with terminal-overrides
Add to `.tmux.conf`:
```
set -as terminal-overrides ',*:Ms=\E]52;%p1%s;%p2%s\007'
```
This explicitly tells tmux that the outer terminal supports the `Ms` (modify
selection) capability, which is the OSC 52 sequence.

#### 3d. Verify ClipboardAddon is receiving data
In `terminal.ts`, add a debug listener before loading ClipboardAddon:
```typescript
// Temporary: log all OSC sequences for debugging
this.terminal.parser.registerOscHandler(52, (data) => {
  console.log('[Terminal] OSC 52 received:', data.substring(0, 50));
  return false; // let ClipboardAddon handle it
});
```

#### 3e. Verify browser clipboard API is available
```typescript
console.log('[Terminal] Clipboard API available:', !!navigator.clipboard);
console.log('[Terminal] Secure context:', window.isSecureContext);
```

**Trade-offs:**
- (+) If fixed, best UX — mouse works everywhere, copy is automatic
- (-) Complex chain with many failure points
- (-) May still break in non-HTTPS deployments

---

### Option 4: Hybrid — Mouse ON + xterm.js Selection Override

**Concept:** Keep `set -g mouse on` but configure xterm.js to NOT forward mouse
events for selection (only forward scroll and button clicks). This way, click-drag
creates an xterm.js-native selection while scroll still works in tmux.

**Changes to `terminal.ts`:**
```typescript
this.terminal = new Terminal({
  // ... existing options ...
  rightClickSelectsWord: true,
  // Override: don't let mouse reporting steal drag-select
});

// After terminal.open(), override mouse handling:
// xterm.js doesn't have a built-in "partial mouse" mode, but we can
// intercept at the DOM level
container.addEventListener('mousedown', (e) => {
  // If no modifier key, prevent xterm.js from forwarding to PTY
  // This allows native xterm.js selection
  if (!e.shiftKey && !e.ctrlKey && !e.metaKey) {
    // We want selection, not mouse reporting
    // ... this approach is fragile
  }
}, { capture: true });
```

**Trade-offs:**
- (+) Would give best of both worlds if it works
- (-) Fighting xterm.js internals — fragile, likely to break on updates
- (-) Complex to implement correctly (need to still allow scroll events through)
- (-) Not recommended

---

### Option 5: Mouse ON + Explicit Copy Button / Right-Click Menu

**Concept:** Keep `set -g mouse on` and rely on tmux's copy-mode for selection
visibility, but add an explicit "Copy" mechanism that reads the tmux paste buffer.

**Approach:** After the user selects in tmux copy-mode (which they can do with
mouse drag since `mouse on` triggers copy-mode), add a toolbar button or
keyboard shortcut that runs `tmux show-buffer` inside the container and returns
the result to the browser clipboard.

**Changes:**
- Add a "Copy Buffer" button to the terminal toolbar
- On click, send `tmux show-buffer` command through a separate exec channel
- Write result to `navigator.clipboard`

**Trade-offs:**
- (+) Works regardless of OSC 52
- (-) Requires extra UI and a second exec channel
- (-) Awkward UX — two-step copy process
- (-) Over-engineered

---

### Option 6: Mouse OFF + Scroll via Terminal Alternate Screen

**Concept:** Disable tmux mouse to get native selection, but restore scroll
functionality via xterm.js's built-in scrollback.

**Problem:** With `mouse off`, xterm.js scroll events don't go to tmux, but
xterm.js has its own scrollback buffer. However, when tmux is running, tmux
manages the alternate screen buffer, so xterm.js's scrollback only contains
what tmux has pushed out of view — which is usually nothing useful.

**This option doesn't really work** — listed for completeness.

---

## Recommended Test Order

| Priority | Option | Effort | Test |
|----------|--------|--------|------|
| 1 | **Option 1: Shift+Drag** | Zero | Just try it in the current build |
| 2 | **Option 3c: Add Ms override** | 1-line tmux.conf change | Test if OSC 52 starts working |
| 3 | **Option 3a+3d: Debug logging** | Temporary code | Identify where OSC 52 breaks |
| 4 | **Option 2: Mouse off** | Remove 3 lines from tmux.conf | Test if native selection is acceptable |

### Quick-test .tmux.conf variants

**Variant A — Current + Ms override (test OSC 52 fix):**
```
set -g mouse on
set -s set-clipboard on
set -g allow-passthrough on
set -as terminal-overrides ',*:Ms=\E]52;%p1%s;%p2%s\007'
bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-no-clear
bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-selection-no-clear
```

**Variant B — Mouse off (test native selection):**
```
set -g mouse off
set -s set-clipboard on
# (mouse bindings removed — not needed)
```

**Variant C — Current + external clipboard (belt and suspenders):**
```
set -g mouse on
set -s set-clipboard external
set -g allow-passthrough on
set -as terminal-overrides ',*:Ms=\E]52;%p1%s;%p2%s\007'
bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-no-clear
bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-selection-no-clear
```
Note: `set-clipboard external` tells tmux to use OSC 52 to set the outer
terminal's clipboard but NOT to also store in tmux's internal paste buffer for
applications. This is the recommended setting for nested terminal scenarios.

**Variant D — Mouse on + set-clipboard external + no-clear + allow-passthrough all:**
```
set -g mouse on
set -s set-clipboard external
set -g allow-passthrough all
set -as terminal-overrides ',*:Ms=\E]52;%p1%s;%p2%s\007'
set -as terminal-features ',xterm*:clipboard'
bind-key -T copy-mode MouseDragEnd1Pane send-keys -X copy-selection-no-clear
bind-key -T copy-mode-vi MouseDragEnd1Pane send-keys -X copy-selection-no-clear
```
