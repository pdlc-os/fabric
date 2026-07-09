# Fix: Workspace file browser path resolution (Issue #130)

**Date:** 2026-06-03
**PR:** #132
**Issue:** #130

## Problem

The Hub UI workspace file browser was showing the wrong directory contents. The `hubManagedProjectPath()` function resolved workspace paths to `~/.fabric/projects/<slug>/` instead of `~/.fabric/groves/<slug>/`.

The three relevant directories per project:
1. `~/.fabric/groves/<slug>/` — actual git checkout, mounted as `/workspace` in agents (correct target)
2. `~/.fabric/projects/<slug>/` — project metadata + Telegram plugin downloads (what the UI was showing)
3. `~/.fabric/grove-configs/<slug>__<uuid>/` — agent configs and shared-dirs

## Root Cause

`hubManagedProjectPath()` checked `projects/` first, fell back to `groves/`, and defaulted to `projects/`. This was backwards — the git checkout (what agents actually work in) lives under `groves/`.

## Fix

Reversed the lookup priority in `hubManagedProjectPath()`:
1. Check `groves/<slug>` first (preferred — actual workspace)
2. Fall back to `projects/<slug>` (backward compatibility)
3. Default to `groves/<slug>` when neither has content

## Files Changed

- `pkg/hub/handlers.go` — reversed path resolution priority
- `pkg/hub/handlers_project_test.go` — updated existing test, added 3 new test cases

## Observations

- The `pkg/config` test suite has a pre-existing failure (`TestEnsureHubReady_GlobalFallbackWithHubEnabled`) caused by leaked `FABRIC_*` environment variables in the container. This is unrelated to this change and passes when those env vars are cleared.
