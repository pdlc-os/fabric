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

package hub

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/pdlc-os/fabric/pkg/messages"
)

// EmailChannel delivers notifications via SMTP email.
type EmailChannel struct {
	host     string // SMTP host (e.g., "smtp.example.com")
	port     string // SMTP port (e.g., "587")
	from     string // Sender email address
	to       string // Recipient email address (comma-separated for multiple)
	username string // SMTP auth username
	password string // SMTP auth password
}

// NewEmailChannel creates an EmailChannel from params.
// Supported params:
//   - host: SMTP host (required)
//   - port: SMTP port (default "587")
//   - from: sender email address (required)
//   - to: recipient email address(es), comma-separated (required)
//   - username: SMTP auth username (optional, skips auth if empty)
//   - password: SMTP auth password (optional)
func NewEmailChannel(params map[string]string) *EmailChannel {
	port := params["port"]
	if port == "" {
		port = "587"
	}

	return &EmailChannel{
		host:     params["host"],
		port:     port,
		from:     params["from"],
		to:       params["to"],
		username: params["username"],
		password: params["password"],
	}
}

func (e *EmailChannel) Name() string { return "email" }

func (e *EmailChannel) Validate() error {
	if e.host == "" {
		return fmt.Errorf("email channel requires a 'host' param")
	}
	if e.from == "" {
		return fmt.Errorf("email channel requires a 'from' param")
	}
	if e.to == "" {
		return fmt.Errorf("email channel requires a 'to' param")
	}
	return nil
}

func (e *EmailChannel) Deliver(_ context.Context, msg *messages.StructuredMessage) error {
	subject := fmt.Sprintf("[Fabric] %s", msg.Type)
	if msg.Sender != "" {
		subject = fmt.Sprintf("[Fabric] %s from %s", msg.Type, msg.Sender)
	}

	recipients := strings.Split(e.to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n\r\nSender: %s\r\nRecipient: %s\r\nType: %s\r\nTimestamp: %s",
		e.from,
		strings.Join(recipients, ", "),
		subject,
		msg.Msg,
		msg.Sender,
		msg.Recipient,
		msg.Type,
		msg.Timestamp,
	)

	addr := e.host + ":" + e.port

	var auth smtp.Auth
	if e.username != "" {
		auth = smtp.PlainAuth("", e.username, e.password, e.host)
	}

	if err := smtp.SendMail(addr, auth, e.from, recipients, []byte(body)); err != nil {
		return fmt.Errorf("email delivery failed: %w", err)
	}

	return nil
}
