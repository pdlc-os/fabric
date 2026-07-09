# Post-Mortem: GCE Metadata Server 502 on Starter-Hub VMs

**Date:** 2026-05-01  
**Status:** Resolved  
**Severity:** High — all agents on affected VMs lost GCP identity  
**Duration:** Hours (exact window unknown; discovered after symptoms were persistent)  
**Affected:** starter-hub GCE VMs running combo hub-broker mode with ~20 agents  

## Summary

After running successfully for several hours, all agents on a starter-hub VM simultaneously lost the ability to obtain GCP access tokens. The GCE metadata server appeared to return `502 Bad Gateway` with body `"token generation failed"` after exactly 30 seconds. The root cause was an iptables REDIRECT rule leaking from agent containers into the host's network namespace, redirecting the Hub's own GCP credential traffic into a circular dependency.

## Timeline

1. Hub starts on a GCE VM, initializes its GCP IAM credentials client using the real metadata server (`169.254.169.254`). The Google Cloud SDK caches the Hub's own access token (~1 hour TTL).

2. Agents are created over time. Because the Hub runs on localhost, `ResolveDockerNetworking()` returns `"host"`, and containers launch with `--network=host` (shared network namespace).

3. Each agent's metadata sidecar (`fabrictool init`) creates an iptables REDIRECT rule in the nat OUTPUT chain:
   ```
   iptables -t nat -A OUTPUT -d 169.254.169.254 -p tcp --dport 80 -j REDIRECT --to-port 18380
   ```
   Because the container shares the host's network namespace, this rule is written to the **host's** iptables table.

4. For ~55 minutes, everything works. The Hub's SDK uses its cached token and never hits the metadata server. Agents get tokens through the sidecar -> Hub -> IAM API chain.

5. The Hub's cached token expires. The SDK tries to refresh from `169.254.169.254`. The iptables rule redirects this to port 18380 (the sidecar). The sidecar calls the Hub's `/api/v1/agent/gcp-token` endpoint. The Hub calls IAM `GenerateAccessToken`, which needs the Hub's own credentials, which tries `169.254.169.254` again — circular dependency.

6. All token requests time out at 30 seconds (the sidecar's HTTP client timeout). Agents see `502 Bad Gateway` from what they believe is the metadata server.

## Root Cause

Three factors combined to create this failure:

**1. iptables rule created in host network namespace (the bug)**

`setupIPTablesRedirect()` in `pkg/fabrictool/metadata/iptables.go` creates a REDIRECT rule with no scope limiters — no interface restriction, no UID filter, no source IP constraint. When executed inside a `--network=host` container, the rule applies to ALL processes on the host, including the Hub.

**2. Host networking used for localhost Hub connectivity**

`ResolveDockerNetworking()` in `pkg/runtime/common.go` returns `"host"` whenever the Hub endpoint is `localhost`, `127.0.0.1`, or `::1`. This is intentional — Docker bridge networking can't reach the host's loopback — but it means containers share the host's network namespace.

**3. Delayed manifestation masked the cause**

The failure only appears ~55–60 minutes after agents start, when the Hub's own cached GCP token expires and needs refreshing. The lag between cause (iptables rule added) and effect (auth failure) made the issue appear unrelated to agent creation.

## Impact

- All agents on the affected VM lost GCP identity simultaneously
- The Hub's own GCP operations (IAM, GCS, Secret Manager) also failed
- Agents could not obtain access tokens or identity tokens
- New agent creation with GCP identity would also fail

## Resolution

**Immediate mitigation:** Remove the leaked iptables rule from the host:
```bash
sudo iptables -t nat -D OUTPUT -d 169.254.169.254/32 -p tcp -j REDIRECT --to-ports 18380
```
Metadata access restored immediately after rule removal.

**Code fix (commit `af759428`):**

| Change | File | Effect |
|--------|------|--------|
| Skip iptables in host network mode | `pkg/fabrictool/metadata/server.go` | Prevents the leak entirely; `GCE_METADATA_HOST` env var is sufficient |
| Shorten sidecar timeout 30s -> 10s | `pkg/fabrictool/metadata/server.go` | Fail fast instead of hanging for 30s |
| Wire in singleflight on sidecar | `pkg/fabrictool/metadata/server.go` | Collapse concurrent token requests into one Hub call |
| Hub-side token cache per SA | `pkg/hub/gcp_token_cache.go` | Avoid redundant IAM API calls across agents sharing the same SA |
| Rate limiter context cancellation | `pkg/hub/gcp_ratelimit.go` | Fix goroutine leak on shutdown |

## Detection Gap

This failure was difficult to detect because:

- The 502 response appeared to come from the GCE metadata server, but was actually from the sidecar
- The response body `"token generation failed"` is the sidecar's error string, not a GCE error — but this required reading source code to recognize
- Standard monitoring of the Hub process wouldn't flag metadata server health
- The ~1 hour delay between cause and effect obscured the correlation

## Delayed Resolution

An earlier investigation (2026-05-01) correctly identified the iptables leak hypothesis as the most likely cause and produced diagnostic commands to verify it. However, when those diagnostics were run on the affected host, the leaked iptables rules were reportedly not present, and the hypothesis was marked "not confirmed."

In hindsight, the earlier diagnostics were either run after the offending containers had already been restarted (which cleans up the rules on exit), run with insufficient permissions (the `iptables` commands require `sudo`), or the output was misinterpreted. The investigation then pivoted to secondary hypotheses (thundering herd, Hub-side caching) which, while real inefficiencies worth fixing, were not the root cause.

This added days to the resolution timeline. When a hypothesis fits the evidence as precisely as this one did (exact error string match, exact timeout match, clear code path), failing to reproduce it once should not be grounds for dismissal — it should prompt a second attempt under controlled conditions (e.g., starting a fresh agent while watching `iptables -t nat -L OUTPUT -n` in a loop).

## Lessons Learned

1. **Host networking shares more than connectivity.** `--network=host` shares the entire network namespace, including iptables rules. Any container modifying iptables in host mode modifies the host's rules. This is a known Docker footgun but easy to forget when the iptables setup happens in a utility library rather than the container entry point.

2. **Delayed-onset failures need explicit correlation.** When a failure manifests an hour after its cause, standard debugging (what changed recently?) points in the wrong direction. The iptables rule was created successfully with no errors, so there was no log entry to correlate.

3. **Error messages should be unique and traceable.** The sidecar returned a generic `"token generation failed"` that could be confused with a GCE metadata server error. Including something like `"fabric-metadata-sidecar: token generation failed"` would have made the source immediately obvious.

4. **Defense-in-depth can backfire.** The iptables redirect was defense-in-depth for tools that don't respect `GCE_METADATA_HOST`. In host network mode, this defense became the vulnerability. Defense-in-depth mechanisms need to be aware of their deployment context.

## Action Items

- [x] Skip iptables interception in host network mode
- [x] Shorten sidecar timeout for fail-fast
- [x] Wire in singleflight for concurrent token dedup
- [x] Add Hub-side token caching per service account
- [ ] Consider prefixing sidecar error messages with a fabric-specific identifier
- [ ] Add a startup health check that verifies the Hub can reach the real metadata server
- [ ] Document the `--network=host` + iptables interaction in the architecture docs

## Related

- [Investigation: GCP Auth Token Failure](../investigations/gcp-auth-fail.md) — the initial investigation that identified the hypothesis
- `pkg/fabrictool/metadata/iptables.go` — iptables rule creation
- `pkg/runtime/common.go:538` — `ResolveDockerNetworking()` host mode decision
- `pkg/hub/gcp_token_cache.go` — new Hub-side token cache
