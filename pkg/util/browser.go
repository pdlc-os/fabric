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

package util

import (
	"os"
	"os/exec"
	"runtime"
)

// OpenBrowser opens the specified URL in the default browser.
// Returns an error if the browser could not be opened.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		// Try xdg-open first, fall back to common browsers
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		// Try xdg-open as a fallback for other Unix-like systems
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}

// IsHeadlessEnvironment returns true if the current environment lacks a display
// server, suggesting a headless (no browser) environment.
// FABRIC_HEADLESS=1 forces headless mode. macOS always returns false (has a display).
// On other platforms, checks for DISPLAY or WAYLAND_DISPLAY environment variables.
func IsHeadlessEnvironment() bool {
	if os.Getenv("FABRIC_HEADLESS") == "1" {
		return true
	}
	if runtime.GOOS == "darwin" {
		return false
	}
	display := os.Getenv("DISPLAY")
	wayland := os.Getenv("WAYLAND_DISPLAY")
	return display == "" && wayland == ""
}
