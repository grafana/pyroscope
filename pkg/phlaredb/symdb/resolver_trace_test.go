package symdb

import (
	"context"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

func sumPprofValues(p *googlev1.Profile) int64 {
	var total int64
	for _, s := range p.Sample {
		for _, v := range s.Value {
			total += v
		}
	}
	return total
}

// Test_Resolver_AddSamplesWithTraceSelectorFromParquetRow drives the trace_id
// sample filter end to end: samples are tagged with synthetic 16-byte trace IDs
// (matching, non-matching, and null) and the resolved pprof must contain only
// the values whose trace ID is in the selector.
func Test_Resolver_AddSamplesWithTraceSelectorFromParquetRow(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	samples := s.indexed[0][0].Samples
	require.NotZero(t, len(samples.StacktraceIDs))

	matching := model.TraceID{0x01}
	other := model.TraceID{0x02}

	stacktraces := make([]parquet.Value, len(samples.StacktraceIDs))
	values := make([]parquet.Value, len(samples.StacktraceIDs))
	traces := make([]parquet.Value, len(samples.StacktraceIDs))
	var wantMatching int64
	for i, sid := range samples.StacktraceIDs {
		stacktraces[i] = parquet.Int32Value(int32(sid))
		values[i] = parquet.Int64Value(int64(samples.Values[i]))
		switch i % 3 {
		case 0:
			traces[i] = parquet.FixedLenByteArrayValue(matching[:])
			if sid > 0 {
				wantMatching += int64(samples.Values[i])
			}
		case 1:
			traces[i] = parquet.FixedLenByteArrayValue(other[:])
		default:
			// No trace id on this sample (optional column null).
			traces[i] = parquet.NullValue()
		}
	}
	require.NotZero(t, wantMatching, "fixture must contain at least one matching sample")

	t.Run("matching trace filters to its samples", func(t *testing.T) {
		r := NewResolver(context.Background(), s.db)
		defer r.Release()
		r.AddSamplesWithTraceSelectorFromParquetRow(0, stacktraces, values, traces, model.TraceSelector{matching: {}})
		resolved, err := r.Pprof()
		require.NoError(t, err)
		assert.Equal(t, wantMatching, sumPprofValues(resolved))
	})

	t.Run("non-matching trace returns empty", func(t *testing.T) {
		r := NewResolver(context.Background(), s.db)
		defer r.Release()
		absent := model.TraceID{0xff}
		r.AddSamplesWithTraceSelectorFromParquetRow(0, stacktraces, values, traces, model.TraceSelector{absent: {}})
		resolved, err := r.Pprof()
		require.NoError(t, err)
		assert.Zero(t, sumPprofValues(resolved))
	})
}
