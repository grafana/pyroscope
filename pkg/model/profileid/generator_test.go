package profileid

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestGenerate(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "service", Value: "test"},
		{Name: "region", Value: "us-east-1"},
	}
	originalLabels := []*typesv1.LabelPair{labels[0].CloneVT(), labels[1].CloneVT()}

	tests := []struct {
		name       string
		timeNanos  int64
		traceID    string
		wantSource Source
		wantID     string // Fixed vectors pin the hash encoding and UUID version.
	}{
		{name: "timestamp takes precedence", timeNanos: 1000, traceID: "trace-123", wantSource: SourceTimestamp, wantID: "c62a98b2-f3c2-834b-b272-8fc2e634fdc7"},
		{name: "trace ID without timestamp", traceID: "trace-123", wantSource: SourceTraceID, wantID: "b5cbe639-baa2-8b65-94a8-c2f44d1387d5"},
		{name: "random without timestamp or trace ID", wantSource: SourceRandom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, source := Generate("tenant", "cpu", labels, tt.timeNanos, tt.traceID)
			second, secondSource := Generate("tenant", "cpu", labels, tt.timeNanos, tt.traceID)
			require.Equal(t, originalLabels, labels, "generation must not modify caller labels or their order")
			require.Equal(t, tt.wantSource, source)
			require.Equal(t, tt.wantSource, secondSource)
			require.Equal(t, uuid.RFC4122, first.Variant())
			require.Equal(t, uuid.RFC4122, second.Variant())
			if source == SourceRandom {
				require.Equal(t, uuid.Version(4), first.Version())
				require.Equal(t, uuid.Version(4), second.Version())
				require.NotEqual(t, first, second)
			} else {
				require.Equal(t, uuid.Version(8), first.Version())
				require.Equal(t, tt.wantID, first.String())
				require.Equal(t, first, second)
			}
		})
	}
}

func TestGenerate_DeterministicInputs(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "service", Value: "test"},
		{Name: "region", Value: "us-east-1"},
	}
	base, source := Generate("tenant", "cpu", labels, 1000, "trace-123")
	require.Equal(t, SourceTimestamp, source)

	tests := []struct {
		name string
		id   func() Source
	}{
		{name: "different tenant", id: func() Source {
			id, source := Generate("tenant-2", "cpu", labels, 1000, "trace-123")
			require.NotEqual(t, base, id)
			return source
		}},
		{name: "different profile type", id: func() Source {
			id, source := Generate("tenant", "memory", labels, 1000, "trace-123")
			require.NotEqual(t, base, id)
			return source
		}},
		{name: "different timestamp", id: func() Source {
			id, source := Generate("tenant", "cpu", labels, 2000, "trace-123")
			require.NotEqual(t, base, id)
			return source
		}},
		{name: "different labels", id: func() Source {
			id, source := Generate("tenant", "cpu", []*typesv1.LabelPair{{Name: "service", Value: "other"}}, 1000, "trace-123")
			require.NotEqual(t, base, id)
			return source
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, SourceTimestamp, tt.id())
		})
	}

	reversedLabels := []*typesv1.LabelPair{labels[1], labels[0]}
	id, _ := Generate("tenant", "cpu", reversedLabels, 1000, "trace-123")
	require.Equal(t, base, id)
}

func TestGenerate_TraceIDAffectsOnlyTraceIDs(t *testing.T) {
	labels := []*typesv1.LabelPair{{Name: "service", Value: "test"}}

	withTimestamp, _ := Generate("tenant", "cpu", labels, 1000, "trace-123")
	withDifferentTrace, _ := Generate("tenant", "cpu", labels, 1000, "trace-456")
	require.Equal(t, withTimestamp, withDifferentTrace)

	withoutTimestamp, _ := Generate("tenant", "cpu", labels, 0, "trace-123")
	withDifferentTrace, _ = Generate("tenant", "cpu", labels, 0, "trace-456")
	require.NotEqual(t, withoutTimestamp, withDifferentTrace)
}
