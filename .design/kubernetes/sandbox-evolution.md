# Sandbox Evolution & Strategy

## Status
Bringing back the **Agent Sandbox** is **not an intractable limitation**. It is a strategic trade-off made to accelerate Milestone 1 (M1). The decision to bypass `SandboxClaim` in favor of direct Pod management was driven by specific maturity gaps in the current Sandbox CRD implementation, not by fundamental architectural incompatibility.

## The Limitation (Why it was removed)
As detailed in `m1-design.md` and `flow.md`, the current `SandboxClaim` CRD lacks the mechanism to pass **per-instance overrides** effectively. The `Sandbox` model assumes configuration is defined statically in the `SandboxTemplate` ("The Class"), making the `SandboxClaim` ("The Request") too rigid for a CLI that requires Docker-like flexibility.

Specific gaps include:
1.  **Environment Variables:** Critical for injecting dynamic secrets like `GEMINI_API_KEY` or `GIT_TOKEN`.
2.  **Commands:** Essential for running different lifecycle tasks (e.g., `resume` vs `start`).
3.  **Images:** Needed for allowing users to choose runtime environments (e.g., `python:3.9` vs `node:18`) without platform admin intervention.

## Path to Restoration
Restoring the Sandbox architecture is feasible and desirable for the "Production Hardening" phase (Milestone 5). It requires adopting one of the following approaches:

### 1. Enhance the CRD (Preferred)
Update the `SandboxClaim` definition (or contribute upstream) to support a `spec.overrides` field. This would allow safely injecting `env` and `command` into the generated Pod while retaining the sandbox's policy enforcement.

### 2. Dynamic Templates
Refactor the CLI to generate a temporary, unique `SandboxTemplate` for each run (e.g., `fabric-agent-template-<uuid>`) containing the specific config, and then link the Claim to that template. This is more "chatty" for the API server but works with the existing CRD structure.

### 3. Auxiliary Resources
If the CRD supports `envFrom`, the CLI could create a Kubernetes Secret containing the dynamic configuration first, then reference that Secret in the `SandboxTemplate` or `Claim`.

## Recommendation
**Proceed with Milestone 1 using Direct Pods.**

Focus on solving the immediate functional problems of "Identity," "Context Sync," and "SCM Integration" without being blocked by CRD limitations. Once the *logic* for these features is solid, refactor the `KubernetesRuntime` to wrap that logic back into a `SandboxClaim` (likely using the "Dynamic Template" or "Enhanced CRD" approach) to regain the security and lifecycle benefits in Milestone 5.
