package store

import (
	"testing"

	"github.com/grafana/pyroscope/pkg/test"
)

func TestTombstoneStore(t *testing.T) {
	_ = test.BoltDB(t)
}
