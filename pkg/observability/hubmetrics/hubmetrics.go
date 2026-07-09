/*
Copyright 2026 The Fabric Authors.
*/

// Package hubmetrics creates the OpenTelemetry MeterProvider used by hub-side
// metric recorders (dbmetrics, dispatchmetrics). It exports directly to GCP
// Cloud Monitoring via Application Default Credentials.
package hubmetrics

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const defaultExportInterval = 60 * time.Second

// MetricGroup identifies a logical group of hub metrics that can be
// independently enabled or disabled.
type MetricGroup struct {
	EnvVar      string
	NamePattern string
}

var metricGroups = []MetricGroup{
	{EnvVar: "FABRIC_METRICS_DB_NOTIFY", NamePattern: "fabric.db.notify.*"},
	{EnvVar: "FABRIC_METRICS_DB_POOL", NamePattern: "fabric.db.pool.*"},
	{EnvVar: "FABRIC_METRICS_DISPATCH", NamePattern: "fabric.dispatch.*"},
	{EnvVar: "FABRIC_METRICS_HUB_AUTH", NamePattern: "fabric.hub.auth.*"},
	{EnvVar: "FABRIC_METRICS_HUB_AUTH", NamePattern: "fabric.hub.registration.*"},
	{EnvVar: "FABRIC_METRICS_HUB_AUTH", NamePattern: "fabric.hub.join.*"},
	{EnvVar: "FABRIC_METRICS_HUB_AUTH", NamePattern: "fabric.hub.rotation.*"},
	{EnvVar: "FABRIC_METRICS_HUB_AUTH", NamePattern: "fabric.hub.brokers.*"},
	{EnvVar: "FABRIC_METRICS_HUB_AUTH", NamePattern: "fabric.hub.dispatch.*"},
	{EnvVar: "FABRIC_METRICS_HUB_GCP", NamePattern: "fabric.hub.gcp.*"},
}

// Option configures the MeterProvider.
type Option func(*options)

type options struct {
	exportInterval time.Duration
	hubID          string
}

// WithExportInterval sets the periodic reader interval. Defaults to 60s.
func WithExportInterval(d time.Duration) Option {
	return func(o *options) { o.exportInterval = d }
}

// WithHubID sets the fabric.hub.id resource attribute.
func WithHubID(id string) Option {
	return func(o *options) { o.hubID = id }
}

// NewMeterProvider creates an OTel SDK MeterProvider that exports to GCP Cloud
// Monitoring. It uses Application Default Credentials (workload identity on
// Cloud Run, attached SA on GCE).
//
// If gcpProjectID is empty, an error is returned — callers should fall back to
// disabled recorders.
func NewMeterProvider(ctx context.Context, gcpProjectID string, opts ...Option) (*metric.MeterProvider, error) {
	if gcpProjectID == "" {
		return nil, fmt.Errorf("GCP project ID is required for hub metrics export")
	}

	o := &options{exportInterval: defaultExportInterval}
	for _, fn := range opts {
		fn(o)
	}

	exporter, err := mexporter.New(mexporter.WithProjectID(gcpProjectID))
	if err != nil {
		return nil, fmt.Errorf("creating GCP metric exporter: %w", err)
	}

	resAttrs := []attribute.KeyValue{
		semconv.ServiceName("fabric-hub"),
	}
	if o.hubID != "" {
		resAttrs = append(resAttrs, attribute.String("fabric.hub.id", o.hubID))
	}
	if envHubID := os.Getenv("FABRIC_HUB_ID"); envHubID != "" && o.hubID == "" {
		resAttrs = append(resAttrs, attribute.String("fabric.hub.id", envHubID))
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(resAttrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	mpOpts := []metric.Option{
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(exporter,
			metric.WithInterval(o.exportInterval),
		)),
	}

	mpOpts = append(mpOpts, groupDropViews()...)

	return metric.NewMeterProvider(mpOpts...), nil
}

// groupDropViews returns OTel View options that drop instruments belonging to
// disabled metric groups. A group is disabled when its env var is set to
// "false" or "0". All groups are enabled by default.
func groupDropViews() []metric.Option {
	var opts []metric.Option
	for _, g := range metricGroups {
		if isGroupDisabled(g.EnvVar) {
			opts = append(opts, metric.WithView(metric.NewView(
				metric.Instrument{Name: g.NamePattern},
				metric.Stream{Aggregation: metric.AggregationDrop{}},
			)))
		}
	}
	return opts
}

func isGroupDisabled(envVar string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(envVar)))
	return v == "false" || v == "0"
}

// GroupScopes returns the instrumentation scopes for each metric group, useful
// for testing and documentation.
func GroupScopes() []MetricGroup {
	return append([]MetricGroup(nil), metricGroups...)
}

// InstrumentationScope returns a scope matching the dbmetrics or
// dispatchmetrics package, useful for building Views in tests.
func InstrumentationScope(name string) instrumentation.Scope {
	return instrumentation.Scope{Name: name}
}
