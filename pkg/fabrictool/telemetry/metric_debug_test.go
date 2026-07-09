/*
Copyright 2025 The Scion Authors.
*/

package telemetry

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type stubMetricExporter struct {
	exportCalls     int
	forceFlushCalls int
	shutdownCalls   int
}

func (s *stubMetricExporter) Temporality(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (s *stubMetricExporter) Aggregation(sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(sdkmetric.InstrumentKindCounter)
}

func (s *stubMetricExporter) Export(context.Context, *metricdata.ResourceMetrics) error {
	s.exportCalls++
	return nil
}

func (s *stubMetricExporter) ForceFlush(context.Context) error {
	s.forceFlushCalls++
	return nil
}

func (s *stubMetricExporter) Shutdown(context.Context) error {
	s.shutdownCalls++
	return nil
}

func TestNewDebugMetricExporter_Passthrough(t *testing.T) {
	inner := &stubMetricExporter{}
	exporter := newDebugMetricExporter(inner)
	if exporter == nil {
		t.Fatal("expected exporter wrapper")
	}

	rm := &metricdata.ResourceMetrics{}
	if err := exporter.Export(context.Background(), rm); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if err := exporter.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() error = %v", err)
	}
	if err := exporter.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	if inner.exportCalls != 1 {
		t.Fatalf("exportCalls = %d, want 1", inner.exportCalls)
	}
	if inner.forceFlushCalls != 1 {
		t.Fatalf("forceFlushCalls = %d, want 1", inner.forceFlushCalls)
	}
	if inner.shutdownCalls != 1 {
		t.Fatalf("shutdownCalls = %d, want 1", inner.shutdownCalls)
	}
}

func TestSummarizeResourceMetrics(t *testing.T) {
	rm := &metricdata.ResourceMetrics{
		ScopeMetrics: []metricdata.ScopeMetrics{
			{
				Metrics: []metricdata.Metrics{
					{
						Name: "gen_ai.tokens.input",
						Data: metricdata.Sum[int64]{
							DataPoints: []metricdata.DataPoint[int64]{
								{Value: 10},
								{Value: 20},
							},
						},
					},
					{
						Name: "agent.tool.duration",
						Data: metricdata.Histogram[float64]{
							DataPoints: []metricdata.HistogramDataPoint[float64]{
								{Count: 1},
							},
						},
					},
				},
			},
		},
	}

	got := summarizeResourceMetrics(rm)
	if got != "gen_ai.tokens.input[sum[int64],2 points], agent.tool.duration[histogram[float64],1 points]" {
		t.Fatalf("unexpected summary: %s", got)
	}
}
