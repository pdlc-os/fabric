## Add logs with event data associated with spans for hooks

Your observation about correlating logs and spans is excellent. Emitting logs for those hooks alongside the spans would indeed work well and is a
  best-practice pattern for high-fidelity observability.


  Here’s why this approach is beneficial and how it integrates with the current design:


  1. How the Correlation Works
  OpenTelemetry achieves this correlation by sharing the trace_id and span_id between the two signals. In the fabric-agent codebase, we already have the
  building blocks:
   - Spans are created in pkg/fabrictool/hooks/handlers/telemetry.go.
   - Logs would be emitted using slog.InfoContext(ctx, ...), where ctx contains the active span.
   - The OTel Log Bridge (which we've already started implementing in pkg/util/logging/otel.go) automatically extracts those IDs and attaches them to the log
     records before sending them to the cloud.


  2. Why use both Spans and Logs?
  While it might seem redundant, they serve different purposes:
   - Spans (The "Shape"): Excellent for visualizing the timeline, hierarchy (parent-child), and latency. They tell you how long a tool took to run.
   - Logs (The "Story"): Better for capturing large amounts of data. For example, a tool's full stdout output might be too large for a span attribute (which
     backends often truncate), but it’s perfect for a log message.


  3. Practical Benefits for Fabric
  If we added correlated logs to the hooks:
   * GCP Console Integration: When viewing a trace waterfall for an agent session, you could click on a tool_call span and immediately see only the logs
     associated with that specific execution.
   * Debugging Tool Failures: You could keep the span "clean" (just showing success/failure and duration) and put the full stack trace or error dump into a
     correlated log record.
   * Privacy Filtering: It's often easier to apply different retention or masking rules to logs than to spans in cloud backends.


  4. Implementation Direction
  The TelemetryHandler currently only has a tracer. To support your suggestion, we would:
   1. Add a logger *slog.Logger to the TelemetryHandler struct.
   2. Initialize this logger with the otelslog handler.
   3. In startSpan or endSpan, call h.logger.InfoContext(inProgress.ctx, "event detail", ...) to emit the correlated log.


  This would effectively bridge the gap between "point-in-time" events and the "duration-based" spans we are currently using.

  ## Include a proper metrics pipeline

  ✦ Yes, it is definitely possible and would be highly valuable. Adding native OTel metric support to the pipeline would allow you to create sophisticated
  dashboards (e.g., token usage trends over time, model cost distribution, harness performance comparisons) in platforms like Google Cloud Monitoring or
  Honeycomb.


  Here is how that would be implemented within the current Fabric architecture:


  1. Extending the Pipeline (Fabrictool)
  Currently, the Pipeline in fabrictool is primarily a Trace Forwarder. To support counters, we would:
   * Add a Metric Receiver: Implement the OTLP MetricServiceServer in pkg/fabrictool/telemetry/receiver.go to listen for metrics from harnesses that support
     OTel naturally (like Codex or OpenCode).
   * Add a Metric Exporter: Update exporter.go to forward these metric protos to the cloud endpoint using the OTLP Metric protocol.
   * Instrumentation (Harness Logic): For harnesses that don't emit OTel metrics natively (like Gemini CLI), the TelemetryHandler would use the OTel Metrics
     SDK to create and increment counters (e.g., gen_ai.tokens.input) using the data parsed from the session files.


  2. Dimension/Label Injection (Resource Attributes)
  To achieve the "complex aggregations" you mentioned, we would decorate these metrics with a consistent set of labels at the fabrictool level:
   * `fabric.harness`: (e.g., gemini, claude, codex)
   * `fabric.agent_id`: For correlating metrics back to a specific agent.
   * `fabric.grove_id`: For aggregating usage by project or group.
   * `model`: The specific model name (e.g., gemini-2.0-flash).
   * `status`: (e.g., success, error) for reliability tracking.


  3. Benefits of Counters vs. Span Attributes

  ┌───────────────┬─────────────────────────────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────
  ┐
  │ Feature       │ Span Attributes (Current)                                   │ OTel Counters (Proposed)
  │
  ├───────────────┼─────────────────────────────────────────────────────────────┼────────────────────────────────────────────────────────────────────────────
  ┤
  │ Aggregation   │ Difficult (requires "log-based metrics" or heavy querying). │ Native and fast. Easy to sum "total tokens yesterday."
  │
  │ Slicing       │ Limited by search indexing in trace backends.               │ Extremely efficient across many dimensions (harness, model, user).
  │
  │ Cost Tracking │ Manual calculation per session.                             │ Can use rate functions to estimate real-time burn rates across all agents.
  │
  │ Alerting      │ Not ideal for threshold alerts.                             │ Perfect for "Alert if token usage exceeds $X/hour."
  │
  └───────────────┴─────────────────────────────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────
  ┘



  4. Implementation Path
  To move from the current trace-only pipeline to a full observability suite:
   1. Refactor `telemetry.Pipeline` to be multi-signal (Traces + Metrics).
   2. Initialize a MeterProvider in fabrictool (similar to how the TracerProvider is handled).
   3. Update `TelemetryHandler.Handle()` to emit metrics in addition to spans when a session or model turn ends.


  This would transform Fabric from a simple agent orchestrator into a platform that provides deep, actionable operational intelligence.


  ## raw otel data-writer

  Until we get a picture of the dialect specific otel data, we will want to have the option of writing a raw capture of all data sent from the different harnesses, so that we can use that data to refine the dialect adapters

  ## generic template enablement

  While we have a large range of harness specific env vars, and a number of fabric specific telemetry settings, we will want to have a set of defaults (mostly on) that can be gated with a simple higher level 'enableTelemetry' type setting in fabric-config