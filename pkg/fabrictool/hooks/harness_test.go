/*
Copyright 2025 The Scion Authors.
*/

package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDialect implements Dialect for testing
type mockDialect struct {
	name string
}

func (d *mockDialect) Name() string {
	return d.name
}

func (d *mockDialect) Parse(data map[string]interface{}) (*Event, error) {
	return &Event{
		Name:    getString(data, "event"),
		RawName: getString(data, "event"),
		Dialect: d.name,
		Data:    EventData{Raw: data},
	}, nil
}

func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

func TestHarnessProcessor_RegisterDialect(t *testing.T) {
	p := NewHarnessProcessor()
	d := &mockDialect{name: "test"}

	p.RegisterDialect(d)

	assert.Contains(t, p.Dialects, "test")
	assert.Equal(t, d, p.Dialects["test"])
}

func TestHarnessProcessor_ProcessRaw(t *testing.T) {
	p := NewHarnessProcessor()
	p.RegisterDialect(&mockDialect{name: "test"})

	var receivedEvent *Event
	p.AddHandler(func(e *Event) error {
		receivedEvent = e
		return nil
	})

	data := map[string]interface{}{
		"event": "test-event",
	}

	err := p.ProcessRaw(data, "test")
	require.NoError(t, err)
	require.NotNil(t, receivedEvent)
	assert.Equal(t, "test-event", receivedEvent.Name)
	assert.Equal(t, "test", receivedEvent.Dialect)
}

func TestHarnessProcessor_ProcessRaw_UnknownDialect(t *testing.T) {
	p := NewHarnessProcessor()

	data := map[string]interface{}{
		"event": "test-event",
	}

	err := p.ProcessRaw(data, "unknown")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown dialect")
}

func TestHarnessProcessor_ProcessJSON(t *testing.T) {
	p := NewHarnessProcessor()
	p.RegisterDialect(&mockDialect{name: "json-test"})

	var receivedEvent *Event
	p.AddHandler(func(e *Event) error {
		receivedEvent = e
		return nil
	})

	jsonData := []byte(`{"event": "json-event"}`)

	err := p.ProcessJSON(jsonData, "json-test")
	require.NoError(t, err)
	require.NotNil(t, receivedEvent)
	assert.Equal(t, "json-event", receivedEvent.Name)
}

func TestHarnessProcessor_ProcessJSON_InvalidJSON(t *testing.T) {
	p := NewHarnessProcessor()
	p.RegisterDialect(&mockDialect{name: "test"})

	jsonData := []byte(`{invalid json}`)

	err := p.ProcessJSON(jsonData, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing JSON")
}
