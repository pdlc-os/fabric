# Google Chat App to Workspace Add-on Migration Plan

## Objective
Migrate the `@extras/fabric-chat-app` Google Chat adapter from a standard webhook-based Chat App architecture to an HTTP Service-based Workspace Add-on architecture focused on chat integration.

## Motivation
Workspace Add-ons offer a more integrated experience across Google Workspace and use a standardized, richer event object (`EventObject`). This migration involves updating the payload parsing, synchronous response handling for interactive elements (like Dialogs), and action URLs.

## Key Changes & Action Items

### 1. Payload and Request Parsing

* **Current state**: The `googlechat/adapter.go` uses `rawEvent` to parse a flat Chat API event with a top-level `type` field (e.g., `ADDED_TO_SPACE`, `MESSAGE`, `CARD_CLICKED`) and flat fields (`event.message`, `event.space`, `event.user`, `event.action`).
* **Target state**: Workspace Add-ons use a nested `EventObject` with no top-level `type` field. The payload is divided into `commonEventObject` (platform context, form inputs, action parameters) and `chat` (Chat-specific payloads). The event type must be inferred from which payload field is present inside `chat`.

#### Raw Event Struct Rewrite

Replace the flat `rawEvent` struct with a nested structure matching the Add-on envelope:

```go
type rawEvent struct {
    CommonEventObject *rawCommonEventObject `json:"commonEventObject"`
    Chat              *rawChatPayload       `json:"chat"`
}

type rawCommonEventObject struct {
    Platform   string                         `json:"platform"`   // WEB, IOS, ANDROID
    HostApp    string                         `json:"hostApp"`
    UserLocale string                         `json:"userLocale"`
    TimeZone   *rawTimeZone                   `json:"timeZone"`
    Parameters map[string]string              `json:"parameters"` // Action params (replaces action.parameters)
    FormInputs map[string]rawFormInputWrapper `json:"formInputs"` // Dialog/widget inputs
}

type rawFormInputWrapper struct {
    StringInputs *rawStringInputs `json:"stringInputs"`
    DateTimeInput *rawDateTimeInput `json:"dateTimeInput"`
    // other input types as needed
}

type rawStringInputs struct {
    Value []string `json:"value"` // Always an array, even for single values
}

type rawChatPayload struct {
    User                    *rawUser                    `json:"user"`
    Space                   *rawSpace                   `json:"space"`
    EventTime               string                      `json:"eventTime"`
    MessagePayload          *rawMessagePayload          `json:"messagePayload"`
    AddedToSpacePayload     *rawAddedToSpacePayload     `json:"addedToSpacePayload"`
    RemovedFromSpacePayload *rawRemovedFromSpacePayload `json:"removedFromSpacePayload"`
    ButtonClickedPayload    *rawButtonClickedPayload    `json:"buttonClickedPayload"`
    AppCommandPayload       *rawAppCommandPayload       `json:"appCommandPayload"`
    WidgetUpdatedPayload    *rawWidgetUpdatedPayload    `json:"widgetUpdatedPayload"`
}
```

#### Payload-specific structs (each carries its own `space`, `message`, etc.):

```go
type rawMessagePayload struct {
    Message                  *rawMessage `json:"message"`
    Space                    *rawSpace   `json:"space"`
    ConfigCompleteRedirectUri string     `json:"configCompleteRedirectUri"`
}

type rawAddedToSpacePayload struct {
    Space                    *rawSpace `json:"space"`
    InteractionAdd           bool      `json:"interactionAdd"` // true if added via @mention
    ConfigCompleteRedirectUri string   `json:"configCompleteRedirectUri"`
}

type rawRemovedFromSpacePayload struct {
    Space *rawSpace `json:"space"`
}

type rawButtonClickedPayload struct {
    Message         *rawMessage `json:"message"`
    Space           *rawSpace   `json:"space"`
    IsDialogEvent   bool        `json:"isDialogEvent"`
    DialogEventType string      `json:"dialogEventType"` // REQUEST_DIALOG, SUBMIT_DIALOG
}

type rawAppCommandPayload struct {
    AppCommandMetadata *rawAppCommandMetadata `json:"appCommandMetadata"`
    Space              *rawSpace              `json:"space"`
    Thread             *rawThread             `json:"thread"`
    Message            *rawMessage            `json:"message"`
    IsDialogEvent      bool                   `json:"isDialogEvent"`
    DialogEventType    string                 `json:"dialogEventType"`
}

type rawAppCommandMetadata struct {
    AppCommandId   json.Number `json:"appCommandId"`
    AppCommandType string      `json:"appCommandType"` // SLASH_COMMAND, QUICK_COMMAND
}
```

