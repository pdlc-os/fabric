/*
Copyright 2025 The Scion Authors.
*/

package telemetry

import (
	"context"
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/scion/pkg/sciontool/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type debugMetricExporter struct {
	inner sdkmetric.Exporter
}

func newDebugMetricExporter(inner sdkmetric.Exporter) sdkmetric.Exporter {
	if inner == nil {
		return nil
	}
	return &debugMetricExporter{inner: inner}
}

func (d *debugMetricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return d.inner.Temporality(kind)
}

func (d *debugMetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return d.inner.Aggregation(kind)
}

func (d *debugMetricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	log.TaggedInfo("metrics", "exporting metrics batch: %s", summarizeResourceMetrics(rm))
	err := d.inner.Export(ctx, rm)
	if err != nil {
		log.Error("metrics export failed: %v", err)
		return err
	}
	log.TaggedInfo("metrics", "metrics batch export completed")
	return nil
}

func (d *debugMetricExporter) ForceFlush(ctx context.Context) error {
	log.TaggedInfo("metrics", "force flushing metric exporter")
	err := d.inner.ForceFlush(ctx)
	if err != nil {
		log.Error("metric exporter force flush failed: %v", err)
		return err
	}
	log.TaggedInfo("metrics", "metric exporter force flush completed")
	return nil
}

func (d *debugMetricExporter) Shutdown(ctx context.Context) error {
	log.TaggedInfo("metrics", "shutting down metric exporter")
	err := d.inner.Shutdown(ctx)
	if err != nil {
		log.Error("metric exporter shutdown failed: %v", err)
		return err
	}
	log.TaggedInfo("metrics", "metric exporter shutdown completed")
	return nil
}

func summarizeResourceMetrics(rm *metricdata.ResourceMetrics) string {
	if rm == nil {
		return "no metrics"
	}

	var summaries []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			summaries = append(summaries, fmt.Sprintf("%s[%s,%d points]", m.Name, metricAggregationType(m.Data), metricPointCount(m.Data)))
		}
	}
	if len(summaries) == 0 {
		return "no metrics"
	}
	return strings.Join(summaries, ", ")
}

func metricAggregationType(data metricdata.Aggregation) string {
	switch data.(type) {
	case metricdata.Sum[int64]:
		return "sum[int64]"
	case metricdata.Sum[float64]:
		return "sum[float64]"
	case metricdata.Gauge[int64]:
		return "gauge[int64]"
	case metricdata.Gauge[float64]:
		return "gauge[float64]"
	case metricdata.Histogram[int64]:
		return "histogram[int64]"
	case metricdata.Histogram[float64]:
		return "histogram[float64]"
	default:
		return fmt.Sprintf("%T", data)
	}
}

func metricPointCount(data metricdata.Aggregation) int {
	switch v := data.(type) {
	case metricdata.Sum[int64]:
		return len(v.DataPoints)
	case metricdata.Sum[float64]:
		return len(v.DataPoints)
	case metricdata.Gauge[int64]:
		return len(v.DataPoints)
	case metricdata.Gauge[float64]:
		return len(v.DataPoints)
	case metricdata.Histogram[int64]:
		return len(v.DataPoints)
	case metricdata.Histogram[float64]:
		return len(v.DataPoints)
	default:
		return 0
	}
}
