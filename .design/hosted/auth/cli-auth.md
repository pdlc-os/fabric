# CLI Authentication

CLI authentication enables `fabric hub` commands to authenticate with a Hub server using a browser-based OAuth flow with localhost callback.

## Commands

```bash
# Check authentication status
fabric hub auth status

# Authenticate with Hub (opens browser)
fabric hub auth login [--hub-url <url>]

# Clear stored credentials
fabric hub auth logout
```

## Device Authorization Flow

The CLI uses OAuth 2.0 with a localhost redirect for systems with a browser:

```
┌──────────┐     ┌─────────────┐     ┌──────────────┐     ┌─────────┐
│   CLI    │     │  Localhost  │     │OAuth Provider│     │ Hub API │
│ Terminal │     │   :18271    │     │              │     │ :9810   │
└──────────┘     └─────────────┘     └──────────────┘     └─────────┘
     │                 │                    │                  │
     │  1. fabric hub auth login            │                  │
     │─────────────────┼───────────────────┼─────────────────►│
     │                 │                   │   2. Get auth URL │
     │◄────────────────┼───────────────────┼──────────────────│
     │  3. Start localhost server          │                  │
     │────────────────►│                   │                  │
     │  4. Open browser with auth URL      │                  │
     │─────────────────┼──────────────────►│                  │
     │                 │  5. User authorizes                  │
     │                 │◄─────────────────►│                  │
     │                 │  6. Redirect to localhost            │
     │                 │◄──────────────────│                  │
     │  7. Receive auth code               │                  │
     │◄────────────────│                   │                  │
     │  8. Exchange code for CLI token     │                  │
     │─────────────────┼───────────────────┼─────────────────►│
     │◄────────────────┼───────────────────┼──────────────────│
     │  9. Store credentials locally       │                  │
     │                 │                   │                  │
```

## Implementation Details

### Localhost Callback Server

```go
// pkg/hub/auth/localhost_server.go

const (
    CallbackPort = 18271  // Arbitrary high port for localhost callback
    CallbackPath = "/callback"
)

type LocalhostAuthServer struct {
    server     *http.Server
    codeChan   chan string
    errChan    chan error
    state      string
}

func (s *LocalhostAuthServer) Start(ctx context.Context) (string, error) {
    // Generate random state for CSRF protection
    s.state = generateRandomState()

    mux := http.NewServeMux()
    mux.HandleFunc(CallbackPath, s.handleCallback)

    s.server = &http.Server{
        Addr:    fmt.Sprintf("127.0.0.1:%d", CallbackPort),
        Handler: mux,
    }

    go s.server.ListenAndServe()

    return fmt.Sprintf("http://127.0.0.1:%d%s", CallbackPort, CallbackPath), nil
}

func (s *LocalhostAuthServer) handleCallback(w http.ResponseWriter, r *http.Request) {
    // Verify state matches
    if r.URL.Query().Get("state") != s.state {
        s.errChan <- fmt.Errorf("state mismatch")
        http.Error(w, "State mismatch", http.StatusBadRequest)
        return
    }

    code := r.URL.Query().Get("code")
    if code == "" {
        errMsg := r.URL.Query().Get("error_description")
        s.errChan <- fmt.Errorf("auth failed: %s", errMsg)
        http.Error(w, "Authentication failed", http.StatusBadRequest)
        return
    }

    // Send success page to browser
    w.Header().Set("Content-Type", "text/html")
    w.Write([]byte(authSuccessHTML))

    s.codeChan <- code
}

func (s *LocalhostAuthServer) WaitForCode(ctx context.Context) (string, error) {
    select {
    case code := <-s.codeChan:
        return code, nil
    case err := <-s.errChan:
        return "", err
    case <-ctx.Done():
        return "", ctx.Err()
    case <-time.After(5 * time.Minute):
        return "", fmt.Errorf("authentication timeout")
    }
}
```

### CLI Auth Command