#### Event Type Detection in `normalizeEvent`

Since there is no top-level `type` field, detection must be based on which payload is non-nil:

| Payload present | Old `type` equivalent | ChatEventType |
|---|---|---|
| `chat.addedToSpacePayload` | `ADDED_TO_SPACE` | `EventSpaceJoin` |
| `chat.removedFromSpacePayload` | `REMOVED_FROM_SPACE` | `EventSpaceRemove` |
| `chat.appCommandPayload` | `MESSAGE` (with `slashCommand`) | `EventCommand` |
| `chat.messagePayload` | `MESSAGE` | `EventMessage` |
| `chat.buttonClickedPayload` (no formInputs) | `CARD_CLICKED` (action) | `EventAction` |
| `chat.buttonClickedPayload` (with formInputs) | `CARD_CLICKED` (dialog) | `EventDialogSubmit` |

#### Field Access Changes

| Old path | New path |
|---|---|
| `event.Type` | Inferred from payload presence |
| `event.Space` | `event.Chat.<payload>.Space` (each payload has its own) |
| `event.Message` | `event.Chat.<payload>.Message` |
| `event.User` | `event.Chat.User` (top-level in chat object) |
| `event.Action.ActionMethodName` | `event.CommonEventObject.Parameters["__action_method_name__"]` (legacy compat) or dropped if using full URLs |
| `event.Action.Parameters` | `event.CommonEventObject.Parameters` |
| `event.Common.FormInputs[key]` | `event.CommonEventObject.FormInputs[key].StringInputs.Value` (array!) |
| `event.Message.SlashCommand` | `event.Chat.AppCommandPayload.AppCommandMetadata` |

#### formInputs Value Format Change

**Critical**: The current code reads `formInputs[key]` as a single string value. In Add-ons, `formInputs` values are **arrays of strings** accessed via `formInputs[widgetId].stringInputs.value[]`. The `normalizeEvent` function must:
- Access `Value[0]` for single-value inputs (text fields)
- Join or pass through `Value[]` for multi-value inputs (checkboxes/multi-selects)
- This affects `handleDialogSubmit` and subscription filter parsing

#### Multi-Turn Event Handling (New)

When added to a space via @mention, Add-ons receive **two sequential requests**:
1. `addedToSpacePayload` with `interactionAdd: true`
2. `messagePayload` with the actual message text

Similarly, when added via slash command:
1. `addedToSpacePayload`
2. `appCommandPayload`

**Action Items**:
* The `handleSpaceJoin` handler must check `interactionAdd` to distinguish direct-add vs @-mention-add
* If `interactionAdd` is true, the handler should complete silently (or send a minimal ack), letting the subsequent `messagePayload`/`appCommandPayload` request drive the real response — this avoids duplicate welcome messages
* Consider using state/deduplication (e.g. a short-lived in-memory set keyed on spaceID) to prevent race conditions if both requests arrive concurrently

### 2. Request Verification

* **Current state**: Standard apps verify requests using the Chat API audience (Project Number) and `chat@system.gserviceaccount.com`.
* **Target state**: Workspace Add-ons verify requests using a **unique per-project service account email** found in the Chat API configuration page in Cloud Console.

