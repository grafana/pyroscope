package heapanalyzer

import (
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/gocore"
)

// object represents a heap object
type object struct {
	addr   core.Address
	size   int64
	typ    *gocore.Type
	repeat int64
}