```go
// cmd/hub_auth.go

var hubAuthCmd = &cobra.Command{
    Use:   "auth",
    Short: "Manage Hub authentication",
}

var hubAuthLoginCmd = &cobra.Command{
    Use:   "login",
    Short: "Authenticate with Hub server",
    RunE: func(cmd *cobra.Command, args []string) error {
        hubURL, _ := cmd.Flags().GetString("hub-url")
        if hubURL == "" {
            hubURL = config.DefaultHubURL()
        }

        // Start localhost callback server
        authServer := auth.NewLocalhostAuthServer()
        callbackURL, err := authServer.Start(cmd.Context())
        if err != nil {
            return fmt.Errorf("failed to start auth server: %w", err)
        }
        defer authServer.Shutdown()

        // Get OAuth URL from Hub
        client := hub.NewClient(hubURL)
        authURL, err := client.GetAuthURL(cmd.Context(), callbackURL)
        if err != nil {
            return fmt.Errorf("failed to get auth URL: %w", err)
        }

        // Open browser
        fmt.Println("Opening browser for authentication...")
        if err := openBrowser(authURL); err != nil {
            fmt.Printf("Please open this URL in your browser:\n%s\n", authURL)
        }

        // Wait for callback
        fmt.Println("Waiting for authentication...")
        code, err := authServer.WaitForCode(cmd.Context())
        if err != nil {
            return fmt.Errorf("authentication failed: %w", err)
        }

        // Exchange code for token
        token, err := client.ExchangeCode(cmd.Context(), code, callbackURL)
        if err != nil {
            return fmt.Errorf("failed to get token: %w", err)
        }

        // Store credentials
        if err := credentials.Store(hubURL, token); err != nil {
            return fmt.Errorf("failed to store credentials: %w", err)
        }

        fmt.Println("Authentication successful!")
        return nil
    },
}

var hubAuthStatusCmd = &cobra.Command{
    Use:   "status",
    Short: "Show authentication status",
    RunE: func(cmd *cobra.Command, args []string) error {
        hubURL := config.DefaultHubURL()

        creds, err := credentials.Load(hubURL)
        if err != nil {
            fmt.Println("Not authenticated")
            return nil
        }

        // Verify token is still valid
        client := hub.NewClient(hubURL)
        client.SetToken(creds.AccessToken)

        user, err := client.GetCurrentUser(cmd.Context())
        if err != nil {
            fmt.Println("Authentication expired. Run 'fabric hub auth login' to re-authenticate.")
            return nil
        }

        fmt.Printf("Authenticated as: %s (%s)\n", user.DisplayName, user.Email)
        fmt.Printf("Hub: %s\n", hubURL)
        return nil
    },
}

var hubAuthLogoutCmd = &cobra.Command{
    Use:   "logout",
    Short: "Clear stored credentials",
    RunE: func(cmd *cobra.Command, args []string) error {
        hubURL := config.DefaultHubURL()

        if err := credentials.Remove(hubURL); err != nil {
            return fmt.Errorf("failed to remove credentials: %w", err)
        }

        fmt.Println("Logged out successfully.")
        return nil
    },
}
```

## Credential Storage

CLI credentials are stored in `~/.fabric/credentials.json`:

```json
{
  "version": 1,
  "hubs": {
    "https://hub.example.com": {
      "accessToken": "eyJ...",
      "refreshToken": "eyJ...",
      "expiresAt": "2025-02-01T12:00:00Z",
      "user": {
        "id": "user-uuid",
        "email": "user@example.com",
        "displayName": "User Name"
      }
    }
  }
}
```

**Security Considerations:**
- File permissions set to `0600` (owner read/write only)
- Tokens are not encrypted at rest (relies on filesystem permissions)
- Refresh tokens enable automatic token renewal

```go
// pkg/credentials/store.go

const (
    CredentialsFile = "credentials.json"
    FileMode        = 0600
)

type Credentials struct {
    Version int                        `json:"version"`
    Hubs    map[string]*HubCredentials `json:"hubs"`
}

type HubCredentials struct {
    AccessToken  string    `json:"accessToken"`
    RefreshToken string    `json:"refreshToken"`
    ExpiresAt    time.Time `json:"expiresAt"`
    User         *User     `json:"user"`
}

func Store(hubURL string, token *TokenResponse) error {
    path := filepath.Join(config.FabricDir(), CredentialsFile)

    creds, _ := load(path)
    if creds == nil {
        creds = &Credentials{Version: 1, Hubs: make(map[string]*HubCredentials)}
    }

    creds.Hubs[hubURL] = &HubCredentials{
        AccessToken:  token.AccessToken,
        RefreshToken: token.RefreshToken,
        ExpiresAt:    time.Now().Add(token.ExpiresIn),
        User:         token.User,
    }

    data, err := json.MarshalIndent(creds, "", "  ")
    if err != nil {
        return err
    }

    return os.WriteFile(path, data, FileMode)
}

func Load(hubURL string) (*HubCredentials, error) {
    path := filepath.Join(config.FabricDir(), CredentialsFile)
    creds, err := load(path)
    if err != nil {
        return nil, err
    }

    hubCreds, ok := creds.Hubs[hubURL]
    if !ok {
        return nil, ErrNotAuthenticated
    }

    // Check if token needs refresh
    if time.Now().After(hubCreds.ExpiresAt.Add(-5 * time.Minute)) {
        return refreshToken(hubURL, hubCreds)
    }

    return hubCreds, nil
}
```
# Future work

## Headless Authentication (postponed)


For systems without a browser (CI/CD, remote servers), support API key authentication:

```bash
# Set API key via environment variable
export FABRIC_API_KEY="sk_live_..."

# Or via config file
fabric hub auth set-key <api-key>
```

API keys are created via the web dashboard and stored in the same credentials file.

---

## Related Documents

- [Auth Overview](auth-overview.md) - Identity model and token types
- [Web Authentication](web-auth.md) - Browser-based OAuth flows
- [Server Authentication](server-auth-design.md) - Hub server-side auth handling
- [Server Auth Setup](server-auth-setup.md) - API keys and dev authentication
