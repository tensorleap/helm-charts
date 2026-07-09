package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Locks the install/upgrade default: with no --cluster-memory-gb flag the installer
// auto-detects (source "auto", so the orchestrator applies the reserve buffer); with a
// flag it is taken verbatim (source "flag", no buffer). This is what makes upgrading an
// existing platform get a sensible memory default with no operator input.
func TestDetectClusterMemory(t *testing.T) {
	t.Run("flag provided is verbatim and marked flag", func(t *testing.T) {
		bytes, source := detectClusterMemory(context.Background(), 64)
		assert.Equal(t, int64(64)*1024*1024*1024, bytes)
		assert.Equal(t, "flag", source)
	})

	t.Run("no flag auto-detects", func(t *testing.T) {
		// bytes depends on the local docker daemon (may be 0 in CI without docker);
		// the load-bearing guarantee is the source label that drives the buffer.
		_, source := detectClusterMemory(context.Background(), 0)
		assert.Equal(t, "auto", source)
	})
}
