/*
Copyright 2025 The Scion Authors.
*/

package telemetry

import (
	"crypto/sha256"
	"encoding/hex"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Filter provides include/exclude filtering for event types.
type Filter struct {
	include map[string]bool // nil = include all
	exclude map[string]bool
}

// RedactionConfig holds configuration for field-level redaction and hashing.
type RedactionConfig struct {
	// Redact is a list of field names to replace with "[REDACTED]"
	Redact []string
	// Hash is a list of field names to hash (SHA256)
	Hash []string
}

// Redactor provides field-level redaction and hashing for telemetry attributes.
type Redactor struct {
	redactFields map[string]bool
	hashFields   map[string]bool
}

// NewRedactor creates a new Redactor from configuration.
func NewRedactor(config RedactionConfig) *Redactor {
	r := &Redactor{
		redactFields: make(map[string]bool),
		hashFields:   make(map[string]bool),
	}

	for _, f := range config.Redact {
		r.redactFields[f] = true
	}
	for _, f := range config.Hash {
		r.hashFields[f] = true
	}

	return r
}

// ShouldRedact returns true if the field should be redacted.
func (r *Redactor) ShouldRedact(key string) bool {
	if r == nil {
		return false
	}
	return r.redactFields[key]
}

// ShouldHash returns true if the field should be hashed.
func (r *Redactor) ShouldHash(key string) bool {
	if r == nil {
		return false
	}
	return r.hashFields[key]
}

// HashValue returns the SHA256 hash of a value as a hex string.
func HashValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

// RedactProtoAttributes applies redaction and hashing to OTLP proto attributes.
func (r *Redactor) RedactProtoAttributes(attrs []*commonpb.KeyValue) []*commonpb.KeyValue {
	if r == nil || len(attrs) == 0 {
		return attrs
	}

	result := make([]*commonpb.KeyValue, len(attrs))
	for i, kv := range attrs {
		result[i] = r.redactProtoKeyValue(kv)
	}
	return result
}

// redactProtoKeyValue redacts or hashes a single KeyValue.
func (r *Redactor) redactProtoKeyValue(kv *commonpb.KeyValue) *commonpb.KeyValue {
	if kv == nil {
		return nil
	}

	key := kv.Key

	// Check if this field should be redacted
	if r.ShouldRedact(key) {
		return &commonpb.KeyValue{
			Key: key,
			Value: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_StringValue{StringValue: "[REDACTED]"},
			},
		}
	}

	// Check if this field should be hashed
	if r.ShouldHash(key) {
		if sv, ok := kv.Value.Value.(*commonpb.AnyValue_StringValue); ok {
			return &commonpb.KeyValue{
				Key: key,
				Value: &commonpb.AnyValue{
					Value: &commonpb.AnyValue_StringValue{StringValue: HashValue(sv.StringValue)},
				},
			}
		}
	}

	// Return unchanged
	return kv
}

// RedactSpan applies redaction and hashing to a span's attributes.
func (r *Redactor) RedactSpan(span *tracepb.Span) *tracepb.Span {
	if r == nil || span == nil {
		return span
	}

	// Create a copy with redacted attributes
	redactedSpan := &tracepb.Span{
		TraceId:                span.TraceId,
		SpanId:                 span.SpanId,
		TraceState:             span.TraceState,
		ParentSpanId:           span.ParentSpanId,
		Flags:                  span.Flags,
		Name:                   span.Name,
		Kind:                   span.Kind,
		StartTimeUnixNano:      span.StartTimeUnixNano,
		EndTimeUnixNano:        span.EndTimeUnixNano,
		Attributes:             r.RedactProtoAttributes(span.Attributes),
		DroppedAttributesCount: span.DroppedAttributesCount,
		Events:                 span.Events,
		DroppedEventsCount:     span.DroppedEventsCount,
		Links:                  span.Links,
		DroppedLinksCount:      span.DroppedLinksCount,
		Status:                 span.Status,
	}

	// Redact event attributes as well
	if len(span.Events) > 0 {
		redactedSpan.Events = make([]*tracepb.Span_Event, len(span.Events))
		for i, evt := range span.Events {
			redactedSpan.Events[i] = &tracepb.Span_Event{
				TimeUnixNano:           evt.TimeUnixNano,
				Name:                   evt.Name,
				Attributes:             r.RedactProtoAttributes(evt.Attributes),
				DroppedAttributesCount: evt.DroppedAttributesCount,
			}
		}
	}

	return redactedSpan
}

// NewFilter creates a new filter from configuration.
func NewFilter(config FilterConfig) *Filter {
	f := &Filter{}

	// Build include set (nil means include all)
	if len(config.Include) > 0 {
		f.include = make(map[string]bool, len(config.Include))
		for _, t := range config.Include {
			f.include[t] = true
		}
	}

	// Build exclude set
	if len(config.Exclude) > 0 {
		f.exclude = make(map[string]bool, len(config.Exclude))
		for _, t := range config.Exclude {
			f.exclude[t] = true
		}
	}

	return f
}

// ShouldProcess returns true if the event type should be processed.
// An event is processed if:
// 1. It's in the include list (or include list is empty, meaning include all)
// 2. AND it's not in the exclude list
func (f *Filter) ShouldProcess(eventType string) bool {
	if f == nil {
		return true
	}

	// Check include list first (nil = include all)
	if f.include != nil && !f.include[eventType] {
		return false
	}

	// Check exclude list
	if f.exclude != nil && f.exclude[eventType] {
		return false
	}

	return true
}

// ShouldProcessSpan checks if a span should be processed based on its name.
// This is a convenience method that treats span name as the event type.
func (f *Filter) ShouldProcessSpan(spanName string) bool {
	return f.ShouldProcess(spanName)
}
