/*
Copyright 2025 The Fabric Authors.
*/

package logging

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Environment variable names for OTel logging configuration.
const (
	EnvOTelEndpoint  = "FABRIC_OTEL_ENDPOINT"
	EnvOTelInsecure  = "FABRIC_OTEL_INSECURE"
	EnvOTelLogEnable = "FABRIC_OTEL_LOG_ENABLED"
)

// NewLoggerProvider creates an OTel LoggerProvider for the log bridge.
// Returns nil if configuration is missing or invalid.
func NewLoggerProvider(ctx context.Context, config OTelConfig) (log.LoggerProvider, func(), error) {
	if config.Endpoint == "" {
		return nil, func() {}, nil
	}

	// Build gRPC options
	var opts []grpc.DialOption
	if config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Create the exporter
	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(config.Endpoint),
		otlploggrpc.WithDialOption(opts...),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating OTLP log exporter: %w", err)
	}

	// Create the LoggerProvider
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	cleanup := func() {
		if err := provider.Shutdown(ctx); err != nil {
			// Log to stderr since the logger may be shutting down
			fmt.Fprintf(os.Stderr, "error shutting down log provider: %v\n", err)
		}
	}

	return provider, cleanup, nil
}

// InitOTelLogging sets up the full OTel logging pipeline.
// Returns the LoggerProvider, a cleanup function, and any error.
// If OTel logging is not configured, returns nil provider (not an error).
func InitOTelLogging(ctx context.Context, config OTelConfig) (log.LoggerProvider, func(), error) {
	// Check if OTel logging is enabled
	if !isOTelLogEnabled() {
		return nil, func() {}, nil
	}

	// Use environment variables if config is empty
	if config.Endpoint == "" {
		config.Endpoint = os.Getenv(EnvOTelEndpoint)
	}
	if config.Endpoint == "" {
		return nil, func() {}, nil
	}

	if !config.Insecure {
		config.Insecure = os.Getenv(EnvOTelInsecure) == "true"
	}

	return NewLoggerProvider(ctx, config)
}

// isOTelLogEnabled checks if OTel log bridging is enabled.
func isOTelLogEnabled() bool {
	val := os.Getenv(EnvOTelLogEnable)
	if val == "" {
		// Default to enabled if OTEL endpoint is set
		return os.Getenv(EnvOTelEndpoint) != ""
	}
	return val == "true" || val == "1" || val == "yes"
}
