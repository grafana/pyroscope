package phlaredb

import (
	"context"
	"testing"

	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func TestSelectProfiles(t *testing.T) {
	// Test With only RowNum and stacktracePartition
	// Test with Labels
	b := newBlock(t, func() []*testhelper.ProfileBuilder { return nil })
	ctx := context.Background()

	SelectProfiles(ctx, b, nil, 0, 0, nil)
}