**Action Items**:
* Update the `GoogleChatConfig` struct to add a `ServiceAccountEmail` field (the per-project service account email from Cloud Console)
* If/when request validation middleware is added, verify the bearer token against this email rather than `chat@system.gserviceaccount.com`
* The **audience** for token verification also changes: from the project number to the HTTP endpoint URL
* Add `EndpointURL` to `GoogleChatConfig` for use in both token audience verification and card action URLs (see §4)

### 3. Synchronous Responses (RenderActions)

* **Current state**: The adapter's `handleEvent` always returns `200 OK` with an empty body. All outbound messages and cards are sent asynchronously via the Chat REST API (`SendMessage`, `doPost`, `doPatch`). Dialog opening (`OpenDialog`) is a stub.
* **Target state**: The Chat REST API is still used for asynchronous message creation, but **interactive features require synchronous JSON responses**.

#### Response Format: Creating Messages Synchronously

For handlers that need to reply inline (e.g. slash command responses), the HTTP response body must use the `hostAppDataAction` wrapper:

```json
{
  "hostAppDataAction": {
    "chatDataAction": {
      "createMessageAction": {
        "message": {
          "text": "Hello!",
          "cardsV2": [{"cardId": "abc", "card": {...}}]
        }
      }
    }
  }
}
```

For updating the message that triggered the event (e.g. replacing a card after button click):
```json
{
  "hostAppDataAction": {
    "chatDataAction": {
      "updateMessageAction": {
        "message": {
          "text": "Updated",
          "cardsV2": [...]
        }
      }
    }
  }
}
```

#### Response Format: Dialogs (RenderActions)

**Opening a dialog** (when a button with `interaction: OPEN_DIALOG` is clicked):
```json
{
  "action": {
    "navigations": [
      {
        "pushCard": {
          "header": {"title": "Dialog Title"},
          "sections": [{"widgets": [...]}],
          "fixedFooter": {
            "primaryButton": {"text": "Submit", "onClick": {"action": {"function": "<endpoint-url>"}}},
            "secondaryButton": {"text": "Cancel", "onClick": {"action": {"function": "<endpoint-url>", "parameters": [{"key": "cancel", "value": "true"}]}}}
          }
        }
      }
    ]
  }
}
```

**Closing a dialog** (after successful submission):
```json
{
  "action": {
    "navigations": [{"endNavigation": "CLOSE_DIALOG"}],
    "notification": {"text": "Done!"}
  }
}
```

#### Action Items

* Modify `handleEvent` to support returning a JSON response body, not just `200 OK`. The handler return type should become a response struct that `handleEvent` serializes.
* Implement `OpenDialog` by returning a `pushCard` navigation response. This replaces the current stub.
* Implement dialog close by returning `endNavigation: CLOSE_DIALOG`.
* Continue using async REST API for: notifications from the broker, messages to spaces not triggered by an incoming request, and any follow-up messages after the synchronous response.
* The `Messenger` interface may need refactoring: `OpenDialog` and `UpdateDialog` don't fit the async pattern — they must produce the HTTP response, not make a separate API call. Consider having `handleEvent` return a response payload, with the command router returning a "dialog response" variant.

### 4. Interactive Components and Action URLs

* **Current state**: `renderWidget` generates cards with `onClick.action.function` containing a simple string action ID (e.g., `agent.start.123`).
* **Target state**: For HTTP Workspace Add-ons, `action.function` must be a **full HTTP URL** of the endpoint that handles the interaction.

#### Action Items

