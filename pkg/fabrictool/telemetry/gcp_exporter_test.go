/*
Copyright 2025 The Scion Authors.
*/

package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

type captureMetricExporter struct {
	exports []*metricdata.ResourceMetrics
}

func (c *captureMetricExporter) Temporality(metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (c *captureMetricExporter) Aggregation(kind metric.InstrumentKind) metric.Aggregation {
	return metric.DefaultAggregationSelector(kind)
}

func (c *captureMetricExporter) Export(_ context.Context, rm *metricdata.ResourceMetrics) error {
	c.exports = append(c.exports, rm)
	return nil
}

func (c *captureMetricExporter) ForceFlush(context.Context) error { return nil }
func (c *captureMetricExporter) Shutdown(context.Context) error   { return nil }

func TestGCPExporter_ExportProtoMetrics(t *testing.T) {
	exp := &captureMetricExporter{}
	exporter := &GCPExporter{metricExporter: exp}

	err := exporter.ExportProtoMetrics(context.Background(), []*metricpb.ResourceMetrics{
		{
			ScopeMetrics: []*metricpb.ScopeMetrics{
				{
					Metrics: []*metricpb.Metric{
						{
							Name: "gemini_cli.token.usage",
							Data: &metricpb.Metric_Sum{
								Sum: &metricpb.Sum{
									AggregationTemporality: metricpb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
									DataPoints: []*metricpb.NumberDataPoint{
										{
											TimeUnixNano: 1,
											Value:        &metricpb.NumberDataPoint_AsInt{AsInt: 100},
										},
									},
								},
							},
						},
						{
							Name: "gen_ai.client.token.usage",
							Data: &metricpb.Metric_Sum{
								Sum: &metricpb.Sum{
									AggregationTemporality: metricpb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
									DataPoints: []*metricpb.NumberDataPoint{
										{
											TimeUnixNano: 2,
											Value:        &metricpb.NumberDataPoint_AsInt{AsInt: 55},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ExportProtoMetrics() error = %v", err)
	}
	if len(exp.exports) != 1 {
		t.Fatalf("len(exports) = %d, want 1", len(exp.exports))
	}
	if len(exp.exports[0].ScopeMetrics) != 1 || len(exp.exports[0].ScopeMetrics[0].Metrics) != 2 {
		t.Fatalf("unexpected exported metrics structure: %+v", exp.exports[0])
	}
	if got := exp.exports[0].ScopeMetrics[0].Metrics[0].Name; got != "gemini_cli.token.usage" {
		t.Fatalf("first exported metric name = %q, want gemini_cli.token.usage", got)
	}
	if got := exp.exports[0].ScopeMetrics[0].Metrics[1].Name; got != "gen_ai.client.token.usage" {
		t.Fatalf("second exported metric name = %q, want gen_ai.client.token.usage", got)
	}
}

func TestGCPExporter_ExportProtoMetrics_FiltersUnsupportedSummary(t *testing.T) {
	exp := &captureMetricExporter{}
	exporter := &GCPExporter{metricExporter: exp}

	err := exporter.ExportProtoMetrics(context.Background(), []*metricpb.ResourceMetrics{
		{
			ScopeMetrics: []*metricpb.ScopeMetrics{
				{
					Metrics: []*metricpb.Metric{
						{
							Name: "summary_only",
							Data: &metricpb.Metric_Summary{
								Summary: &metricpb.Summary{
									DataPoints: []*metricpb.SummaryDataPoint{
										{
											TimeUnixNano: 1,
											Attributes: []*commonpb.KeyValue{
												{Key: "k", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "v"}}},
											},
											Count: 1,
											Sum:   2,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ExportProtoMetrics() error = %v", err)
	}
	if len(exp.exports) != 0 {
		t.Fatalf("len(exports) = %d, want 0", len(exp.exports))
	}
}
