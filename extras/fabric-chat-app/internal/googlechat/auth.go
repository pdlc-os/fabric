// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package googlechat

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const chatBotScope = "https://www.googleapis.com/auth/chat.bot"

// NewAuthenticatedClient creates an HTTP client authenticated for the Google
// Chat API. If credentialsFile is non-empty, the service account key at that
// path is used. Otherwise Application Default Credentials (ADC) are used,
// which on a GCE VM resolves to the instance's service account.
func NewAuthenticatedClient(ctx context.Context, credentialsFile string, log *slog.Logger) (*http.Client, error) {
	var creds *google.Credentials
	var err error

	if credentialsFile != "" {
		data, readErr := os.ReadFile(credentialsFile)
		if readErr != nil {
			return nil, fmt.Errorf("reading credentials file %s: %w", credentialsFile, readErr)
		}
		creds, err = google.CredentialsFromJSON(ctx, data, chatBotScope)
		if err != nil {
			return nil, fmt.Errorf("parsing credentials from %s: %w", credentialsFile, err)
		}
		log.Info("using service account key file for Chat API auth", "file", credentialsFile)
	} else {
		creds, err = google.FindDefaultCredentials(ctx, chatBotScope)
		if err != nil {
			return nil, fmt.Errorf("finding default credentials: %w", err)
		}
		log.Info("using Application Default Credentials (ADC) for Chat API auth")
	}

	return oauth2.NewClient(ctx, creds.TokenSource), nil
}

// PreflightAuth verifies that the configured credentials can obtain a valid
// access token for the Google Chat API. On failure it logs actionable
// remediation steps including gcloud commands that reference the given
// projectID.
func PreflightAuth(ctx context.Context, credentialsFile, projectID string, log *slog.Logger) error {
	var creds *google.Credentials
	var err error

	if credentialsFile != "" {
		data, readErr := os.ReadFile(credentialsFile)
		if readErr != nil {
			return fmt.Errorf("reading credentials file %s: %w", credentialsFile, readErr)
		}
		creds, err = google.CredentialsFromJSON(ctx, data, chatBotScope)
	} else {
		creds, err = google.FindDefaultCredentials(ctx, chatBotScope)
	}
	if err != nil {
		log.Error("Chat API credential preflight failed", "error", err)
		logRemediationSteps(credentialsFile, projectID, log)
		return fmt.Errorf("credential preflight: %w", err)
	}

	// Attempt to obtain a token to confirm the credentials are valid.
	tok, tokenErr := creds.TokenSource.Token()
	if tokenErr != nil {
		log.Error("Chat API token preflight failed — could not obtain access token", "error", tokenErr)
		logRemediationSteps(credentialsFile, projectID, log)
		return fmt.Errorf("token preflight: %w", tokenErr)
	}

	log.Info("Chat API token obtained", "token_expiry", tok.Expiry)

	// Make a lightweight API call to verify the token's scopes are sufficient.
	// A token can be valid but lack the chat.bot scope (common with GCE default
	// scopes), which only surfaces as a 403 at runtime.
	client := oauth2.NewClient(ctx, creds.TokenSource)
	resp, apiErr := client.Get("https://chat.googleapis.com/v1/spaces?pageSize=1")
	if apiErr != nil {
		log.Error("Chat API preflight request failed", "error", apiErr)
		logRemediationSteps(credentialsFile, projectID, log)
		return fmt.Errorf("API preflight: %w", apiErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		log.Error("Chat API preflight failed — insufficient scopes or permissions",
			"status", resp.StatusCode, "response", string(body))
		logRemediationSteps(credentialsFile, projectID, log)
		return fmt.Errorf("API preflight: Chat API returned 403 — check VM OAuth scopes and API enablement")
	}

	log.Info("Chat API credential preflight passed")
	return nil
}

// logRemediationSteps prints human-readable instructions for fixing Chat API
// credential issues.
func logRemediationSteps(credentialsFile, projectID string, log *slog.Logger) {
	log.Error("Remediation steps:")
	log.Error(fmt.Sprintf("  1. Ensure the Google Chat API is enabled:"))
	log.Error(fmt.Sprintf("       gcloud services enable chat.googleapis.com --project=%s", projectID))
	if credentialsFile == "" {
		log.Error(fmt.Sprintf("  2. Ensure the GCE VM's OAuth scopes include the chat.bot scope."))
		log.Error(fmt.Sprintf("       The cloud-platform scope does NOT cover Google Workspace APIs."))
		log.Error(fmt.Sprintf("       Stop the VM and update scopes (from your workstation):"))
		log.Error(fmt.Sprintf("       SA=$(curl -sH 'Metadata-Flavor: Google' http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/email)"))
		log.Error(fmt.Sprintf("       gcloud compute instances stop <VM> --zone=<ZONE> --project=%s", projectID))
		log.Error(fmt.Sprintf("       gcloud compute instances set-service-account <VM> --zone=<ZONE> --project=%s \\", projectID))
		log.Error(fmt.Sprintf("         --service-account=$SA --scopes=https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/chat.bot"))
		log.Error(fmt.Sprintf("       gcloud compute instances start <VM> --zone=<ZONE> --project=%s", projectID))
		log.Error(fmt.Sprintf("  3. Alternatively, set CHAT_APP_CREDENTIALS in chat-app.env to a service"))
		log.Error(fmt.Sprintf("       account key file path to bypass VM scopes entirely."))
	} else {
		log.Error(fmt.Sprintf("  2. Verify the key file at %s is valid and not expired.", credentialsFile))
		log.Error(fmt.Sprintf("  3. Ensure the service account key was created for a service account that"))
		log.Error(fmt.Sprintf("       is configured as the Chat app in the Google Chat API console."))
	}
}
