package profileid

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestGenerateFromTrace(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "service", Value: "test"},
		{Name: "region", Value: "us-east-1"},
	}
	base := GenerateFromTrace("tenant", "trace-123", labels, 1000, 0)

	tests := []struct {
		name string
		id   func() uuid.UUID
	}{
		{name: "same inputs", id: func() uuid.UUID { return GenerateFromTrace("tenant", "trace-123", labels, 1000, 0) }},
		{name: "different tenant", id: func() uuid.UUID { return GenerateFromTrace("tenant-2", "trace-123", labels, 1000, 0) }},
		{name: "different trace", id: func() uuid.UUID { return GenerateFromTrace("tenant", "trace-456", labels, 1000, 0) }},
		{name: "different timestamp", id: func() uuid.UUID { return GenerateFromTrace("tenant", "trace-123", labels, 2000, 0) }},
		{name: "different position", id: func() uuid.UUID { return GenerateFromTrace("tenant", "trace-123", labels, 1000, 1) }},
		{name: "different labels", id: func() uuid.UUID {
			return GenerateFromTrace("tenant", "trace-123", []*typesv1.LabelPair{{Name: "service", Value: "other"}}, 1000, 0)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "same inputs" {
				require.Equal(t, base, tt.id())
				return
			}
			require.NotEqual(t, base, tt.id())
		})
	}

	reversedLabels := []*typesv1.LabelPair{labels[1], labels[0]}
	require.Equal(t, base, GenerateFromTrace("tenant", "trace-123", reversedLabels, 1000, 0))
}
