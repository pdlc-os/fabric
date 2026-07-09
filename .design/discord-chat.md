# Discord Chat Provider for Fabric Chat App

**Created:** 2026-04-27
**Status:** Draft
**Related:** `.design/google-chat.md`, `.design/slack-chat.md`, `.design/message-broker-plugin-evolution.md`, `.design/chat-plugin-tradeoffs.md`

---

## Overview

This design describes the Discord adapter implementation for the `fabric-chat-app`. It follows the same pattern as the Google Chat (Phases 1–2) and Slack (Phase 3) adapters: implement the `chatapp.Messenger` interface, normalize Discord events to `ChatEvent`, and render the existing `Card` / `Dialog` / `Widget` model as Discord Embeds and Message Components. All other core infrastructure — the command router, notification relay, identity mapper, broker plugin, and SQLite state layer — is reused without modification.

Discord's architecture differs from both Google Chat and Slack in important ways. Events arrive primarily via a persistent WebSocket Gateway connection rather than HTTP webhooks. Slash commands are registered globally via the Discord API rather than configured in a web console. Rich messages use Embeds and Message Components (buttons, select menus) rather than Cards V2 or Block Kit. And per-agent identity requires channel webhooks rather than a per-message scope flag. These differences are significant at the adapter layer but transparent to the shared core.

### Goals

- Implement `chatapp.Messenger` for Discord using the Discord API (Gateway + REST)
- Receive events via the Discord Gateway (WebSocket) with HTTP Interactions Endpoint as an alternative
- Render the existing `Card` / `Dialog` / `Widget` model as Discord Embeds + Message Components
- Use Discord channel webhooks to show per-agent personas (name + avatar)
- Use Discord Modals for the `Dialog` abstraction (text/textarea fields)
- Use Discord Threads for notification conversation threading
- Provide a guild-level dashboard via a `/fabric dashboard` command
- Integrate with the existing command router, notification relay, and identity mapper without changes to shared code

### Non-Goals (This Phase)

- Discord voice channel integration
- Discord Stage channel support
- Discord Forum channel support (threads-only channels) — regular text channel + thread-per-notification is sufficient for MVP; Forum channels could be revisited as a cleaner organized surface later
- Discord AutoMod integration
- Custom Discord Activities (Rich Presence)
- Sharding for large-scale deployments (>2,500 guilds) — Fabric's bot is self-hosted per team, not a public distributed bot
- File/attachment relay to agents
- OAuth2 user authorization flow for email retrieval

---

## Architecture

### Where the Discord Adapter Fits

The Discord adapter slots into the existing architecture at the same layer as the Google Chat and Slack adapters. All three are interchangeable implementations of `chatapp.Messenger` that normalize platform events to `ChatEvent` and render `Card`/`Dialog` types to platform-native formats.

```
                    ┌─────────────────────────────────────────┐
                    │            Discord Platform             │
                    │  Gateway (WSS) / Interactions / REST    │
                    └──────────┬──────────────────────────────┘
                               │
                    ┌──────────▼──────────────────────────────┐
                    │     internal/discord/adapter.go          │
                    │     (implements chatapp.Messenger)       │
                    │                                         │
                    │  ┌────────────┐  ┌──────────────────┐   │
                    │  │ Event      │  │ Embed + Component │   │
                    │  │ Normalizer │  │ Renderer          │   │
                    │  └──────┬─────┘  └────────┬─────────┘   │
                    │         │                  │             │
                    └─────────┼──────────────────┼─────────────┘
                              │                  │
                  ┌───────────▼──────────────────▼────────────┐
                  │         Shared Core Engine                │
                  │  CommandRouter · NotificationRelay        │
                  │  IdentityMapper · StateStore · Broker     │
                  └───────────────────────────────────────────┘
```

All three adapters can run simultaneously in the same process. A new `Discord DiscordConfig` field is added to `PlatformsConfig`; when `discord.enabled: true`, `main.go` creates the Discord adapter alongside the existing adapters, wiring it to the same `CommandRouter`.

### Module & Package Layout

All Discord-specific code lives in a single new package:

```
extras/fabric-chat-app/
├── internal/
│   ├── discord/
│   │   ├── adapter.go        # DiscordAdapter: Messenger impl, Gateway/HTTP
│   │   ├── embeds.go         # Card → Discord Embed rendering
│   │   ├── components.go     # Widget buttons/selects → Message Components
│   │   ├── events.go         # Event normalization (Discord → ChatEvent)
│   │   ├── modals.go         # Dialog → Discord Modal rendering
│   │   ├── webhooks.go       # Per-channel webhook management for agent identity
│   │   ├── commands.go       # Application command registration
│   │   ├── verify.go         # Ed25519 interaction signature verification
│   │   └── adapter_test.go   # Unit tests
│   ├── chatapp/              # (unchanged)
│   ├── googlechat/           # (unchanged)
│   ├── identity/             # (unchanged)
│   └── state/                # (unchanged)
```

### Dependencies

The adapter uses the most widely adopted Go Discord library:

```
github.com/bwmarrin/discordgo
```

This is added to the `extras/fabric-chat-app/go.mod` only, keeping the Discord SDK out of the main Fabric module's dependency tree (same pattern as the Google Chat and Slack SDKs).

---

## Discord App Configuration

### Bot Application Setup

