/*
Copyright 2025 The Scion Authors.
*/

package dialects

import (
	"testing"

	"github.com/GoogleCloudPlatform/scion/pkg/sciontool/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiDialect_Name(t *testing.T) {
	d := NewGeminiDialect()
	assert.Equal(t, "gemini", d.Name())
}

func TestGeminiDialect_Parse(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]interface{}
		wantName   string
		wantTool   string
		wantPrompt string
	}{
		{
			name: "BeforeTool event",
			input: map[string]interface{}{
				"hook_event_name": "BeforeTool",
				"tool_name":       "shell",
			},
			wantName: hooks.EventToolStart,
			wantTool: "shell",
		},
		{
			name: "AfterTool event",
			input: map[string]interface{}{
				"hook_event_name": "AfterTool",
				"tool_name":       "read_file",
			},
			wantName: hooks.EventToolEnd,
			wantTool: "read_file",
		},
		{
			name: "BeforeAgent event",
			input: map[string]interface{}{
				"hook_event_name": "BeforeAgent",
				"prompt":          "Write some code",
			},
			wantName:   hooks.EventAgentStart,
			wantPrompt: "Write some code",
		},
		{
			name: "AfterAgent event",
			input: map[string]interface{}{
				"hook_event_name": "AfterAgent",
			},
			wantName: hooks.EventAgentEnd,
		},
		{
			name: "SessionStart event",
			input: map[string]interface{}{
				"hook_event_name": "SessionStart",
			},
			wantName: hooks.EventSessionStart,
		},
		{
			name: "SessionEnd event",
			input: map[string]interface{}{
				"hook_event_name": "SessionEnd",
				"reason":          "complete",
			},
			wantName: hooks.EventSessionEnd,
		},
		{
			name: "BeforeModel event",
			input: map[string]interface{}{
				"hook_event_name": "BeforeModel",
			},
			wantName: hooks.EventModelStart,
		},
		{
			name: "AfterModel event",
			input: map[string]interface{}{
				"hook_event_name": "AfterModel",
			},
			wantName: hooks.EventModelEnd,
		},
		{
			name: "Notification event",
			input: map[string]interface{}{
				"hook_event_name": "Notification",
				"message":         "Some notification",
			},
			wantName: hooks.EventNotification,
		},
	}

	d := NewGeminiDialect()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := d.Parse(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, event.Name)
			assert.Equal(t, "gemini", event.Dialect)

			if tt.wantTool != "" {
				assert.Equal(t, tt.wantTool, event.Data.ToolName)
			}
			if tt.wantPrompt != "" {
				assert.Equal(t, tt.wantPrompt, event.Data.Prompt)
			}
		})
	}
}

func TestGeminiDialect_ParseFilePath(t *testing.T) {
	d := NewGeminiDialect()

	t.Run("file_path from tool_input object", func(t *testing.T) {
		event, err := d.Parse(map[string]interface{}{
			"hook_event_name": "AfterTool",
			"tool_name":       "replace",
			"tool_input": map[string]interface{}{
				"file_path":  "src/index.ts",
				"old_string": "foo",
				"new_string": "bar",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "src/index.ts", event.Data.FilePath)
	})

	t.Run("no file_path when tool_input is string", func(t *testing.T) {
		event, err := d.Parse(map[string]interface{}{
			"hook_event_name": "AfterTool",
			"tool_name":       "shell",
			"tool_input":      "ls -la",
		})
		require.NoError(t, err)
		assert.Empty(t, event.Data.FilePath)
	})
}
