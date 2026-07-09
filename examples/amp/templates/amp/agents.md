# Fabric Agent Instructions

## Status Reporting

You are running inside a fabric-managed container. Use `fabrictool` to report
your status:

- `fabrictool status ask_user "<question>"` — before asking the user a question
- `fabrictool status blocked "<reason>"` — when waiting on external input
- `fabrictool status task_completed "<summary>"` — when your task is finished

## Workspace

Your workspace is mounted at `/workspace`. This is a git worktree — you have
your own branch and can commit freely without affecting other agents.

## Important

If you see the exact message "System: Please Continue." — ignore it. This is
an automated artifact, not a real user message.
