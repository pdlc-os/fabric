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

package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print the current agent's identity",
	Long: `Print the current agent's identity when running inside an agent container.
Returns the agent slug by default, or full identity details with --format json.

When run outside an agent container, falls back to the system whoami command.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := os.Getenv("FABRIC_AGENT_SLUG")
		name := os.Getenv("FABRIC_AGENT_NAME")

		if slug == "" && name == "" {
			return runSystemWhoami()
		}

		if slug == "" {
			slug = name
		}

		if isJSONOutput() {
			return outputJSON(map[string]string{
				"slug": slug,
				"name": name,
				"id":   os.Getenv("FABRIC_AGENT_ID"),
			})
		}

		fmt.Println(slug)
		return nil
	},
}

func runSystemWhoami() error {
	path, err := exec.LookPath("whoami")
	if err != nil {
		return fmt.Errorf("not running as a fabric agent and system whoami not found")
	}
	sysCmd := exec.Command(path)
	sysCmd.Stdout = os.Stdout
	sysCmd.Stderr = os.Stderr
	return sysCmd.Run()
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
