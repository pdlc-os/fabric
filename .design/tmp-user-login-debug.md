# Debug: User Token Invalidation After Server Restart

**Status:** Active investigation
**Symptom:** After every server restart, previously signed-in users get `invalid access token: failed to verify token: go-jose/go-jose: error in cryptographic primitive`. Signing out and signing back in resolves it until the next restart.

## Background

The hub server generates two HS256 signing keys on first run:
- `agent_signing_key` — signs agent JWTs (10h default)
- `user_signing_key` — signs user access/refresh JWTs (15m / 7d)

These are persisted to the `secrets` table in SQLite and loaded on subsequent starts via `ensureSigningKey()` in `pkg/hub/server.go`.

## Fixed Issues (confirmed)

### 1. PK collision during scope-ID migration (commit 7326f0c3)

Commit 65739619 changed hub-scoped secrets from a hardcoded `scope_id="hub"` to a per-instance hub ID (SHA-256 of hostname). The migration in `ensureSigningKey()` tried to re-save old keys under the new scope_id but used the same primary key ID (`"hub-<keyName>"`), causing a UNIQUE constraint violation on INSERT. The migration silently failed every restart, leaving keys stranded under the old scope_id.

**Fix:** Delete the old record before inserting the new one, and include the hub ID in the record's primary key (`"hub-<hubID>-<keyName>"`).

### 2. Keys not appearing in hub secret list

Because the migration silently failed, no record existed under the current `scope_id`. The admin UI (`/settings`) and CLI (`fabric hub secret list --scope hub`) query by `scope_id`, so the signing keys were invisible.

**Fix:** Same as above — successful migration makes them listable.

## Remaining Issue: Token invalidation on every restart

Server logs show `"Loaded existing signing key from store"` for both keys on restart — NOT `"Persisted new signing key"`. This means the keys are being found in the database. Yet tokens signed during the previous run fail signature verification.

### Hypotheses investigated

#### H1: Signing key is different across restarts (DISPROVED by test)

`TestServer_UserTokenSurvivesRestart` creates a server, generates a user JWT, closes the SQLite file, reopens it, creates a new server, and validates the old JWT. The test passes with file-backed SQLite. Key fingerprints match across the restart.

#### H2: Hostname-derived hub ID is unstable (RULED OUT)

The server runs on a GCE VM (`fabric-gteam`) with a stable hostname. `DefaultHubID()` uses `sha256(hostname)[:12]`, which is deterministic. Logs confirm the same hub ID (`a827a6646d3e`) across restarts.

#### H3: `base64.StdEncoding.DecodeString` silently fails after logging

`ensureSigningKey()` logs `"Loaded existing signing key from store"` before returning `base64.StdEncoding.DecodeString(val)`. If the decode returns an error, the caller's `if err == nil` check fails, and `cfg.UserTokenConfig.SigningKey` stays nil. Then `NewUserTokenService` generates a random ephemeral key (line 97-103 of usertoken.go) with no error logged.

**Status:** Plausible but unlikely — the value was encoded with `EncodeToString` so it should be valid base64. Diagnostic logging now surfaces this case.

#### H4: Session store (`/tmp/fabric-sessions/`) is losing data

The web frontend stores JWTs in server-side filesystem sessions (gorilla/sessions `FilesystemStore`), not in browser localStorage. If session files were lost between restarts (e.g., `PrivateTmp=yes` in systemd), the JWT would be gone and the middleware would pass through without a Bearer header.

**Status:** Ruled out. The systemd unit does NOT use `PrivateTmp`. Logs confirm a real JWT is being sent (`auth_prefix: "Bearer eyJhbGci..."`), proving the session file is readable.

#### H5: Stale session files contain JWTs signed with old ephemeral keys