* Add an `ExternalURL` field to `GoogleChatConfig` (the publicly reachable URL of the Chat App's HTTP endpoint).
* Update `renderCardV2` and `renderWidget` to set `onClick.action.function` to the full URL (e.g., `https://fabric-chat-app-xyz.run.app`).
* Move the current action ID strings into `action.parameters`:
  ```json
  {
    "onClick": {
      "action": {
        "function": "https://fabric-chat-app-xyz.run.app",
        "parameters": [
          {"key": "action", "value": "agent.start.agentId123"}
        ]
      }
    }
  }
  ```
* In `normalizeEvent`, read action IDs from `commonEventObject.parameters["action"]` instead of `action.actionMethodName`.
* For dialog-opening buttons, add `"interaction": "OPEN_DIALOG"` to the action.

#### Backward Compatibility During Transition

* Configure the "Card interaction URL" in Cloud Console to point to the same endpoint
* Pre-conversion cards sent before migration will pass the old function name as `commonEventObject.parameters["__action_method_name__"]`
* `normalizeEvent` should fall back to checking `parameters["__action_method_name__"]` if `parameters["action"]` is absent, to handle clicks on old cards during transition

### 5. Slash Commands → App Commands

* **Current state**: Slash commands arrive as `MESSAGE` events with a `slashCommand` field on the message.
* **Target state**: Slash commands arrive as a separate `appCommandPayload` with `appCommandMetadata` containing `appCommandId` and `appCommandType`.

#### Action Items

* Update `normalizeEvent` to detect `chat.appCommandPayload` and map it to `EventCommand`
* Extract the command from `appCommandMetadata.appCommandId` — this is a numeric ID, not the command name. The router needs a mapping from command IDs to command names (configured in Cloud Console).
* Add a `CommandIDMap map[string]string` to `GoogleChatConfig` (e.g., `{"1": "fabric"}`) to translate numeric IDs to command names.
* Extract the message text from `appCommandPayload.message.argumentText` for the command arguments.

### 6. Deployment and Configuration

#### Cloud Console Configuration

* Navigate to the Chat API "Interactive Features" section
* Choose HTTP Service as the app type
* Provide the endpoint URL (Cloud Run service URL)
* Configure slash commands — note that command IDs are assigned by the console
* Set the "Card interaction URL" for backward compatibility (same endpoint URL)

#### Config File Changes

Add new fields to `GoogleChatConfig`:

```go
type GoogleChatConfig struct {
    Enabled             bool              `yaml:"enabled"`
    ProjectID           string            `yaml:"project_id"`
    Credentials         string            `yaml:"credentials"`
    ListenAddress       string            `yaml:"listen_address"`
    ExternalURL         string            `yaml:"external_url"`          // Public endpoint URL
    ServiceAccountEmail string            `yaml:"service_account_email"` // Per-project SA for verification
    CommandIDMap        map[string]string  `yaml:"command_id_map"`        // Console command ID → name
    // Audience field removed — ExternalURL serves as audience
}
```

#### Testing & Cutover

* Test the new Add-on payload structure thoroughly in a staging environment
* Deploy and verify all event types work: messages, slash commands, button clicks, dialog open/close, space join/remove
* Verify notification cards with full-URL actions render and function correctly
* Verify the dual-request pattern on @-mention-add works without duplicate responses
* Execute the **irreversible** "Convert to add-on" action in Cloud Console to migrate all users

### 7. Summary of Files to Modify

| File | Changes |
|---|---|
| `internal/googlechat/adapter.go` | Rewrite `rawEvent` structs, rewrite `normalizeEvent`, update `handleEvent` to return response payloads, update `renderCardV2`/`renderWidget` for full URLs and action parameters, implement dialog open/close responses |
| `internal/chatapp/config.go` | Add `ExternalURL`, `ServiceAccountEmail`, `CommandIDMap` to `GoogleChatConfig`; remove `Audience` |
| `internal/chatapp/commands.go` | Update action ID parsing to read from `parameters["action"]`, handle `__action_method_name__` fallback |
| `internal/chatapp/events.go` | No changes expected (ChatEvent abstraction remains valid) |
| `internal/chatapp/messenger.go` | Consider whether `OpenDialog`/`UpdateDialog` should move to a synchronous response mechanism rather than async API calls |
| `internal/chatapp/notifications.go` | Update rendered cards to use full URLs for action buttons |
