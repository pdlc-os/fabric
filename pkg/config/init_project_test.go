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
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnsureFabricGitignore_AddsEntry(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepoDir(t, tmpDir)

	// No .gitignore exists yet
	if err := EnsureFabricGitignore(tmpDir); err != nil {
		t.Fatalf("EnsureFabricGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(content) != ".fabric/agents/\n" {
		t.Errorf("expected '.fabric/agents/\\n', got %q", string(content))
	}
}

func TestEnsureFabricGitignore_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepoDir(t, tmpDir)

	// Write .gitignore with .fabric/ already present (covers agents/ too)
	_ = os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("node_modules/\n.fabric/\n"), 0644)

	if err := EnsureFabricGitignore(tmpDir); err != nil {
		t.Fatalf("EnsureFabricGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(content) != "node_modules/\n.fabric/\n" {
		t.Errorf("expected no change, got %q", string(content))
	}
}

func TestEnsureFabricGitignore_AppendsToExisting(t *testing.T) {
	tmpDir := t.TempDir()
	setupGitRepoDir(t, tmpDir)

	// Write .gitignore without trailing newline and without .fabric coverage
	_ = os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("node_modules/"), 0644)

	if err := EnsureFabricGitignore(tmpDir); err != nil {
		t.Fatalf("EnsureFabricGitignore failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if string(content) != "node_modules/\n.fabric/agents/\n" {
		t.Errorf("expected 'node_modules/\\n.fabric/agents/\\n', got %q", string(content))
	}
}

func TestEnsureFabricGitignore_RecognizesVariants(t *testing.T) {
	// All of these patterns cause git check-ignore to report .fabric/agents/ as ignored
	for _, pattern := range []string{".fabric", ".fabric/", "/.fabric", "/.fabric/", ".fabric/agents", ".fabric/agents/"} {
		t.Run(pattern, func(t *testing.T) {
			tmpDir := t.TempDir()
			setupGitRepoDir(t, tmpDir)
			_ = os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(pattern+"\n"), 0644)

			if err := EnsureFabricGitignore(tmpDir); err != nil {
				t.Fatalf("EnsureFabricGitignore failed for pattern %q: %v", pattern, err)
			}

			content, err := os.ReadFile(filepath.Join(tmpDir, ".gitignore"))
			if err != nil {
				t.Fatalf("failed to read .gitignore: %v", err)
			}
			// Should not have added another entry
			if string(content) != pattern+"\n" {
				t.Errorf("for pattern %q: expected no change, got %q", pattern, string(content))
			}
		})
	}
}

func TestInitProject_NonGitCreatesMarkerAndExternalDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	mockRuntimeDetection(t, "docker")

	// Create a non-git project directory
	projectDir := t.TempDir()
	fabricDir := filepath.Join(projectDir, ".fabric")

	// Change to the project directory (non-git)
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	_ = os.Chdir(projectDir)

	// InitProject with the .fabric path as target
	if err := InitProject(fabricDir, GetMockHarnesses()); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	// Verify .fabric is a marker file (not a directory)
	markerPath := filepath.Join(projectDir, ".fabric")
	info, err := os.Stat(markerPath)
	if err != nil {
		t.Fatalf("marker file does not exist: %v", err)
	}
	if info.IsDir() {
		t.Fatal("expected .fabric to be a file (marker), but it's a directory")
	}

	// Read the marker and verify content
	marker, err := ReadProjectMarker(markerPath)
	if err != nil {
		t.Fatalf("ReadProjectMarker failed: %v", err)
	}
	if marker.ProjectSlug == "" {
		t.Error("marker should have a project-slug")
	}
	if marker.ProjectID == "" {
		t.Error("marker should have a project-id")
	}

	// Verify external project directory was created
	externalPath, err := marker.ExternalProjectPath()
	if err != nil {
		t.Fatalf("ExternalProjectPath failed: %v", err)
	}

	// Check standard dirs
	for _, sub := range []string{"templates", "agents"} {
		p := filepath.Join(externalPath, sub)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("expected %s/ to exist in external project", sub)
		}
	}

	// Check settings.yaml with workspace_path
	settingsPath := filepath.Join(externalPath, "settings.yaml")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Fatal("expected settings.yaml in external project")
	}
	data, _ := os.ReadFile(settingsPath)
	if !containsSubstring(string(data), "workspace_path") {
		t.Error("settings.yaml should contain workspace_path")
	}
	if !containsSubstring(string(data), "grove_id") {
		t.Error("settings.yaml should contain grove_id")
	}
}

func TestInitProject_NonGitRejectsOldStyleDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	mockRuntimeDetection(t, "docker")

	// Create a non-git project with old-style .fabric directory
	projectDir := t.TempDir()
	fabricDir := filepath.Join(projectDir, ".fabric")
	_ = os.MkdirAll(fabricDir, 0755)

	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	_ = os.Chdir(projectDir)

	err := InitProject(fabricDir, GetMockHarnesses())
	if err == nil {
		t.Fatal("expected error for old-style non-git project, got nil")
	}
	if !containsSubstring(err.Error(), "outdated") {
		t.Errorf("expected error about outdated format, got: %v", err)
	}
}

func TestInitProject_NonGitIdempotent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	mockRuntimeDetection(t, "docker")

	projectDir := t.TempDir()
	fabricDir := filepath.Join(projectDir, ".fabric")

	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	_ = os.Chdir(projectDir)

	// First init
	if err := InitProject(fabricDir, GetMockHarnesses()); err != nil {
		t.Fatalf("first InitProject failed: %v", err)
	}

	// Read marker from first init
	marker1, _ := ReadProjectMarker(filepath.Join(projectDir, ".fabric"))

	// Second init should succeed and use existing marker
	if err := InitProject(fabricDir, GetMockHarnesses()); err != nil {
		t.Fatalf("second InitProject failed: %v", err)
	}

	// Read marker after second init — should be unchanged
	marker2, _ := ReadProjectMarker(filepath.Join(projectDir, ".fabric"))
	if marker1.ProjectID != marker2.ProjectID {
		t.Errorf("project-id changed between inits: %q → %q", marker1.ProjectID, marker2.ProjectID)
	}
}

