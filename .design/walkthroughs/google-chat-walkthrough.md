# Google Chat App Walkthrough

**Created:** 2026-04-08
**Goal:** Step-by-step tour to exercise and demo all features of the Fabric Google Chat integration

---

## Prerequisites

Before starting, ensure:

1. **A running Fabric Hub** with at least one Runtime Broker
2. **The chat app deployed** — either via `make install` on a starter-hub VM or as a standalone service (see the [chat app README](../../extras/fabric-chat-app/README.md))
3. **Google Chat API configured** — the Workspace Add-on is registered with an HTTP endpoint pointing to the chat app's `/chat/events` URL, with the `/fabric` slash command (Command ID 1) defined
4. **A GCP service account** with Google Chat API permissions and Secret Manager access to the Hub's signing key
5. **At least one grove** registered on the Hub (with a Runtime Broker providing it)
6. **Two or more Hub user accounts** — one for the demo presenter, one (optional) to demonstrate multi-user notification routing

### Environment Check

Verify the chat app is healthy before starting the demo:

```
GET https://<CHAT_APP_URL>/chat/healthz
```

This checks Hub API reachability, broker plugin connection, and database accessibility.

---

## Part 1: First Contact — Adding the Bot

### Step 1.1: Add the Bot to a Space

1. Open Google Chat and navigate to a space (or create a new one for the demo)
2. Click the space name > **Apps & integrations** > **Add apps**
3. Search for the registered app name (e.g., "Fabric") and add it

**What happens:** The bot receives a `ADDED_TO_SPACE` event and posts a welcome message introducing itself and listing available commands.

### Step 1.2: Direct Message the Bot

1. In Google Chat, start a new 1:1 conversation with the Fabric bot
2. Type anything (e.g., "hello")

**What happens:** The bot responds with a help prompt since no grove is linked and no command was issued. This confirms the bot is receiving messages in DM context.

---

## Part 2: Identity — Registering Your Account

Before issuing commands that touch the Hub, your Google Chat identity must be linked to a Hub user account.

### Step 2.1: Check Current Status

```
/fabric info
```

**Expected output:** Shows you are **not registered** and the space is **not linked** to a grove. Also displays the Hub hostname and version.

### Step 2.2: Register via Email Auto-Match

```
/fabric register
```

**What happens (happy path):** If your Google Chat email matches a Hub user's email, the app automatically links the accounts. You'll see a confirmation card: *"Registered as you@example.com"*.

### Step 2.3: Register via Device Authorization (Fallback)

If your Chat email doesn't match any Hub user:

1. Run `/fabric register` — the bot responds with a **verification URL** and a **user code**
2. Open the verification URL in your browser
3. Enter the user code and authorize the app
4. Return to Google Chat and run:

```
/fabric register confirm
```

**What happens:** The app polls for token completion, links your Chat account to the authorized Hub user, and confirms registration.

### Step 2.4: Verify Registration

```
/fabric info
```

**Expected output:** Now shows your registration status, linked Hub email, and registration method (auto or manual).

### Step 2.5: Unregister (Optional — Demo the Reverse)

```
/fabric unregister
```

**What happens:** Removes the Chat-to-Hub account link. Subsequent commands that require authentication will fail until you register again. Re-register before continuing.

---

## Part 3: Linking a Space to a Grove

Linking a space scopes all agent commands in that space to a specific grove (project).

### Step 3.1: Link the Space

```
/fabric link <grove-slug>
```

Replace `<grove-slug>` with the slug of a grove registered on the Hub (e.g., `my-project`).

**What happens:** The space is linked to the grove. The bot confirms with the grove name and slug. From now on, agent commands in this space operate against this grove.

### Step 3.2: Confirm the Link

```
/fabric info
```

**Expected output:** Shows the space is linked to the grove, along with the number of agents currently in that grove.

---

## Part 4: Agent Lifecycle — Create, Start, Stop, Delete

### Step 4.1: List Existing Agents

```
/fabric list
```

**What happens:** Displays a card listing all agents in the linked grove with their current status (RUNNING, STOPPED, etc.) and recent activity. If no agents exist yet, the list will be empty.

### Step 4.2: Create an Agent

```
/fabric create demo-agent
```

**What happens:** Creates a new agent named `demo-agent` in the linked grove. The bot confirms creation with the agent's details.

### Step 4.3: Start the Agent

```
/fabric start demo-agent
```

**What happens:** The Hub dispatches a start request to the Runtime Broker. The bot confirms the agent is starting. The agent transitions through STARTING to RUNNING.

### Step 4.4: Check Agent Status

