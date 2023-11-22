package phlaredb

// import (
// 	"context"
// 	"testing"

// 	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
// 	"github.com/stretchr/testify/require"
// )

// func TestSelectProfiles(t *testing.T) {
// 	// Test With only RowNum and stacktracePartition
// 	// Test with Labels
// 	b := newBlock(t, func() []*testhelper.ProfileBuilder { return nil })
// 	ctx := context.Background()

// 	it, err := SelectProfiles(ctx, b, nil, 0, 0, nil)
// 	require.NoError(t, err)

// 	require.True(t, it.Next())
// 	// it.At().
// }