func TestInitProject_GitCreatesProjectIDAndExternalDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	mockRuntimeDetection(t, "docker")

	// Create a git repo
	projectDir := filepath.Join(t.TempDir(), "my-git-project")
	_ = os.MkdirAll(projectDir, 0755)
	setupGitRepoDir(t, projectDir)

	fabricDir := filepath.Join(projectDir, ".fabric")

	// Change to the project directory
	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	_ = os.Chdir(projectDir)

	if err := InitProject(fabricDir, GetMockHarnesses()); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	// Verify .fabric is a directory (not a marker file, since it's a git project)
	info, err := os.Stat(fabricDir)
	if err != nil {
		t.Fatalf(".fabric does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected .fabric to be a directory for git projects")
	}

	// Verify grove-id file was created
	projectID, err := ReadProjectID(fabricDir)
	if err != nil {
		t.Fatalf("ReadProjectID failed: %v", err)
	}
	if projectID == "" {
		t.Error("grove-id should not be empty")
	}

	// Verify external agents directory was created
	externalDir, err := GetGitProjectExternalAgentsDir(fabricDir)
	if err != nil {
		t.Fatalf("GetGitProjectExternalAgentsDir failed: %v", err)
	}
	if externalDir == "" {
		t.Fatal("external agents dir should not be empty")
	}
	if _, err := os.Stat(externalDir); os.IsNotExist(err) {
		t.Errorf("expected external agents directory to exist at %s", externalDir)
	}

	// Verify settings.yaml is in the external config dir (machine-specific, not committed)
	externalConfigDir, err := GetGitProjectExternalConfigDir(fabricDir)
	if err != nil {
		t.Fatalf("GetGitProjectExternalConfigDir failed: %v", err)
	}
	if externalConfigDir == "" {
		t.Fatal("external config dir should not be empty")
	}
	if _, err := os.Stat(externalConfigDir); os.IsNotExist(err) {
		t.Errorf("expected external config directory to exist at %s", externalConfigDir)
	}
	if _, err := os.Stat(filepath.Join(externalConfigDir, "settings.yaml")); os.IsNotExist(err) {
		t.Errorf("expected settings.yaml to exist in external config dir %s", externalConfigDir)
	}

	// Verify templates/ lives in-repo (committable) and settings.yaml is NOT in-repo
	if _, err := os.Stat(filepath.Join(fabricDir, "templates")); os.IsNotExist(err) {
		t.Error("expected templates/ to exist in-repo for git projects (committable)")
	}
	if _, err := os.Stat(filepath.Join(fabricDir, "settings.yaml")); err == nil {
		t.Error("settings.yaml should not exist in-repo for git projects")
	}

	// Verify agents dir exists in-repo (for worktrees)
	agentsDir := filepath.Join(fabricDir, "agents")
	if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
		t.Error("expected agents/ to exist in-repo for worktrees")
	}
}

func TestInitProject_GitIdempotentProjectID(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	mockRuntimeDetection(t, "docker")

	projectDir := filepath.Join(t.TempDir(), "idempotent-project")
	_ = os.MkdirAll(projectDir, 0755)
	setupGitRepoDir(t, projectDir)

	fabricDir := filepath.Join(projectDir, ".fabric")

	origWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(origWd) }()
	_ = os.Chdir(projectDir)

	// First init
	if err := InitProject(fabricDir, GetMockHarnesses()); err != nil {
		t.Fatalf("first InitProject failed: %v", err)
	}
	projectID1, _ := ReadProjectID(fabricDir)

	// Second init
	if err := InitProject(fabricDir, GetMockHarnesses()); err != nil {
		t.Fatalf("second InitProject failed: %v", err)
	}
	projectID2, _ := ReadProjectID(fabricDir)

	if projectID1 != projectID2 {
		t.Errorf("project-id changed between inits: %q → %q", projectID1, projectID2)
	}
}

// setupGitRepoDir initializes a git repository in the given directory.
func setupGitRepoDir(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
}

func TestInitProject_CreatesEmptyTemplatesDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	mockRuntimeDetection(t, "docker")
	mockIsGitRepo(t, true)

	tempDir := t.TempDir()

	// Run InitProject
	if err := InitProject(tempDir, GetMockHarnesses()); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	// Templates always live in the in-repo .fabric/templates/ (for git projects) or
	// in the external config dir (for non-git projects). Since tests run inside a git repo,
	// check tempDir directly.
	templatesDir := filepath.Join(tempDir, "templates")
	if info, err := os.Stat(templatesDir); err != nil || !info.IsDir() {
		t.Fatalf("Expected templates/ directory to exist at %s", templatesDir)
	}

	// Verify that templates/default does NOT exist (default template lives in global project only)
	defaultDir := filepath.Join(tempDir, "templates", "default")
	if _, err := os.Stat(defaultDir); !os.IsNotExist(err) {
		t.Errorf("Expected templates/default to NOT exist at project level, but it does at %s", defaultDir)
	}

	// Verify per-harness templates were NOT created
	for _, name := range []string{"gemini", "claude"} {
		perHarnessDir := filepath.Join(tempDir, "templates", name)
		if _, err := os.Stat(perHarnessDir); !os.IsNotExist(err) {
			t.Errorf("Expected per-harness template %s to NOT be created at project level", name)
		}
	}
}
