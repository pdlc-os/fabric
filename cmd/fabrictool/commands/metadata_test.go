/*
Copyright 2026 The Fabric Authors.
*/

package commands

import (
	"testing"
)

func TestRunMetadataStatus_NoEnv(t *testing.T) {
	// With no FABRIC_METADATA_MODE set, runMetadataStatus should return 1
	// (not configured) without panicking.
	t.Setenv("FABRIC_METADATA_MODE", "")
	t.Setenv("GCE_METADATA_HOST", "")
	t.Setenv("GCE_METADATA_ROOT", "")

	code := runMetadataStatus()
	if code != 1 {
		t.Fatalf("expected exit code 1 when metadata not configured, got %d", code)
	}
}

func TestRunMetadataStatus_ConfiguredButNoServer(t *testing.T) {
	// With FABRIC_METADATA_MODE set but no server running, should report failures.
	t.Setenv("FABRIC_METADATA_MODE", "assign")
	t.Setenv("FABRIC_METADATA_PORT", "19999")
	t.Setenv("GCE_METADATA_HOST", "localhost:19999")
	t.Setenv("GCE_METADATA_ROOT", "localhost:19999")

	code := runMetadataStatus()
	if code != 1 {
		t.Fatalf("expected exit code 1 when server not running, got %d", code)
	}
}