Between the scope-ID refactor (65739619) and the fix (5ec16de2), the server generated a new ephemeral signing key on every restart (the key couldn't be persisted due to the PK collision). Session files from that period would contain JWTs signed with ephemeral keys that no longer exist.

**Status:** This explains a one-time failure after deploying the fix, but the user reports it happens on EVERY restart — even after logging out and back in (which creates a fresh JWT with the current key).

#### H6: Multiple signing key records in the database

If the database has records for the same key name under different scope_ids (e.g., `""`, `"hub"`, and `"a827a6646d3e"`), different code paths might load different values.

**Status:** Unlikely — `GetSecretValue` queries by the unique index `(key, scope, scope_id)` with the current hub ID. Only one record can match.

#### H7: GCP Secret Manager interference

The server has `FABRIC_SERVER_SECRETS_BACKEND=gcpsm`, but `ensureSigningKey()` runs during `New()` when `s.secretBackend` is nil (it's set later via `SetSecretBackend`). Signing keys always go through direct SQLite storage, never through GCP SM.

**Status:** Ruled out.

## Diagnostic Logging (commit 9923237a)

Added three log points to narrow the root cause:

### 1. Key fingerprint on init
```
"User token service initialized" key_fingerprint=<first-8-bytes-of-sha256>
```
Logged in `New()` after `NewUserTokenService` creates the signer. Shows the actual key bytes being used for signing/verification.

### 2. Key fingerprint on verification failure
```
"Token verification failed" key_fingerprint=<hex> key_len=<n>
```
Logged in `ValidateUserToken()` when `token.Claims()` returns an error. Allows comparing the verification key with the init key.

### 3. Ephemeral key warning
```
"Failed to load user signing key, will use ephemeral key" error=<err>
```
Logged in `New()` when `ensureSigningKey` returns an error. Previously this was silently swallowed, causing `NewUserTokenService` to generate a random key with no indication.

## How to interpret the diagnostic output

Deploy the build, restart the server twice, and compare:

| Scenario | Init fingerprint (run N) | Init fingerprint (run N+1) | Failure fingerprint | Diagnosis |
|----------|-------------------------|---------------------------|--------------------|----|
| All three match | `abc123` | `abc123` | `abc123` | Key is stable. Problem is elsewhere (session data, token encoding). |
| Init differs between runs | `abc123` | `def456` | `def456` | Key is changing. Check for `"Failed to load user signing key"` warning — indicates base64 decode or DB read failure. |
| Init matches but failure differs | `abc123` | `abc123` | `def456` | Two UserTokenService instances with different keys. Check for re-initialization. |
| No failure fingerprint logged | — | — | (missing) | The failure isn't in `ValidateUserToken` — check if it's hitting a different auth path (UAT, agent token, etc.). |

## Fix: Server Bootstrap Secrets (2026-03-31)

### Root cause

The `ensureSigningKey()` function in `hub.New()` checks the secret backend (GCP SM) first, then falls back to SQLite. However, `SetSecretBackend()` was called **after** `hub.New()`, so `s.secretBackend` was always `nil` during signing key initialization. This meant:

1. Signing keys always came from SQLite, never from the intended production secret backend
2. Before the scope-ID migration fix, SQLite lookups failed silently, causing ephemeral keys
3. The old binary had no warning log for this case — ephemeral keys were generated silently
4. Each restart generated a new random ephemeral key, invalidating all previous tokens

### Fix applied

1. **Moved secret backend initialization before `hub.New()`** in `cmd/server_foreground.go`
2. **Added `SecretBackend` field to `ServerConfig`** so it's available during `ensureSigningKey()`
3. **Added SQLite-to-backend sync**: when a key is found in SQLite but the backend is configured, the key is synced to the backend for future restarts
4. **Added `logSigningKeyFailure()`**: logs at ERROR level (not WARN) when a secret backend is configured but key loading fails
5. **Added fingerprint logging for agent token service** (was only on user tokens)

### Expected behavior after fix

- First restart after deploying: `"Loaded existing signing key from store"` + `"Synced signing key from store to secret backend"`
- Subsequent restarts: `"Loaded existing signing key from secret backend"`
- Both agent and user key fingerprints logged at INFO level on every startup
- One-time token invalidation expected for tokens signed by previous builds' ephemeral keys; users need to log in once after deploying the fix
