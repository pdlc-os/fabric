# Issue: Auth Dialog Appears Despite Valid Credentials

## Description
The authentication dialog is appearing unexpectedly during startup, even when the `GEMINI_API_KEY` environment variable is correctly set and an authentication method is already selected in the settings.

## Symptoms
- User has `GEMINI_API_KEY` set in the environment.
- `security.auth.selectedType` is set to `gemini-api-key` (or equivalent).
- On launch, the interactive "How would you like to authenticate?" dialog is shown instead of proceeding directly to the application.

## Potential Causes
1.  **Initializer Logic:** `packages/cli/src/core/initializer.ts` might be incorrectly evaluating `shouldOpenAuthDialog`.
    ```typescript
    const shouldOpenAuthDialog =
      settings.merged.security?.auth?.selectedType === undefined || !!authError;
    ```
    If `authError` is being set during a non-interactive validation step, it might be triggering the dialog.

2.  **Hook State:** `packages/cli/src/ui/auth/useAuth.ts` might be defaulting to `AuthState.Updating` or `AuthState.Unauthenticated` in a way that triggers the UI.

3.  **Environment Variable Propagation:** In the context of `fabric`, verify that `GEMINI_API_KEY` is being correctly passed into the container environment.

4.  **Enforced Auth Type Mismatch:** If `security.auth.enforcedType` is set and doesn't match the effective auth type, it may be triggering a re-authentication prompt.

## Proposed Investigation Steps
- Check `packages/cli/src/validateNonInterActiveAuth.ts` to see how it handles environment variables vs settings.
- Debug `performInitialAuth` in `packages/cli/src/core/auth.ts`.
- Verify the state transitions in `useAuthCommand` hook.
- Review `packages/cli/src/gemini.tsx` startup sequence, specifically the logic around line 345 which sets default auth types.

## Related Files
- `packages/cli/src/core/initializer.ts`
- `packages/cli/src/ui/auth/useAuth.ts`
- `packages/cli/src/ui/auth/AuthDialog.tsx`
- `packages/cli/src/validateNonInterActiveAuth.ts`
- `fabric/pkg/config/auth.go`
