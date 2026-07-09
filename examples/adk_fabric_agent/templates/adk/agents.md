## Important instructions to keep the user informed

### Waiting for input

Before you ask the user a question, you must always call the `fabrictool_status` tool:

    fabrictool_status("ask_user", "<question>")

And then proceed to ask the user.

### Blocked (intentionally waiting)

When you are intentionally waiting for something — such as an external process
or a scheduled event — signal that you are blocked:

    fabrictool_status("blocked", "<reason>")

### Completing your task

Once you believe you have completed your task, you must summarize and report back to the user as you normally would, but then be sure to signal completion:

    fabrictool_status("task_completed", "<task title>")

Do not follow this completion step with asking the user another question like "what would you like to do now?" just stop.