```
/fabric status demo-agent
```

**What happens:** Displays an interactive status card with:
- Agent name, status, and current activity
- Key-value details (template, branch, runtime)
- **Action buttons**: Stop, View Logs (and others depending on state)

### Step 4.5: View Agent Logs

```
/fabric logs demo-agent
```

**What happens:** Fetches the last 50 lines of the agent's logs and displays them in a code-formatted message. Output is truncated to 2000 characters to stay within Chat message limits.

**Alternative:** Click the **View Logs** button on a status card or notification card — same result.

### Step 4.6: Stop the Agent

```
/fabric stop demo-agent
```

**What happens:** The agent container is stopped. The bot confirms the agent has been stopped.

### Step 4.7: Restart the Agent (from Status Card)

1. Run `/fabric status demo-agent` to get the status card
2. Click the **Start** button on the card

**What happens:** Same as `/fabric start` but triggered via card interaction. Demonstrates that card buttons are fully functional action shortcuts.

### Step 4.8: Delete the Agent

```
/fabric delete demo-agent
```

**What happens:** A **confirmation dialog** pops up asking you to confirm deletion. This is a two-step process to prevent accidental deletions:

1. Click **Confirm** in the dialog
2. The agent is deleted and the dialog closes with a snackbar notification: *"Agent deleted"*

**Note:** If the agent is still running, it will be stopped first.

---

## Part 5: Messaging Agents

### Step 5.1: Send a Message via Slash Command

```
/fabric message demo-agent Please check the staging environment
```

**What happens:** The message is delivered to the agent as if from your Hub user (using an impersonation token). The agent sees the message prefixed with your identity (e.g., `user:you@example.com`).

### Step 5.2: Send a Threaded Message

```
/fabric message --thread <thread-id> demo-agent Follow up on the previous request
```

**What happens:** The message is associated with a specific thread for context continuity.

### Step 5.3: Send a Message via @Mention

Type a regular message in the space and @mention the bot:

```
@Fabric tell demo-agent to run the test suite
```

**What happens:** The bot parses the mention, identifies the target agent from context, and routes the message. This is a more natural, conversational way to interact with agents.

---

## Part 6: Notifications — Subscribing to Agent Activity

### Step 6.1: Subscribe to All Notifications

```
/fabric subscribe demo-agent
```

**What happens:** A **filter dialog** appears with checkboxes for activity types:
- COMPLETED
- WAITING_FOR_INPUT
- ERROR
- STALLED
- LIMITS_EXCEEDED

Leave all unchecked to subscribe to **all** activities, or select specific ones.

Click **Subscribe** to confirm.

### Step 6.2: Trigger a Notification — COMPLETED

Start an agent with a simple task and let it finish. When the agent completes:

