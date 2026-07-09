# Web Authentication (OAuth)

Web authentication uses standard OAuth 2.0 authorization code flow with session cookies.

## Flow Diagram

```
┌─────────┐     ┌─────────────┐     ┌──────────────┐     ┌─────────┐
│ Browser │────►│Web Frontend │────►│OAuth Provider│────►│ Hub API │
│         │     │   :9820     │     │(Google/GitHub)│    │ :9810   │
└─────────┘     └─────────────┘     └──────────────┘     └─────────┘
     │                │                    │                  │
     │  1. GET /auth/login/google          │                  │
     │───────────────►│                    │                  │
     │                │  2. Redirect to OAuth                 │
     │◄───────────────│───────────────────►│                  │
     │  3. User authorizes                 │                  │
     │◄───────────────────────────────────►│                  │
     │                │  4. Callback with code                │
     │───────────────►│◄───────────────────│                  │
     │                │  5. Exchange code for tokens          │
     │                │───────────────────►│                  │
     │                │◄───────────────────│                  │
     │                │  6. Create/lookup user                │
     │                │────────────────────────────────────►│
     │                │◄────────────────────────────────────│
     │                │  7. Issue session token               │
     │                │────────────────────────────────────►│
     │                │◄────────────────────────────────────│
     │  8. Set session cookie                                 │
     │◄───────────────│                    │                  │
     │  9. Redirect to app                 │                  │
     │◄───────────────│                    │                  │
```

## Session Management

Web sessions use HTTP-only cookies with the following properties:

```typescript
const sessionConfig = {
  name: 'fabric:sess',
  maxAge: 24 * 60 * 60 * 1000,  // 24 hours
  httpOnly: true,
  secure: true,                  // HTTPS only in production
  sameSite: 'lax',
  signed: true
};
```

## Hub API Endpoints

```
POST /api/v1/auth/login
  Request:  { provider, email, name, avatar, providerToken }
  Response: { user, accessToken, refreshToken }

POST /api/v1/auth/refresh
  Request:  { refreshToken }
  Response: { accessToken, refreshToken }

POST /api/v1/auth/logout
  Request:  { refreshToken? }
  Response: { success: true }

GET /api/v1/auth/me
  Response: { user }
```

---

## Related Documents

- [Auth Overview](auth-overview.md) - Identity model and token types
- [CLI Authentication](cli-auth.md) - Terminal-based authentication
- [Server Authentication](server-auth-design.md) - Hub server-side auth handling
- [Server Auth Setup](server-auth-setup.md) - API keys and security considerations