The Discord bot is created and configured in the [Discord Developer Portal](https://discord.com/developers/applications). The key configuration steps:

1. **Create Application** in the Developer Portal
2. **Enable Bot** under the "Bot" section
3. **Configure Privileged Gateway Intents** (see below)
4. **Set Interactions Endpoint URL** (optional, for HTTP mode)
5. **Generate OAuth2 URL** with required scopes and permissions for guild installation

### Required Permissions & Intents

**OAuth2 Scopes:**

| Scope | Purpose |
|-------|---------|
| `bot` | Bot user in guilds |
| `applications.commands` | Slash command registration |

**Bot Permissions (integer: 277562386496):**

| Permission | Hex | Purpose |
|-----------|-----|---------|
| `Send Messages` | `0x800` | Post messages in channels |
| `Send Messages in Threads` | `0x4000000000` | Reply in threads |
| `Create Public Threads` | `0x800000000` | Create threads for notification conversations |
| `Manage Webhooks` | `0x20000000` | Create per-channel webhooks for agent identity |
| `Embed Links` | `0x4000` | Send rich embeds |
| `Read Message History` | `0x10000` | Context for thread operations |
| `Use Application Commands` | `0x80000000` | Register and receive slash commands |
| `Add Reactions` | `0x40` | Optional: acknowledge messages |

**Gateway Intents:**

| Intent | Privileged | Purpose |
|--------|-----------|---------|
| `GUILDS` | No | Guild join/leave, channel info |
| `GUILD_MESSAGES` | No | Message events in guild channels |
| `DIRECT_MESSAGES` | No | DM events with the bot |
| `MESSAGE_CONTENT` | Yes | Read @mention message text (optional, see below) |

> **Note on MESSAGE_CONTENT:** This privileged intent is required only if @mention message routing is enabled (parsing `@Fabric tell deploy-agent ...` messages). If the adapter relies solely on slash commands and interactions for all input, this intent is not needed. The adapter supports both modes: slash-command-only (no privileged intents) and slash-command + @mention (requires MESSAGE_CONTENT). The default is slash-command-only.

### Bot Token

Discord uses a single Bot Token for both the Gateway WebSocket connection and all REST API calls. There is no separate signing secret or app-level token.

```
Bot Token: obtained from Developer Portal → Bot → Token
Public Key: obtained from Developer Portal → General Information (for HTTP interaction verification)
Application ID: obtained from Developer Portal → General Information (for command registration)
```

### Gateway vs. HTTP Interactions Endpoint

The adapter supports two modes for receiving interactions (slash commands, button clicks, modal submissions):

| Mode | When to use | How it works |
|------|-------------|--------------|
| **Gateway (WebSocket)** | Default; most deployments | Bot maintains persistent WSS connection to Discord; all events arrive over the socket |
| **HTTP Interactions Endpoint** | Serverless, stateless, or split deployments | Discord POSTs interactions to a configured URL; requires Ed25519 signature verification |

> **Important difference from Slack:** In Slack, HTTP mode and Socket Mode are complete alternatives — each handles all events independently. In Discord, the Gateway handles *all* event types (messages, guild events, presence, etc.), while the HTTP Interactions Endpoint handles *only* interactions (slash commands, components, modals). A Gateway connection is needed regardless for non-interaction events like `GUILD_CREATE` and `MESSAGE_CREATE`. The HTTP Interactions Endpoint is primarily useful when the adapter wants to handle interactions separately (e.g., behind a load balancer) while still running a Gateway connection for other events.

For most deployments, Gateway-only mode is recommended. The adapter always opens a Gateway connection; the HTTP Interactions Endpoint is an optional supplement.

---

## Adapter Implementation

### DiscordAdapter Structure

```go
package discord

import (
    "context"
    "log/slog"
    "net/http"
    "sync"

    "github.com/bwmarrin/discordgo"

    "github.com/pdlc-os/fabric/extras/fabric-chat-app/internal/chatapp"
)

const PlatformName = "discord"

// EventHandler processes normalized chat events.
type EventHandler func(ctx context.Context, event *chatapp.ChatEvent) (*chatapp.EventResponse, error)

// Config holds Discord adapter configuration.
type Config struct {
    BotToken         string // Bot token from Developer Portal
    ApplicationID    string // Application ID for command registration
    PublicKey        string // Ed25519 public key for HTTP interaction verification
    ListenAddress    string // HTTP listen address (optional, for interactions endpoint)
    InteractionsHTTP bool   // Use HTTP Interactions Endpoint alongside Gateway
    MentionRouting   bool   // Enable @mention message routing (requires MESSAGE_CONTENT intent)
    GuildID          string // Restrict commands to a single guild (dev mode); empty = global
}

// Adapter implements chatapp.Messenger for Discord.
type Adapter struct {
    session      *discordgo.Session
    appID        string
    publicKey    string
    httpServer   *http.Server
    handler      EventHandler
    iconProvider IconProvider
    log          *slog.Logger

    // Webhook cache: channelID → webhook
    webhooksMu sync.RWMutex
    webhooks   map[string]*discordgo.Webhook

    // User cache: userID → cached profile
    userCache *userCache
}
```

### Messenger Interface Mapping

Each `Messenger` method maps to specific Discord API calls:

| Messenger Method | Discord API | Notes |
|-----------------|-------------|-------|
| `SendMessage()` | `ChannelMessageSendComplex` or webhook `Execute` | Uses webhook for agent-identity messages; bot API for system messages |
| `SendCard()` | `ChannelMessageSendComplex` with embeds + components | Card rendered as Embed with Action Rows |
| `UpdateMessage()` | `ChannelMessageEditComplex` | Uses message ID directly |
| `OpenDialog()` | `InteractionRespond` with `InteractionResponseModal` | Converts `Dialog` to Discord Modal |
| `UpdateDialog()` | _(not supported — Discord modals are immutable once opened)_ | See Modals section |
| `GetUser()` | `User` (REST) or `GuildMember` | Returns Discord user profile; email not available |
| `SetAgentIdentity()` | _(no-op, identity set per-message via webhook)_ | Identity applied at send time via channel webhook |

#### SendMessage

```go
func (a *Adapter) SendMessage(ctx context.Context, req chatapp.SendMessageRequest) (string, error) {
    // Build message data
    data := &discordgo.MessageSend{
        Content: req.Text,
    }

    // Block Kit cards → Embed + Components
    if req.Card != nil {
        data.Embeds = []*discordgo.MessageEmbed{renderEmbed(req.Card)}
        components := renderComponents(req.Card)
        if len(components) > 0 {
            data.Components = components
        }
    }

    // Threading
    if req.ThreadID != "" {
        // Send to thread channel instead of parent
        msg, err := a.session.ChannelMessageSendComplex(req.ThreadID, data)
        if err != nil {
            return "", err
        }
        return msg.ID, nil
    }

    // Per-agent identity via webhook
    if req.AgentID != "" {
        return a.sendViaWebhook(ctx, req.SpaceID, req.AgentID, data)
    }

    // Standard bot message
    msg, err := a.session.ChannelMessageSendComplex(req.SpaceID, data)
    if err != nil {
        return "", err
    }
    return msg.ID, nil
}
```

The return value is the Discord message ID (a snowflake string). This is stored as the message reference for updates and threading.

#### sendViaWebhook (Agent Identity)

```go
func (a *Adapter) sendViaWebhook(ctx context.Context, channelID, agentSlug string, data *discordgo.MessageSend) (string, error) {
    wh, err := a.getOrCreateWebhook(ctx, channelID)
    if err != nil {
        // Fallback to regular bot message if webhook creation fails
        a.log.Warn("webhook fallback", "channel", channelID, "error", err)
        msg, err := a.session.ChannelMessageSendComplex(channelID, data)
        if err != nil {
            return "", err
        }
        return msg.ID, nil
    }

    params := &discordgo.WebhookParams{
        Content:    data.Content,
        Username:   agentSlug,
        AvatarURL:  a.iconProvider.IconURL(agentSlug),
        Embeds:     data.Embeds,
        Components: data.Components,
    }

    msg, err := a.session.WebhookExecute(wh.ID, wh.Token, true, params)
    if err != nil {
        return "", err
    }
    return msg.ID, nil
}
```

#### SendCard

```go
func (a *Adapter) SendCard(ctx context.Context, spaceID string, card chatapp.Card) (string, error) {
    return a.SendMessage(ctx, chatapp.SendMessageRequest{
        SpaceID: spaceID,
        Card:    &card,
    })
}
```

#### UpdateMessage

```go
func (a *Adapter) UpdateMessage(ctx context.Context, messageID string, req chatapp.SendMessageRequest) error {
    edit := &discordgo.MessageEdit{
        ID:      messageID,
        Channel: req.SpaceID,
        Content: &req.Text,
    }

    if req.Card != nil {
        embeds := []*discordgo.MessageEmbed{renderEmbed(req.Card)}
        edit.Embeds = &embeds
        components := renderComponents(req.Card)
        edit.Components = &components
    }

    _, err := a.session.ChannelMessageEditComplex(edit)
    return err
}
```

#### OpenDialog

```go
func (a *Adapter) OpenDialog(ctx context.Context, triggerID string, dialog chatapp.Dialog) error {
    // triggerID is the interaction ID + token, encoded as "interactionID:token"
    interactionID, token := parseInteractionRef(triggerID)

    response := &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseModal,
        Data: renderModal(&dialog),
    }

    return a.session.InteractionRespond(&discordgo.Interaction{
        ID:    interactionID,
        Token: token,
    }, response)
}
```

> **Discord limitation:** Discord modals cannot be updated after they are opened. `UpdateDialog()` is a no-op that returns `nil`. If the dialog content needs to change, the modal must be closed and reopened (which requires a new interaction trigger). In practice this is not an issue — the chat app's dialog flows are single-step forms that do not use `UpdateDialog()`.

#### GetUser

```go
func (a *Adapter) GetUser(ctx context.Context, userID string) (*chatapp.ChatUser, error) {
    // Check cache first
    if cached := a.userCache.get(userID); cached != nil {
        return cached, nil
    }

    user, err := a.session.User(userID)
    if err != nil {
        return nil, err
    }

    chatUser := &chatapp.ChatUser{
        PlatformID:  user.ID,
        DisplayName: user.GlobalName,
    }
    // Note: Email is NOT available via the bot API.
    // Discord only exposes email through OAuth2 user authorization
    // with the 'email' scope, which requires user consent flow.
    // chatUser.Email remains empty — identity mapping must use
    // device auth registration, not email-based auto-register.

    a.userCache.set(userID, chatUser)
    return chatUser, nil
}
```

This is the most significant difference from Google Chat and Slack. See the Identity section for details.

#### SetAgentIdentity

```go
func (a *Adapter) SetAgentIdentity(ctx context.Context, agent chatapp.AgentIdentity) error {
    // No-op: Discord agent identity is applied per-message via webhook
    return nil
}
```

---

## Event Handling

### Event Ingestion

The adapter receives events through two Discord API surfaces:

| Source | Event Types |
|--------|-------------|
| **Gateway WebSocket** | `READY`, `GUILD_CREATE`, `GUILD_DELETE`, `MESSAGE_CREATE`, `INTERACTION_CREATE`, `THREAD_CREATE` |
| **HTTP Interactions Endpoint** (optional) | Slash commands, button clicks, select menus, modal submissions |

When both are enabled, the HTTP endpoint takes priority for interactions (Discord only delivers interactions to one destination). The Gateway always handles non-interaction events.

#### Gateway Connection

```go
func (a *Adapter) Start() error {
    a.session.Identify.Intents = discordgo.IntentsGuilds |
        discordgo.IntentsGuildMessages |
        discordgo.IntentsDirectMessages

    if a.mentionRouting {
        a.session.Identify.Intents |= discordgo.IntentMessageContent
    }

    // Register event handlers
    a.session.AddHandler(a.handleReady)
    a.session.AddHandler(a.handleGuildCreate)
    a.session.AddHandler(a.handleGuildDelete)
    a.session.AddHandler(a.handleMessageCreate)
    a.session.AddHandler(a.handleInteractionCreate)

    if err := a.session.Open(); err != nil {
        return fmt.Errorf("opening gateway connection: %w", err)
    }

    // Optionally start HTTP interactions endpoint
    if a.httpServer != nil {
        go func() {
            if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                a.log.Error("interactions endpoint error", "error", err)
            }
        }()
    }

    return nil
}

func (a *Adapter) Stop(ctx context.Context) error {
    if a.httpServer != nil {
        a.httpServer.Shutdown(ctx)
    }
    return a.session.Close()
}
```

#### HTTP Interactions Endpoint (Optional)

```go
func (a *Adapter) handleHTTPInteraction(w http.ResponseWriter, r *http.Request) {
    // 1. Read body
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    // 2. Verify Ed25519 signature
    if !discordgo.VerifyInteraction(r, body, a.publicKey) {
        http.Error(w, "invalid signature", http.StatusUnauthorized)
        return
    }

    // 3. Parse interaction
    var interaction discordgo.Interaction
    if err := json.Unmarshal(body, &interaction); err != nil {
        http.Error(w, "bad request", http.StatusBadRequest)
        return
    }

    // 4. Handle PING (required for endpoint validation)
    if interaction.Type == discordgo.InteractionPing {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]int{"type": 1})
        return
    }

    // 5. Process interaction
    a.processInteraction(&interaction, w)
}
```

### Event Normalization

All Discord events are normalized to `chatapp.ChatEvent` before being dispatched to the shared `CommandRouter`:

```go
func (a *Adapter) normalizeSlashCommand(i *discordgo.InteractionCreate) *chatapp.ChatEvent {
    data := i.ApplicationCommandData()

    // Build args string from subcommand + options
    args := buildArgsFromOptions(data.Options)

    event := &chatapp.ChatEvent{
        Type:     chatapp.EventCommand,
        Platform: PlatformName,
        SpaceID:  i.ChannelID,
        UserID:   interactionUserID(i),
        Command:  "fabric",
        Args:     args,
    }

    return event
}

func (a *Adapter) normalizeMessageMention(m *discordgo.MessageCreate) *chatapp.ChatEvent {
    // Strip the bot mention prefix (e.g., "<@BOT_ID> tell deploy-agent...")
    text := stripBotMention(m.Content, a.session.State.User.ID)

    event := &chatapp.ChatEvent{
        Type:     chatapp.EventMessage,
        Platform: PlatformName,
        SpaceID:  m.ChannelID,
        UserID:   m.Author.ID,
        Text:     text,
    }

    // If the message is in a thread, capture the thread ID
    if m.Thread != nil {
        event.ThreadID = m.ChannelID // Thread IS a channel in Discord
    }

    return event
}

func (a *Adapter) normalizeComponentInteraction(i *discordgo.InteractionCreate) *chatapp.ChatEvent {
    data := i.MessageComponentData()

    event := &chatapp.ChatEvent{
        Type:     chatapp.EventAction,
        Platform: PlatformName,
        SpaceID:  i.ChannelID,
        UserID:   interactionUserID(i),
        ActionID: data.CustomID,
    }

    // For select menus, capture the selected value(s)
    if len(data.Values) > 0 {
        event.ActionData = strings.Join(data.Values, ",")
    }

    return event
}

func (a *Adapter) normalizeModalSubmit(i *discordgo.InteractionCreate) *chatapp.ChatEvent {
    data := i.ModalSubmitData()

    event := &chatapp.ChatEvent{
        Type:        chatapp.EventDialogSubmit,
        Platform:    PlatformName,
        SpaceID:     i.ChannelID,
        UserID:      interactionUserID(i),
        ActionID:    data.CustomID, // Modal's custom_id = submit action ID
        DialogData:  extractModalValues(data.Components),
    }

    return event
}
```

**Key normalization differences from Google Chat and Slack:**

| Aspect | Google Chat | Slack | Discord |
|--------|------------|-------|---------|
| User email | In every event payload | Must call `users.info` | Not available via bot API |
| Thread ID | `thread.name` (resource path) | `thread_ts` (message timestamp) | Thread channel ID (snowflake) |
| Space ID | `space.name` (resource path) | Channel ID (e.g., `C0ABC123`) | Channel ID (snowflake) |
| Message ID | Resource name | `ts` (timestamp) | Snowflake string |
| Action callbacks | `commonEventObject` params | `action_id` + `value` in block action | `custom_id` in component data |
| Dialog submissions | `commonEventObject.formInputs` | `view.state.values` | `modal_submit` interaction with component values |
| Slash command args | `message.argumentText` | `text` field of slash command | Options tree from application command data |
| Bot added to guild | `addedToSpacePayload` | `member_joined_channel` event | `GUILD_CREATE` event |
| Bot removed | `removedFromSpacePayload` | `member_left_channel` event | `GUILD_DELETE` event |
| Event delivery | HTTP POST (webhook) | HTTP POST or Socket Mode (WSS) | Gateway (WSS) or HTTP POST (interactions only) |
| Interaction model | Synchronous HTTP response | Acknowledge within 3s, then async | Acknowledge within 3s, then async |

### Interaction Response Model

Discord's interaction response pattern is the most structured of the three platforms. When the adapter receives an interaction, it must respond within 3 seconds with a specific response type:

| Response Type | Value | When to use |
|-------------|-------|-------------|
| `InteractionResponseChannelMessageWithSource` | 4 | Immediate short response |
| `InteractionResponseDeferredChannelMessageWithSource` | 5 | Acknowledge, send follow-up later |
| `InteractionResponseModal` | 9 | Open a modal form |
| `InteractionResponseUpdateMessage` | 7 | Update the message a button was on |
| `InteractionResponseDeferredMessageUpdate` | 6 | Acknowledge button click, update later |

For commands that require Hub API calls:

```go
func (a *Adapter) processSlashCommand(i *discordgo.InteractionCreate) {
    // 1. Acknowledge with deferred response (shows "Fabric is thinking...")
    a.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Flags: discordgo.MessageFlagsEphemeral, // Only for ephemeral commands
        },
    })

    // 2. Process command asynchronously
    go func() {
        event := a.normalizeSlashCommand(i)
        resp, err := a.handler(context.Background(), event)
        if err != nil {
            a.log.Error("command handler error", "error", err)
            a.editFollowup(i, "An error occurred processing your command.")
            return
        }

        // 3. Send follow-up with the actual response
        if resp != nil {
            a.sendInteractionFollowup(i, resp)
        }
    }()
}
```

Follow-up messages are sent via the interaction webhook, which remains valid for 15 minutes:

```go
func (a *Adapter) sendInteractionFollowup(i *discordgo.InteractionCreate, resp *chatapp.EventResponse) {
    if resp.Message != nil {
        params := &discordgo.WebhookParams{
            Content: resp.Message.Text,
        }
        if resp.Message.Card != nil {
            params.Embeds = []*discordgo.MessageEmbed{renderEmbed(resp.Message.Card)}
            params.Components = renderComponents(resp.Message.Card)
        }
        a.session.FollowupMessageCreate(i.Interaction, true, params)
    }

    if resp.Dialog != nil {
        // Modal can only be opened from the initial response, not a follow-up.
        // If the dialog was not opened in the initial interaction response,
        // post a message with a button that opens the modal on click.
        a.sendModalTrigger(i, resp.Dialog)
    }
}
```

The `EventResponse` type maps to Discord responses as follows:

| EventResponse field | Discord behavior |
|--------------------|---------------|
| `Message` | Follow-up message via interaction webhook |
| `UpdateMessage` | `InteractionResponseUpdateMessage` on the originating message |
| `Dialog` | `InteractionResponseModal` from interaction (or modal-trigger button if deferred) |
| `CloseDialog` | No-op (Discord modals auto-close on submit) |
| `Notification` | Ephemeral follow-up message (`MessageFlagsEphemeral`) |

### Ephemeral vs. Public Responses

Discord natively supports ephemeral messages (visible only to the invoking user) via the `MessageFlagsEphemeral` flag. This maps directly to the same command visibility split used in the Slack adapter:

| Command | Visibility | Reason |
|---------|-----------|--------|
| `help` | Ephemeral | Only the invoker needs to see it |
| `info` | Ephemeral | Contains user-specific registration info |
| `register` / `unregister` | Ephemeral | User-specific auth flow |
| `broadcast` | Ephemeral | Channel admin configuration |
| All others | Public | Team should see agent operations |

The adapter sets `MessageFlagsEphemeral` on the initial deferred response for ephemeral commands and on all follow-up messages for that interaction.

---

## Embed Rendering

### Card to Discord Embed Mapping

The platform-agnostic `Card` model maps to Discord Embeds as follows:

| Card Element | Discord Embed Element |
|-------------|----------------------|
| `CardHeader.Title` | `embed.Title` |
| `CardHeader.Subtitle` | `embed.Description` (first line) |
| `CardHeader.IconURL` | `embed.Thumbnail.URL` |
| `CardSection` with header | Bold text in `embed.Description` or separate field with section name |
| `WidgetText` | `embed.Description` paragraph or `embed.Field` (inline: false) |
| `WidgetKeyValue` | `embed.Field` with Name=label, Value=content (inline: true) |
| `WidgetButton` | Action Row component (see Components below) |
| `WidgetDivider` | `───` separator line in description |
| `WidgetImage` | `embed.Image.URL` |
| `WidgetInput` | _(not rendered in embed; triggers modal on interaction)_ |
| `WidgetCheckbox` | _(rendered as select menu component)_ |
| `CardAction` list | Action Row with Button components |

### Rendering Implementation

```go
// embeds.go

func renderEmbed(card *chatapp.Card) *discordgo.MessageEmbed {
    embed := &discordgo.MessageEmbed{}

    // Header
    if card.Header.Title != "" {
        embed.Title = card.Header.Title
    }
    if card.Header.Subtitle != "" {
        embed.Description = card.Header.Subtitle
    }
    if card.Header.IconURL != "" {
        embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
            URL: card.Header.IconURL,
        }
    }

    // Color based on notification type (set by caller via card metadata)
    embed.Color = embedColor(card)

    // Sections → Fields
    for _, section := range card.Sections {
        if section.Header != "" {
            embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
                Name:  section.Header,
                Value: "​", // Zero-width space (Discord requires non-empty value)
            })
        }
        for _, widget := range section.Widgets {
            fields := renderWidgetFields(&widget)
            embed.Fields = append(embed.Fields, fields...)
        }
    }

    return embed
}

func renderWidgetFields(w *chatapp.Widget) []*discordgo.MessageEmbedField {
    switch w.Type {
    case chatapp.WidgetText:
        return []*discordgo.MessageEmbedField{{
            Name:   "​",
            Value:  w.Content,
            Inline: false,
        }}

    case chatapp.WidgetKeyValue:
        return []*discordgo.MessageEmbedField{{
            Name:   w.Label,
            Value:  w.Content,
            Inline: true,
        }}

    case chatapp.WidgetImage:
        // Images cannot be inline fields; handled separately
        // Return a field with a link to the image
        return []*discordgo.MessageEmbedField{{
            Name:   w.Label,
            Value:  fmt.Sprintf("[View Image](%s)", w.Content),
            Inline: false,
        }}

    case chatapp.WidgetDivider:
        return []*discordgo.MessageEmbedField{{
            Name:   "​",
            Value:  "───────────────────",
            Inline: false,
        }}

    default:
        // Buttons, inputs, checkboxes are rendered as components, not embed fields
        return nil
    }
}
```

### Notification Embed Colors

Discord embeds support a sidebar color. The adapter maps notification activity types to colors:

```go
func embedColor(card *chatapp.Card) int {
    // Color is communicated via a convention in CardHeader.Subtitle or a dedicated field.
    // The notification relay already prefixes status text with emoji indicators.
    // Map known patterns to colors:
    colorMap := map[string]int{
        "COMPLETED":         0x2ECC71, // Green
        "WAITING_FOR_INPUT": 0xF1C40F, // Yellow
        "ERROR":             0xE74C3C, // Red
        "STALLED":           0xE67E22, // Orange
        "LIMITS_EXCEEDED":   0xE67E22, // Orange
        "DELETED":           0x95A5A6, // Gray
    }
    // Default: Fabric brand color
    return 0x1A1A2E
}
```

### Discord Embed Limits

Discord imposes specific limits on embeds:

| Limit | Value | Mitigation |
|-------|-------|------------|
| Embeds per message | 10 | Only 1 embed per card in current design |
| Title length | 256 chars | Truncate card header title |
| Description length | 4096 chars | Truncate long descriptions |
| Fields per embed | 25 | Truncate long agent lists; paginate |
| Field name length | 256 chars | Labels already short |
| Field value length | 1024 chars | Truncate log output |
| Total embed chars | 6000 | Monitor total; split into multiple embeds if needed |
| Action rows per message | 5 | Max 5 rows of buttons/selects |
| Buttons per action row | 5 | Group actions into rows of 5 |
| Components per message | 25 total | Unlikely to hit in current design |

The existing 2000-char truncation for log output in `cmdLogs()` satisfies the field value limit. The 25-field limit is tighter than Slack's 50-block limit and may require pagination for large agent lists.

---

## Message Components

Discord Message Components (buttons, select menus) are sent alongside embeds in Action Rows. They are separate from the embed — unlike Slack Block Kit where buttons are blocks within the same structure.

### Component Rendering

```go
// components.go

func renderComponents(card *chatapp.Card) []discordgo.MessageComponent {
    var rows []discordgo.MessageComponent

    // Collect buttons from widget-level and card-level actions
    var buttons []discordgo.MessageComponent

    for _, section := range card.Sections {
        for _, widget := range section.Widgets {
            switch widget.Type {
            case chatapp.WidgetButton:
                btn := discordgo.Button{
                    Label:    widget.Label,
                    Style:    discordgo.PrimaryButton,
                    CustomID: widget.ActionID,
                }
                buttons = append(buttons, btn)

            case chatapp.WidgetCheckbox:
                // Render as a string select menu
                var options []discordgo.SelectMenuOption
                for _, opt := range widget.Options {
                    options = append(options, discordgo.SelectMenuOption{
                        Label: opt.Label,
                        Value: opt.Value,
                    })
                }
                menu := discordgo.SelectMenu{
                    CustomID:    widget.ActionID,
                    Placeholder: widget.Label,
                    MenuType:    discordgo.StringSelectMenu,
                    Options:     options,
                    MinValues:   intPtr(0),
                    MaxValues:   len(options),
                }
                rows = append(rows, discordgo.ActionsRow{
                    Components: []discordgo.MessageComponent{menu},
                })
            }
        }
    }

    // Card-level actions
    for _, action := range card.Actions {
        style := discordgo.PrimaryButton
        switch action.Style {
        case "primary":
            style = discordgo.PrimaryButton
        case "danger":
            style = discordgo.DangerButton
        default:
            style = discordgo.SecondaryButton
        }
        btn := discordgo.Button{
            Label:    action.Label,
            Style:    style,
            CustomID: action.ActionID,
        }
        buttons = append(buttons, btn)
    }

    // Group buttons into action rows (max 5 per row)
    for i := 0; i < len(buttons); i += 5 {
        end := i + 5
        if end > len(buttons) {
            end = len(buttons)
        }
        rows = append(rows, discordgo.ActionsRow{
            Components: buttons[i:end],
        })
    }

    return rows
}
```

### Component Interaction Handling

When a user clicks a button or selects from a menu, Discord sends an `INTERACTION_CREATE` event with type `MessageComponent`. The adapter normalizes this to `ChatEvent` with `Type: EventAction`:

```go
func (a *Adapter) handleComponentInteraction(i *discordgo.InteractionCreate) {
    // Acknowledge immediately (update the message to remove the "loading" state)
    a.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseDeferredMessageUpdate,
    })

    // Normalize and dispatch
    event := a.normalizeComponentInteraction(i)
    go func() {
        resp, err := a.handler(context.Background(), event)
        if err != nil {
            a.log.Error("component handler error", "error", err)
            return
        }
        if resp != nil {
            a.sendInteractionFollowup(i, resp)
        }
    }()
}
```

---

## Modal Rendering (Dialogs)

### Discord Modal Constraints

Discord Modals are more limited than Slack Modals:

| Feature | Slack Modal | Discord Modal |
|---------|-------------|---------------|
| Text inputs | Yes | Yes |
| Textarea inputs | Yes | Yes (paragraph style) |
| Select menus | Yes (static_select) | **No** |
| Checkboxes | Yes | **No** |
| Max components | ~100 blocks | 5 action rows (1 text input each) |
| Update after open | Yes (`views.update`) | **No** |
| Stacked modals | Yes (`views.push`) | **No** |

This means `Dialog` fields of type `select` and `checkbox` cannot be rendered in a Discord modal. The adapter handles this with a two-phase approach:

1. **Text/textarea fields** → Rendered in the Discord modal directly
2. **Select/checkbox fields** → Rendered as message components (select menus) in a pre-modal message, with the modal triggered after selections are made

For the common dialog flows in the chat app, this works well:

| Dialog | Fields | Discord Rendering |
|--------|--------|-------------------|
| Ask-user response | 1 text input | Single modal with text input |
| Delete confirmation | None (just confirm/cancel) | Button confirmation (no modal needed) |
| Register device auth | 1 text display | Ephemeral message with instructions |
| Subscribe activity filter | Checkboxes | Select menu in message |
| Create agent | Text + select | Modal for name → select menu for template |

### Modal Rendering Implementation

```go
// modals.go

func renderModal(dialog *chatapp.Dialog) *discordgo.InteractionResponseData {
    var components []discordgo.MessageComponent

    for _, field := range dialog.Fields {
        switch field.Type {
        case "text":
            input := discordgo.TextInput{
                CustomID:    field.ID,
                Label:       truncate(field.Label, 45), // Discord label limit
                Style:       discordgo.TextInputShort,
                Placeholder: field.Placeholder,
                Required:    field.Required,
            }
            components = append(components, discordgo.ActionsRow{
                Components: []discordgo.MessageComponent{input},
            })

        case "textarea":
            input := discordgo.TextInput{
                CustomID:    field.ID,
                Label:       truncate(field.Label, 45),
                Style:       discordgo.TextInputParagraph,
                Placeholder: field.Placeholder,
                Required:    field.Required,
            }
            components = append(components, discordgo.ActionsRow{
                Components: []discordgo.MessageComponent{input},
            })

        case "select", "checkbox":
            // Cannot render in modal — handled separately as message components
            continue
        }
    }

    // Discord allows max 5 action rows in a modal
    if len(components) > 5 {
        components = components[:5]
    }

    return &discordgo.InteractionResponseData{
        CustomID:   dialog.Submit.ActionID,
        Title:      truncate(dialog.Title, 45), // Discord title limit
        Components: components,
    }
}
```

### Modal Value Extraction

When a modal is submitted, Discord sends an `INTERACTION_CREATE` event with type `ModalSubmit`. The adapter extracts values from the nested component structure:

```go
func extractModalValues(components []discordgo.MessageComponent) map[string]string {
    result := make(map[string]string)

    for _, row := range components {
        actionsRow, ok := row.(*discordgo.ActionsRow)
        if !ok {
            continue
        }
        for _, comp := range actionsRow.Components {
            input, ok := comp.(*discordgo.TextInput)
            if !ok {
                continue
            }
            result[input.CustomID] = input.Value
        }
    }

    return result
}
```

---

## Dynamic Agent Identity

### Channel Webhooks for Per-Agent Personas

Discord bots have a fixed name and avatar set in the Developer Portal. To achieve per-agent identity (like Slack's `chat:write.customize`), Discord offers **channel webhooks**. A webhook can send messages with a custom `username` and `avatar_url` per execution.

The adapter creates one webhook per channel (named "Fabric Agent Relay") and reuses it for all agent messages in that channel. The webhook is created lazily on first use and cached.

```go
// webhooks.go

func (a *Adapter) getOrCreateWebhook(ctx context.Context, channelID string) (*discordgo.Webhook, error) {
    // Check cache
    a.webhooksMu.RLock()
    if wh, ok := a.webhooks[channelID]; ok {
        a.webhooksMu.RUnlock()
        return wh, nil
    }
    a.webhooksMu.RUnlock()

    // Check existing channel webhooks
    webhooks, err := a.session.ChannelWebhooks(channelID)
    if err != nil {
        return nil, fmt.Errorf("listing webhooks: %w", err)
    }

    for _, wh := range webhooks {
        if wh.Name == "Fabric Agent Relay" && wh.User.ID == a.session.State.User.ID {
            a.webhooksMu.Lock()
            a.webhooks[channelID] = wh
            a.webhooksMu.Unlock()
            return wh, nil
        }
    }

    // Create new webhook
    wh, err := a.session.WebhookCreate(channelID, "Fabric Agent Relay", "")
    if err != nil {
        return nil, fmt.Errorf("creating webhook: %w", err)
    }

    a.webhooksMu.Lock()
    a.webhooks[channelID] = wh
    a.webhooksMu.Unlock()
    return wh, nil
}
```

### Webhook vs. Bot Message Routing

| Message Source | Send Method | Identity Shown |
|---------------|-------------|----------------|
| System messages (command responses, help) | Bot API (`ChannelMessageSendComplex`) | Fabric bot (fixed name/avatar) |
| Agent notifications | Channel webhook (`WebhookExecute`) | Agent slug + RoboHash avatar |
| Agent-to-user messages | Channel webhook (`WebhookExecute`) | Agent slug + RoboHash avatar |

This produces the same per-agent appearance as the Slack adapter:

```
deploy-agent [BOT]  2:45 PM
┌─────────────────────────────────────┐
│ Completed | Deployment finished     │
│                                     │
│ All health checks passing.          │
│                                     │
│ [View Logs]                         │
└─────────────────────────────────────┘
```

### Agent Icon Generation

Same `IconProvider` abstraction as the Slack adapter, defaulting to [RoboHash](https://robohash.org/):

```go
type IconProvider interface {
    IconURL(agentSlug string) string
}

type robohashProvider struct{}

func (r *robohashProvider) IconURL(agentSlug string) string {
    return fmt.Sprintf("https://robohash.org/%s?set=set1&size=128x128", url.PathEscape(agentSlug))
}
```

Discord recommends webhook avatars be at least 128x128 pixels (larger than Slack's 48x48).

### Webhook Lifecycle

Webhooks are managed by the adapter:

- **Created** lazily when the first agent message is sent to a channel
- **Cached** in memory for the lifetime of the adapter process
- **Reused** across all agent messages in the same channel (different agents use different `username`/`avatar_url` per execution)
- **Cleaned up** when a space is unlinked from a grove (optional — orphaned webhooks are harmless)

If the webhook is deleted externally (e.g., by a guild admin), the adapter detects the error on next send and recreates it.

---

## Threading

### Thread Model

Discord threads are full channel objects (with their own snowflake IDs) created from a parent message or as standalone threads in a channel. This is fundamentally different from both Google Chat (thread keys) and Slack (thread timestamps).

| Concept | Google Chat | Slack | Discord |
|---------|------------|-------|---------|
| Thread ID type | Resource name string | Message timestamp | Channel ID (snowflake) |
| Thread reference | `thread.name` | `thread_ts` | Thread channel ID |
| New thread | Omit thread, or use unique `threadKey` | Omit `thread_ts` | `MessageStartThread` or `ThreadStart` |
| Reply to thread | Set `thread.name` | Set `thread_ts` to parent's `ts` | Send message to thread channel ID |
| Thread lifetime | Permanent | Permanent | Configurable auto-archive (1h/24h/3d/7d) |

### Implementation

When a notification card is posted, the adapter can create a thread from it for follow-up conversation:

```go
func (a *Adapter) sendNotificationWithThread(ctx context.Context, channelID string, card *chatapp.Card, agentSlug string) (messageID string, threadID string, err error) {
    // 1. Send the notification message
    msgID, err := a.SendMessage(ctx, chatapp.SendMessageRequest{
        SpaceID: channelID,
        Card:    card,
        AgentID: agentSlug,
    })
    if err != nil {
        return "", "", err
    }

    // 2. Create a thread from the notification for follow-up
    thread, err := a.session.MessageThreadStartComplex(channelID, msgID, &discordgo.ThreadStart{
        Name:                agentSlug + " — conversation",
        AutoArchiveDuration: 1440, // 24 hours
        Type:                discordgo.ChannelTypeGuildPublicThread,
    })
    if err != nil {
        // Thread creation failed — not fatal, just no threading
        a.log.Warn("thread creation failed", "error", err)
        return msgID, "", nil
    }

    return msgID, thread.ID, nil
}
```

When sending replies within a thread, the adapter sends to the thread's channel ID directly:

```go
if req.ThreadID != "" {
    // ThreadID IS a channel ID in Discord — send directly to it
    msg, err := a.session.ChannelMessageSendComplex(req.ThreadID, data)
    if err != nil {
        return "", err
    }
    return msg.ID, nil
}
```

The `/fabric message --thread <thread-id>` command works with Discord thread IDs (snowflake strings like `1234567890123456789`).

### Thread Auto-Archive

Discord threads auto-archive after a configurable inactivity period. The adapter creates notification threads with a 24-hour auto-archive duration by default. Archived threads can be unarchived by posting a new message. This is a natural fit for notification conversations — active conversations stay open, stale ones clean up automatically.

---

## User Identity & Mentions

### Email Unavailability

This is the most significant identity difference from Google Chat and Slack.

| Platform | Email Retrieval | Auto-Register by Email |
|----------|----------------|----------------------|
| Google Chat | In every event payload (Google-asserted) | Yes — primary path |
| Slack | Via `users.info` API call (requires `users:read.email` scope) | Yes — with API call |
| Discord | **Not available** via bot API; requires OAuth2 user authorization | **No** |

Discord's bot API (`GET /users/{user.id}`) returns username, discriminator, avatar, and global display name — but **not email**. Email is only available through the OAuth2 authorization flow with the `email` scope, which requires:

1. Redirecting the user to Discord's authorization page
2. User explicitly granting email access
3. Exchanging an authorization code for an access token
4. Calling `GET /users/@me` with the access token

This is a heavyweight user interaction that doesn't fit the casual `/fabric register` flow.

### Registration Strategy

For Discord, the registration flow changes:

1. **Auto-register by email: Not available.** The `eventUserLookup` will return an empty email for Discord events. The `ResolveOrAutoRegister()` path will always fall through to the manual registration flow.

2. **Manual registration via device auth: Primary path.** When a Discord user runs `/fabric register`, they are guided through the device auth flow (same as the fallback path for Google Chat and Slack):
   - Bot responds with a device code + verification URL (ephemeral message)
   - User visits the URL, logs into the Fabric Hub, enters the code
   - Mapping is created: Discord user ID ↔ Hub user ID

3. **Explicit email registration: Optional enhancement.** A Discord-specific subcommand could allow users to provide their email directly:
   ```
   /fabric register email:alice@example.com
   ```
   The adapter would attempt to match this email against Hub users. If a match is found and the email is confirmed through the device auth flow, the mapping is created. This combines the convenience of email matching with the security of device auth verification.

### @Mention Formatting

Discord @mentions use the format `<@USER_ID>` (wrapping the snowflake user ID in angle brackets with an `@` prefix). The `formatMention()` function in `notifications.go` adds a Discord case:

```go
func formatMention(platform, platformUserID string) string {
    switch platform {
    case "discord":
        return fmt.Sprintf("<@%s>", platformUserID)
    case "slack":
        return fmt.Sprintf("<@%s>", platformUserID)
    case "google_chat":
        return fmt.Sprintf("<users/%s>", platformUserID)
    default:
        return platformUserID
    }
}
```

Note that Discord and Slack share the same mention format (`<@ID>`) despite using different ID systems (snowflakes vs. Slack user IDs).

### Discord User Lookup

```go
type discordUserLookup struct {
    adapter *Adapter
}

func (dl *discordUserLookup) GetUser(ctx context.Context, userID string) (*identity.ChatUserInfo, error) {
    user, err := dl.adapter.GetUser(ctx, userID)
    if err != nil {
        return nil, err
    }
    return &identity.ChatUserInfo{
        PlatformID: user.PlatformID,
        // Email intentionally empty — Discord does not provide it via bot API.
        // This forces the manual registration path in ResolveOrAutoRegister().
    }, nil
}
```

### User Profile Caching

Discord rate limits are per-route and more restrictive than Slack's:

```go
type userCache struct {
    mu    sync.RWMutex
    users map[string]*cachedUser
}

type cachedUser struct {
    user      *chatapp.ChatUser
    fetchedAt time.Time
}

const userCacheTTL = 30 * time.Minute // Longer TTL since Discord profiles change less frequently
```

---

## Slash Command Registration

### Application Commands

Discord slash commands are registered via the Discord API, not through a web portal configuration. The adapter registers the `/fabric` command on startup with subcommands matching the existing command set:

```go
// commands.go

func (a *Adapter) registerCommands() error {
    commands := []*discordgo.ApplicationCommand{
        {
            Name:        "fabric",
            Description: "Fabric agent management",
            Options: []*discordgo.ApplicationCommandOption{
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "list",
                    Description: "List agents in the linked grove",
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "status",
                    Description: "Show agent status",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "agent",
                            Description: "Agent name",
                            Required:    true,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "start",
                    Description: "Start an agent",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "agent",
                            Description: "Agent name",
                            Required:    true,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "stop",
                    Description: "Stop an agent",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "agent",
                            Description: "Agent name",
                            Required:    true,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "create",
                    Description: "Create a new agent",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "name",
                            Description: "Agent name",
                            Required:    true,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "delete",
                    Description: "Delete an agent",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "agent",
                            Description: "Agent name",
                            Required:    true,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "logs",
                    Description: "View agent logs",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "agent",
                            Description: "Agent name",
                            Required:    true,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "message",
                    Description: "Send a message to an agent",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "agent",
                            Description: "Agent name",
                            Required:    true,
                        },
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "text",
                            Description: "Message text",
                            Required:    true,
                        },
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "thread",
                            Description: "Thread ID for threaded reply",
                            Required:    false,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "link",
                    Description: "Link this channel to a grove",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "grove",
                            Description: "Grove slug",
                            Required:    true,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "unlink",
                    Description: "Unlink this channel from its grove",
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "register",
                    Description: "Register your Discord account with the Fabric Hub",
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "unregister",
                    Description: "Remove your Discord-Hub account link",
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "subscribe",
                    Description: "Subscribe to agent notifications",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "agent",
                            Description: "Agent name",
                            Required:    true,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "unsubscribe",
                    Description: "Unsubscribe from agent notifications",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "agent",
                            Description: "Agent name",
                            Required:    true,
                        },
                    },
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "info",
                    Description: "Show your registration info and linked grove",
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "help",
                    Description: "Show available commands",
                },
                {
                    Type:        discordgo.ApplicationCommandOptionSubCommand,
                    Name:        "broadcast",
                    Description: "Configure notification broadcast policy",
                    Options: []*discordgo.ApplicationCommandOption{
                        {
                            Type:        discordgo.ApplicationCommandOptionString,
                            Name:        "policy",
                            Description: "Activity types to broadcast (e.g., ERROR WAITING_FOR_INPUT), 'all', or 'none'",
                            Required:    false,
                        },
                    },
                },
            },
        },
    }

    guildID := a.guildID // Empty string = global commands
    for _, cmd := range commands {
        _, err := a.session.ApplicationCommandCreate(a.appID, guildID, cmd)
        if err != nil {
            return fmt.Errorf("registering command %s: %w", cmd.Name, err)
        }
    }

    return nil
}
```

### Global vs. Guild Commands

| Mode | When to use | Propagation delay |
|------|-------------|-------------------|
| **Global** (`guildID = ""`) | Production | Up to 1 hour for new guilds |
| **Guild-specific** (`guildID = "123..."`) | Development and testing | Instant |

The adapter defaults to global commands in production. The `guild_id` config field overrides this for development.

### Subcommand Argument Extraction

Discord slash commands deliver arguments as structured `ApplicationCommandInteractionDataOption` trees, not as a raw text string. The adapter flattens these back to the `Args` string expected by the shared `CommandRouter`:

```go
func buildArgsFromOptions(options []*discordgo.ApplicationCommandInteractionDataOption) string {
    if len(options) == 0 {
        return ""
    }

    // First option is the subcommand
    sub := options[0]
    args := sub.Name

    // Subcommand options become space-separated arguments
    for _, opt := range sub.Options {
        switch opt.Type {
        case discordgo.ApplicationCommandOptionString:
            if opt.Name == "thread" {
                args += " --thread " + opt.StringValue()
            } else {
                args += " " + opt.StringValue()
            }
        }
    }

    return args
}
```

This means `/fabric status my-agent` produces `Args: "status my-agent"`, which the existing `CommandRouter.handleCommand()` parses identically to Google Chat and Slack.

### Command Cleanup

On shutdown, the adapter can optionally deregister its commands. This is useful for development but should be disabled in production (global commands take up to an hour to propagate):

```go
func (a *Adapter) deregisterCommands() error {
    cmds, err := a.session.ApplicationCommands(a.appID, a.guildID)
    if err != nil {
        return err
    }
    for _, cmd := range cmds {
        a.session.ApplicationCommandDelete(a.appID, a.guildID, cmd.ID)
    }
    return nil
}
```

---

## Guild & Channel Management

### Guild Events

When the bot is added to or removed from a guild, Discord sends `GUILD_CREATE` and `GUILD_DELETE` events:

```go
func (a *Adapter) handleGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
    a.log.Info("joined guild", "guild", g.Name, "id", g.ID)
    // No automatic space-grove linking — users must explicitly /fabric link
}

func (a *Adapter) handleGuildDelete(s *discordgo.Session, g *discordgo.GuildDelete) {
    a.log.Info("removed from guild", "id", g.ID)

    // Clean up all space links for channels in this guild.
    // Note: We don't receive individual channel IDs in GUILD_DELETE,
    // so we query the state store for all links with this guild's channels.
    // The adapter tracks guild→channel mappings from GUILD_CREATE events.
}
```

### Channel-Level Linking

Space-grove linking operates at the channel level (not guild level), consistent with Slack's channel-level linking:

```
/fabric link production     → Links current channel to "production" grove
/fabric unlink              → Unlinks current channel
```

A guild can have multiple channels linked to different groves (e.g., `#deploy` → production, `#ml-team` → ml-experiments).

### Permission Checks

Discord provides rich role-based permissions. The adapter can leverage these for authorization:

```go
func (a *Adapter) hasAdminPermission(guildID, userID string) bool {
    member, err := a.session.GuildMember(guildID, userID)
    if err != nil {
        return false
    }

    perms, err := a.session.UserChannelPermissions(userID, guildID)
    if err != nil {
        return false
    }

    return perms&discordgo.PermissionManageGuild != 0 ||
        perms&discordgo.PermissionAdministrator != 0
}
```

Guild admins can `/fabric link` and `/fabric broadcast`. Non-admin users can use all other commands based on their Hub permissions (resolved via identity mapping).

---

## Configuration

### DiscordConfig

A new `DiscordConfig` struct is added alongside the existing platform configs:

```go
type DiscordConfig struct {
    Enabled          bool   `yaml:"enabled"`
    BotToken         string `yaml:"bot_token"`          // Bot token from Developer Portal
    ApplicationID    string `yaml:"application_id"`     // Application ID for command registration
    PublicKey        string `yaml:"public_key"`         // Ed25519 public key (HTTP interactions)
    ListenAddress    string `yaml:"listen_address"`     // HTTP listen address (optional)
    InteractionsHTTP bool   `yaml:"interactions_http"`  // Use HTTP interactions endpoint
    MentionRouting   bool   `yaml:"mention_routing"`    // Enable @mention routing (needs MESSAGE_CONTENT)
    GuildID          string `yaml:"guild_id"`           // Restrict to single guild (dev mode)
}
```

### Full Configuration Example

```yaml
# fabric-chat-app.yaml

hub:
  endpoint: "https://hub.example.com"
  user: "chat-app@example.com"

plugin:
  listen_address: "localhost:9090"

platforms:
  google_chat:
    enabled: false

  slack:
    enabled: false

  discord:
    enabled: true
    bot_token: "${DISCORD_BOT_TOKEN}"
    application_id: "${DISCORD_APP_ID}"
    public_key: "${DISCORD_PUBLIC_KEY}"     # Only needed for interactions_http
    listen_address: ":8445"                 # Only needed for interactions_http
    interactions_http: false                 # Default: Gateway-only
    mention_routing: false                  # Default: slash commands only
    guild_id: ""                            # Empty = global commands (production)

state:
  database: "/var/lib/fabric-chat-app/state.db"

notifications:
  trigger_activities:
    - COMPLETED
    - WAITING_FOR_INPUT
    - ERROR
    - STALLED
    - LIMITS_EXCEEDED

logging:
  level: "info"
  format: "json"
```

---

## Changes to Shared Code

The Discord adapter is designed to require minimal changes to shared code. Most changes were already made or planned for the Slack adapter; Discord piggybacks on those.

### 1. Platform-Aware Mention Formatting (notifications.go)

Add a `"discord"` case to `formatMention()`. If the Slack adapter's platform-aware mention formatting is already implemented, this is a single additional case:

```go
case "discord":
    return fmt.Sprintf("<@%s>", platformUserID)
```

### 2. User Email Lookup (commands.go)

The `eventUserLookup` fallback to `Messenger.GetUser()` (added for Slack) also serves Discord. The difference is that for Discord, `GetUser()` returns an empty email, which causes `ResolveOrAutoRegister()` to fall through to the device auth path. No additional changes needed.

### 3. PlatformsConfig (config.go)

Add the `Discord` field:

```go
type PlatformsConfig struct {
    GoogleChat GoogleChatConfig `yaml:"google_chat"`
    Slack      SlackConfig      `yaml:"slack"`
    Discord    DiscordConfig    `yaml:"discord"`
}
```

### 4. main.go Adapter Initialization

Add Discord adapter startup alongside Google Chat and Slack:

```go
if cfg.Platforms.Discord.Enabled {
    discordAdapter := discord.NewAdapter(discord.Config{
        BotToken:         cfg.Platforms.Discord.BotToken,
        ApplicationID:    cfg.Platforms.Discord.ApplicationID,
        PublicKey:        cfg.Platforms.Discord.PublicKey,
        ListenAddress:    cfg.Platforms.Discord.ListenAddress,
        InteractionsHTTP: cfg.Platforms.Discord.InteractionsHTTP,
        MentionRouting:   cfg.Platforms.Discord.MentionRouting,
        GuildID:          cfg.Platforms.Discord.GuildID,
    }, router.HandleEvent, log.With("component", "discord"))

    router.RegisterMessenger(PlatformName, discordAdapter)

    go func() {
        if err := discordAdapter.Start(); err != nil {
            log.Error("discord adapter error", "error", err)
        }
    }()
}
```

### 5. Multi-Platform Messenger Dispatch

Same approach as described in the Slack design — the `CommandRouter` holds messengers keyed by platform name. Discord is a third entry. If the multi-messenger dispatch is already implemented for Slack, no additional work is needed here.

### 6. State Schema — No Changes

The existing `user_mappings`, `space_links`, and `agent_subscriptions` tables are platform-agnostic. Discord uses `platform = "discord"` and Discord snowflake IDs for `platform_user_id` and `space_id`. No schema changes needed.

The `space_settings` table (added by the Slack adapter for broadcast policy) is also reused by Discord without changes.

---

## Rate Limiting & API Constraints

Discord enforces rate limits per-route with specific bucket semantics:

| API Route | Rate Limit | Mitigation |
|-----------|-----------|------------|
| `POST /channels/{id}/messages` | 5 req/5s per channel | Queue outbound messages; serialize per-channel |
| `PATCH /channels/{id}/messages/{id}` | 5 req/5s per channel | Avoid rapid updates |
| `POST /interactions/{id}/{token}/callback` | 1 per interaction | Already single-response by design |
| `POST /webhooks/{id}/{token}` | 5 req/5s per webhook | Serialize agent messages per-channel |
| `GET /users/{id}` | 30 req/30s | Cache user profiles (30 min TTL) |
| `POST /channels/{id}/threads` | 10 per 10 min | Create threads conservatively |
| **Global rate limit** | 50 req/s total | Monitor aggregate usage |

The `discordgo` library handles `429 Too Many Requests` responses with automatic retry based on the `Retry-After` header. The adapter relies on this built-in handling.

For notification fan-out (sending cards to multiple channels for a grove), the adapter serializes sends with a small delay between channels to avoid hitting per-route limits.

### Gateway Rate Limits

The Gateway connection has its own limits:

| Limit | Value | Notes |
|-------|-------|-------|
| Identify | 1 per 5s | Only on initial connect |
| Gateway commands | 120 per 60s | Includes heartbeats |
| Presence updates | 5 per 60s | Not used by this adapter |

The `discordgo` library manages heartbeating and reconnection automatically.

---

## Testing Strategy

### Unit Tests

- **Embed rendering:** Verify `renderEmbed()` produces correct embed structures for all card types. Validate against Discord's embed limits.
- **Component rendering:** Verify `renderComponents()` produces correct action rows with buttons and select menus.
- **Modal rendering:** Verify `renderModal()` produces valid modal structures. Test text/textarea fields; verify select/checkbox fields are excluded.
- **Event normalization:** Verify all Discord event types normalize correctly to `ChatEvent`. Test slash commands with various subcommand/option combinations.
- **Argument extraction:** Verify `buildArgsFromOptions()` produces the same `Args` string format as Google Chat and Slack.
- **Signature verification:** Test Ed25519 verification with valid and invalid signatures.
- **Webhook cache:** Test cache hit/miss behavior, including fallback when webhook is deleted.
- **User cache:** Test cache hit/miss/expiry behavior.

### Integration Tests

- **Round-trip:** Simulate a slash command → command router → response → embed render cycle.
- **Component flow:** Simulate button click → interaction → command handler → follow-up message.
- **Modal flow:** Simulate interaction → modal open → submission → `EventDialogSubmit` → response.
- **Webhook identity:** Verify agent messages use webhook with correct username/avatar.

### Manual Testing with Discord

Discord provides a straightforward development setup:

1. Create a test application in the Developer Portal
2. Create a test guild (Discord server)
3. Install the bot with guild-specific commands (`guild_id` config)
4. Guild-scoped commands update instantly (no propagation delay)

The `discordgo` library does not include a test server, but the adapter's event handlers can be tested by constructing `discordgo.InteractionCreate` and `discordgo.MessageCreate` structs directly.

---

## Implementation Plan

### Phase 4a: Core Discord Adapter

- [ ] Add `github.com/bwmarrin/discordgo` dependency to `extras/fabric-chat-app/go.mod`
- [ ] Add `DiscordConfig` to `PlatformsConfig` in `config.go`
- [ ] Implement `internal/discord/adapter.go` — `Adapter` struct, Gateway connection, `Start()`/`Stop()`
- [ ] Implement `internal/discord/events.go` — event normalization for slash commands, messages, interactions
- [ ] Implement `internal/discord/commands.go` — application command registration, argument extraction
- [ ] Implement `internal/discord/embeds.go` — `Card` → Discord Embed rendering for all widget types
- [ ] Implement `internal/discord/components.go` — button and select menu rendering
- [ ] Implement `internal/discord/modals.go` — `Dialog` → Discord Modal rendering + value extraction
- [ ] Wire Discord adapter in `main.go`
- [ ] Add `"discord"` case to mention formatting in `notifications.go`
- [ ] Unit tests for embed rendering, component rendering, modal rendering, and event normalization

### Phase 4b: Agent Identity & Threading

- [ ] Implement `internal/discord/webhooks.go` — per-channel webhook management
- [ ] Implement per-agent username/avatar via webhook execution
- [ ] Implement `IconProvider` abstraction with `robohashProvider` (shared with Slack adapter)
- [ ] Thread creation from notification messages
- [ ] Thread ID round-trip through `SendMessageRequest` → `ChatEvent`
- [ ] Broadcast reply support via per-channel policy (reuse `space_settings` table)

### Phase 4c: Interactions & Polish

- [ ] Implement `internal/discord/verify.go` — Ed25519 signature verification for HTTP interactions endpoint
- [ ] Optional HTTP Interactions Endpoint alongside Gateway
- [ ] Ephemeral vs. public response routing for slash commands
- [ ] Deferred interaction responses for long-running commands
- [ ] User profile cache with TTL
- [ ] Optional @mention message routing (with MESSAGE_CONTENT intent)
- [ ] Command cleanup on shutdown (dev mode)
- [ ] End-to-end testing in Discord test server

---

## Resolved Decisions

1. **SDK choice:** `github.com/bwmarrin/discordgo` — the most widely used Go Discord library, actively maintained, covers Gateway, REST API, interactions, and all message component types. Alternative `github.com/diamondburned/arikawa/v3` was considered for its stronger typing but rejected for its smaller community and less adoption in production.

2. **Event delivery: Gateway (WebSocket) as primary.** Discord's Gateway is the natural fit for a long-running process. Unlike Slack's Events API (HTTP) vs. Socket Mode (WSS) dichotomy, Discord's Gateway handles all event types. HTTP Interactions Endpoint is optional.

3. **Per-agent identity: Channel webhooks.** Discord bots have fixed identity. Channel webhooks allow custom `username` and `avatar_url` per message, achieving the same effect as Slack's `chat:write.customize`. One webhook per channel is created lazily and cached.

4. **User email: Not available; device auth is the only registration path.** Discord's bot API does not expose user email. Auto-registration by email is not possible, and no OAuth2 workaround will be implemented. Users run `/fabric register`, receive a device code and Hub URL, complete the flow in the browser, and the Discord–Hub mapping is created. This is the sole registration path.

5. **Modal limitations: Two-phase approach.** Discord modals only support text inputs. Select/checkbox dialog fields are rendered as message components (select menus, buttons) outside the modal. For the chat app's current dialog flows, this is not a significant limitation.

6. **Slash command registration: Global by default, guild-scoped for dev.** Global commands propagate within an hour; guild commands are instant. The `guild_id` config field controls this.

7. **Thread model: Thread channels from notification messages.** Discord threads are full channels with their own IDs, unlike Slack's timestamp-based threading. Threads are created from notification card messages with 24-hour auto-archive.

8. **@Mention routing: Optional, disabled by default.** Parsing @mention messages requires the MESSAGE_CONTENT privileged intent. Since slash commands handle all commands natively, @mention routing is an optional enhancement. This avoids requiring a privileged intent for basic operation.

9. **Guild vs. channel linking: Channel-level.** Consistent with Slack's channel-level linking. A guild can have multiple channels linked to different groves.

13. **Authorization: Hub-based permissions only.** The Hub is the authoritative source for who can perform agent operations. The only Discord-native check is `MANAGE_GUILD` for channel-level admin commands (`link`, `unlink`, `broadcast`). No Discord role mapping is implemented.

10. **Agent icon generation:** Reuses the `IconProvider` abstraction from the Slack adapter with `robohashProvider` default, using 128x128 images (Discord's recommended minimum for webhook avatars).

11. **Ephemeral messages: Native support.** Discord's `MessageFlagsEphemeral` maps directly to the visibility requirements for `help`, `info`, `register`, and `unregister` commands. No workaround needed (unlike Google Chat which has no ephemeral message concept).

12. **Interaction response timing: Deferred response pattern.** Like Slack's 3-second requirement, Discord requires interaction acknowledgment within 3 seconds. The adapter defers with type 5 (shows "thinking..." indicator) and sends follow-up messages asynchronously after command processing.

## Open Questions

_None at this time._