**What happens:** A notification card appears in the space with:
- Header: agent name with a checkmark icon
- Activity: COMPLETED
- Your @mention in the message text (since you're subscribed)
- **View Logs** button

### Step 6.3: Trigger a Notification — WAITING_FOR_INPUT

Start an agent that will ask for user input (e.g., an agent running Claude Code that encounters a confirmation prompt).

**What happens:** A notification card appears with:
- Header: agent name with a pause icon
- Activity: WAITING_FOR_INPUT
- An **inline text input field** for responding directly from the card
- **View Logs** button

### Step 6.4: Respond to a WAITING_FOR_INPUT Notification

1. In the notification card's text input field, type your response
2. Click the **Respond** button

**What happens:** Your response is sent to the agent as a message. The agent receives it and continues execution. The response is attributed to your Hub identity.

### Step 6.5: Trigger a Notification — ERROR

If an agent encounters an error:

**What happens:** A notification card appears with:
- Header: agent name with an error icon
- Activity: ERROR
- **View Logs** button
- **Restart** button (primary/blue styling)

Click **Restart** to start the agent again directly from the notification.

### Step 6.6: Observe STALLED and LIMITS_EXCEEDED Notifications

These fire when an agent becomes unresponsive or exceeds resource limits:

| Activity | Card Actions |
|----------|-------------|
| STALLED | View Logs, Restart (primary), Stop (danger/red) |
| LIMITS_EXCEEDED | View Logs, Stop (danger/red) |

### Step 6.7: Acknowledge a Notification

Click the **Acknowledge** button on any notification card.

**What happens:** The notification is marked as acknowledged (visual feedback via card update).

### Step 6.8: Unsubscribe

```
/fabric unsubscribe demo-agent
```

**What happens:** You stop receiving notification cards for this agent. Direct user-targeted messages (sent specifically to you by an agent) are still delivered regardless of subscription status.

---

## Part 7: User-Targeted Messages

Unlike broadcast notifications, **user-targeted messages** are sent directly to a specific user by an agent. These are always delivered regardless of subscriptions.

### Step 7.1: Receive a Direct Message from an Agent

When an agent sends a message targeting your Hub user (e.g., via `fabrictool status ask_user`):

**What happens:** A card message appears in the space with:
- The agent's name in the header
- The message content
- Your @mention in the text body
- If other users are subscribed to this agent, they are also @mentioned

### Step 7.2: Multi-User Routing (Optional — Requires Second User)

1. Have a second user register and subscribe to the same agent
2. Trigger a notification

**What happens:** Both subscribed users are @mentioned in the notification card. User-targeted messages go only to the intended recipient, but subscribers get visibility.

---

## Part 8: Space Management

### Step 8.1: Unlink the Space

```
/fabric unlink
```

**What happens:** The space-grove link is removed. All active notification subscriptions for this space are cancelled. Agent commands will no longer work until a new grove is linked.

### Step 8.2: Re-Link to a Different Grove

```
/fabric link <different-grove-slug>
```

**What happens:** The space is now scoped to the new grove. Running `/fabric list` shows agents from the new grove.

### Step 8.3: Remove the Bot from a Space

Remove the Fabric app from the space via Google Chat's space settings.

**What happens:** The bot receives a `REMOVED_FROM_SPACE` event and automatically cleans up the space link and subscriptions.

---

## Part 9: Help and Discovery

### Step 9.1: View All Commands

```
/fabric help
```

**What happens:** Displays a complete command reference with usage and descriptions for all available subcommands.

---

## Quick Reference Card

| Feature | How to Exercise |
|---------|----------------|
| Welcome message | Add bot to a space |
| Identity registration (auto) | `/fabric register` (matching email) |
| Identity registration (device auth) | `/fabric register` + browser flow + `/fabric register confirm` |
| Space-grove linking | `/fabric link <grove>` |
| Agent list | `/fabric list` |
| Agent lifecycle | `/fabric create`, `start`, `stop`, `delete <agent>` |
| Agent status card | `/fabric status <agent>` |
| Agent logs | `/fabric logs <agent>` |
| Send message to agent | `/fabric message <agent> <text>` |
| @mention messaging | `@Fabric tell <agent> ...` |
| Subscribe to notifications | `/fabric subscribe <agent>` (with filter dialog) |
| WAITING_FOR_INPUT response | Inline text field on notification card |
| Card button actions | Start, Stop, Restart, View Logs, Acknowledge |
| Deletion confirmation | `/fabric delete <agent>` (dialog prompt) |
| Unsubscribe | `/fabric unsubscribe <agent>` |
| Unlink space | `/fabric unlink` |
| Unregister | `/fabric unregister` |
| Health check | `GET /chat/healthz` |

---

## Demo Flow (15-Minute Script)

For a concise live demo, follow this abbreviated sequence:

1. **Setup** (pre-done): Bot added to space, presenter registered
2. `/fabric info` — show unlinked state
3. `/fabric link my-project` — link to a grove
4. `/fabric info` — show linked state
5. `/fabric list` — show existing agents
6. `/fabric create demo-agent` — create an agent
7. `/fabric start demo-agent` — start it
8. `/fabric status demo-agent` — show interactive status card
9. `/fabric subscribe demo-agent` — subscribe with filter dialog
10. `/fabric message demo-agent Check the build status` — send a message
11. Wait for a WAITING_FOR_INPUT notification, respond inline
12. `/fabric logs demo-agent` — view logs
13. `/fabric stop demo-agent` — stop via command
14. Click **Start** on the status card — restart via button
15. `/fabric delete demo-agent` — delete with confirmation dialog
16. `/fabric unlink` — clean up

---

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| Bot doesn't respond to slash commands | HTTP endpoint URL misconfigured | Verify the Chat API config points to `<url>/chat/events` |
| "Not registered" on every command | Chat email doesn't match Hub user | Run `/fabric register` and complete device auth flow |
| "Space not linked" errors | No grove linked to this space | Run `/fabric link <grove-slug>` |
| Notifications not appearing | Not subscribed, or activity filtered out | Run `/fabric subscribe <agent>` and check filter settings |
| Card buttons return errors | Impersonation token expired or signing key mismatch | Check chat app logs; verify signing key in Secret Manager |
| Bot welcome message not shown | Bot added via @mention (skips welcome) | Add via space settings instead |
| Health check fails | Hub unreachable, DB inaccessible, or plugin disconnected | Check `journalctl -u fabric-chat-app` for details |
