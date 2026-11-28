package querybackend

import (
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/model"
)

func TestSpanPresencePredicate(t *testing.T) {
	p := spanPresencePredicate{spans: model.SpanSelector{42: {}}}
	require.False(t, p.KeepValue(parquet.Value{}))
	require.True(t, p.KeepValue(parquet.Int64Value(42)))
	require.False(t, p.KeepValue(parquet.Int64Value(43)))
	// Unsigned span IDs must retain their full 64-bit representation.
	p.spans[1<<63] = struct{}{}
	require.True(t, p.KeepValue(parquet.Int64Value(-1<<63)))
}
