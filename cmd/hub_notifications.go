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

	"github.com/spf13/cobra"
)

// hubNotificationsCmd is kept as a redirect to the new top-level notifications command.
var hubNotificationsCmd = &cobra.Command{
	Use:        "notifications",
	Aliases:    []string{"notification"},
	Short:      "Notifications have moved to 'fabric notifications'",
	Deprecated: "use 'fabric notifications' instead.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("this command has moved. Use 'fabric notifications' instead")
	},
}

// hubNotificationsAckCmd is kept as a redirect.
var hubNotificationsAckCmd = &cobra.Command{
	Use:        "ack [notification-id]",
	Short:      "Acknowledge notification(s) — moved to 'fabric notifications ack'",
	Deprecated: "use 'fabric notifications ack' instead.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("this command has moved. Use 'fabric notifications ack' instead")
	},
}

func init() {
	hubCmd.AddCommand(hubNotificationsCmd)
	hubNotificationsCmd.AddCommand(hubNotificationsAckCmd)
}
