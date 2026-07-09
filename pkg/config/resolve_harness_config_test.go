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

package config

import (
	"testing"

	"github.com/pdlc-os/fabric/pkg/api"
)

func TestResolveHarnessConfigName_CLIFlag(t *testing.T) {
	res, err := ResolveHarnessConfigName(HarnessConfigInputs{
		CLIFlag: "claude-custom",
		TemplateCfg: &api.FabricConfig{
			DefaultHarnessConfig: "gemini",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "claude-custom" {
		t.Errorf("expected name 'claude-custom', got %q", res.Name)
	}
	if res.Source != "cli-flag" {
		t.Errorf("expected source 'cli-flag', got %q", res.Source)
	}
}

func TestResolveHarnessConfigName_StoredConfig(t *testing.T) {
	res, err := ResolveHarnessConfigName(HarnessConfigInputs{
		StoredConfig: &api.FabricConfig{HarnessConfig: "claude"},
		TemplateCfg:  &api.FabricConfig{DefaultHarnessConfig: "gemini"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "claude" {
		t.Errorf("expected name 'claude', got %q", res.Name)
	}
	if res.Source != "stored-config" {
		t.Errorf("expected source 'stored-config', got %q", res.Source)
	}
}

func TestResolveHarnessConfigName_TemplateDefault(t *testing.T) {
	res, err := ResolveHarnessConfigName(HarnessConfigInputs{
		TemplateCfg: &api.FabricConfig{
			DefaultHarnessConfig: "gemini",
			HarnessConfig:        "claude",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "gemini" {
		t.Errorf("expected name 'gemini', got %q", res.Name)
	}
	if res.Source != "template-default" {
		t.Errorf("expected source 'template-default', got %q", res.Source)
	}
}

func TestResolveHarnessConfigName_TemplateHarnessConfig(t *testing.T) {
	res, err := ResolveHarnessConfigName(HarnessConfigInputs{
		TemplateCfg: &api.FabricConfig{HarnessConfig: "claude"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "claude" {
		t.Errorf("expected name 'claude', got %q", res.Name)
	}
	if res.Source != "template-harness-config" {
		t.Errorf("expected source 'template-harness-config', got %q", res.Source)
	}
}

func TestResolveHarnessConfigName_StoredHarnessNameFallback(t *testing.T) {
	res, err := ResolveHarnessConfigName(HarnessConfigInputs{
		StoredConfig: &api.FabricConfig{Harness: "gemini"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "gemini" {
		t.Errorf("expected name 'gemini', got %q", res.Name)
	}
	if res.Source != "stored-harness-name" {
		t.Errorf("expected source 'stored-harness-name', got %q", res.Source)
	}
}

func TestResolveHarnessConfigName_ProfileDefault(t *testing.T) {
	res, err := ResolveHarnessConfigName(HarnessConfigInputs{
		ProfileName: "dev",
		Settings: &VersionedSettings{
			Profiles: map[string]V1ProfileConfig{
				"dev": {DefaultHarnessConfig: "claude"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "claude" {
		t.Errorf("expected name 'claude', got %q", res.Name)
	}
	if res.Source != "profile-dev" {
		t.Errorf("expected source 'profile-dev', got %q", res.Source)
	}
}

func TestResolveHarnessConfigName_ProfileDefault_ActiveProfile(t *testing.T) {
	res, err := ResolveHarnessConfigName(HarnessConfigInputs{
		Settings: &VersionedSettings{
			ActiveProfile: "prod",
			Profiles: map[string]V1ProfileConfig{
				"prod": {DefaultHarnessConfig: "gemini"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "gemini" {
		t.Errorf("expected name 'gemini', got %q", res.Name)
	}
	if res.Source != "profile-prod" {
		t.Errorf("expected source 'profile-prod', got %q", res.Source)
	}
}

func TestResolveHarnessConfigName_SettingsDefault(t *testing.T) {
	res, err := ResolveHarnessConfigName(HarnessConfigInputs{
		Settings: &VersionedSettings{
			DefaultHarnessConfig: "claude",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "claude" {
		t.Errorf("expected name 'claude', got %q", res.Name)
	}
	if res.Source != "settings-default" {
		t.Errorf("expected source 'settings-default', got %q", res.Source)
	}
}

func TestResolveHarnessConfigName_Error(t *testing.T) {
	_, err := ResolveHarnessConfigName(HarnessConfigInputs{})
	if err == nil {
		t.Fatal("expected error when no harness-config can be resolved")
	}
}

func TestResolveHarnessConfigName_PriorityChain(t *testing.T) {
	// All sources present — CLI flag should win
	res, err := ResolveHarnessConfigName(HarnessConfigInputs{
		CLIFlag:      "from-cli",
		StoredConfig: &api.FabricConfig{HarnessConfig: "from-stored"},
		TemplateCfg:  &api.FabricConfig{DefaultHarnessConfig: "from-template"},
		Settings:     &VersionedSettings{DefaultHarnessConfig: "from-settings"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "from-cli" {
		t.Errorf("expected CLI flag to win, got %q (source=%s)", res.Name, res.Source)
	}

	// Without CLI flag — stored config should win
	res, err = ResolveHarnessConfigName(HarnessConfigInputs{
		StoredConfig: &api.FabricConfig{HarnessConfig: "from-stored"},
		TemplateCfg:  &api.FabricConfig{DefaultHarnessConfig: "from-template"},
		Settings:     &VersionedSettings{DefaultHarnessConfig: "from-settings"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "from-stored" {
		t.Errorf("expected stored config to win, got %q (source=%s)", res.Name, res.Source)
	}

	// Without CLI or stored — template default should win
	res, err = ResolveHarnessConfigName(HarnessConfigInputs{
		TemplateCfg: &api.FabricConfig{DefaultHarnessConfig: "from-template"},
		Settings:    &VersionedSettings{DefaultHarnessConfig: "from-settings"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "from-template" {
		t.Errorf("expected template default to win, got %q (source=%s)", res.Name, res.Source)
	}
}
