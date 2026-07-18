package docker

import (
	"context"
	"testing"
)

func TestRuntimeMetricsRejectsNonLocalRuntime(t *testing.T) {
	reader := &Reader{}
	if _, err := reader.RuntimeMetrics(context.Background(), "swarm", "demo", ""); err == nil {
		t.Fatal("expected swarm metrics to be rejected as non-local")
	}
}

func TestRuntimeSnapshotRejectsMissingReader(t *testing.T) {
	reader := &Reader{}
	if _, err := reader.RuntimeSnapshot(context.Background(), "standalone", "demo"); err == nil {
		t.Fatal("expected runtime snapshot to require a Docker client")
	}
}
