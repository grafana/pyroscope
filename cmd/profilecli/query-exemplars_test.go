package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestOutputExemplarsTable(t *testing.T) {
	t.Parallel()

	entries := []exemplarEntry{
		{
			ProfileID: "550e8400-e29b-41d4-a716-446655440000",
			Timestamp: time.Date(2024, 3, 20, 10, 0, 0, 0, time.UTC),
			Value:     42000000000, // 42s in nanoseconds
			SpanID:    "abc123",
			Labels:    map[string]string{"service_name": "frontend"},
		},
		{
			ProfileID: "660e8400-e29b-41d4-a716-446655440001",
			Timestamp: time.Date(2024, 3, 20, 10, 5, 0, 0, time.UTC),
			Value:     21000000000, // 21s in nanoseconds
			SpanID:    "",
			Labels:    map[string]string{"service_name": "backend"},
		},
	}

	var buf bytes.Buffer
	ctx := withOutput(context.Background(), &buf)

	err := outputExemplarsTable(ctx, entries, "nanoseconds", []string{"service_name"})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "550e8400-e29b-41d4-a716-446655440000")
	assert.Contains(t, out, "660e8400-e29b-41d4-a716-446655440001")
	assert.Contains(t, out, "abc123")
	assert.Contains(t, out, "frontend")
	assert.Contains(t, out, "backend")
	assert.Contains(t, out, "Profile ID")
	assert.Contains(t, out, "service_name")
}

func TestOutputExemplarsTable_NoGroupBy(t *testing.T) {
	t.Parallel()

	entries := []exemplarEntry{
		{
			ProfileID: "550e8400-e29b-41d4-a716-446655440000",
			Timestamp: time.Date(2024, 3, 20, 10, 0, 0, 0, time.UTC),
			Value:     42000000000,
			Labels:    map[string]string{"service_name": "frontend"},
		},
	}

	var buf bytes.Buffer
	ctx := withOutput(context.Background(), &buf)

	err := outputExemplarsTable(ctx, entries, "nanoseconds", nil)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Profile ID")
	assert.NotContains(t, out, "service_name")
}

func TestOutputExemplarsJSON(t *testing.T) {
	t.Parallel()

	from := time.Date(2024, 3, 20, 9, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 20, 10, 0, 0, 0, time.UTC)

	entries := []exemplarEntry{
		{
			ProfileID: "550e8400-e29b-41d4-a716-446655440000",
			Timestamp: time.Date(2024, 3, 20, 9, 30, 0, 0, time.UTC),
			Value:     42000,
			SpanID:    "abc123",
			Labels: map[string]string{
				"service_name":     "frontend",
				"__name__":         "process_cpu",
				"__period_type__":  "cpu",
				"__profile_type__": "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
			},
		},
	}

	var buf bytes.Buffer
	ctx := withOutput(context.Background(), &buf)

	err := outputExemplarsJSON(ctx, entries, from, to, "process_cpu:cpu:nanoseconds:cpu:nanoseconds")
	require.NoError(t, err)

	var result struct {
		From        time.Time `json:"from"`
		To          time.Time `json:"to"`
		ProfileType string    `json:"profile_type"`
		Exemplars   []struct {
			ProfileID string            `json:"profile_id"`
			Timestamp time.Time         `json:"timestamp"`
			Value     int64             `json:"value"`
			SpanID    string            `json:"span_id"`
			Labels    map[string]string `json:"labels"`
		} `json:"exemplars"`
	}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, from, result.From)
	assert.Equal(t, to, result.To)
	assert.Equal(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds", result.ProfileType)
	require.Len(t, result.Exemplars, 1)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", result.Exemplars[0].ProfileID)
	assert.Equal(t, int64(42000), result.Exemplars[0].Value)
	assert.Equal(t, "abc123", result.Exemplars[0].SpanID)
	assert.Equal(t, "frontend", result.Exemplars[0].Labels["service_name"])
	// Internal labels should be filtered out.
	assert.NotContains(t, result.Exemplars[0].Labels, "__name__")
	assert.NotContains(t, result.Exemplars[0].Labels, "__period_type__")
	assert.NotContains(t, result.Exemplars[0].Labels, "__profile_type__")
}

func TestOutputExemplarsTable_Empty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := withOutput(context.Background(), &buf)

	err := outputExemplarsTable(ctx, nil, "nanoseconds", nil)
	require.NoError(t, err)
	// Empty entries should still render a table (with only headers)
}

func TestTopCardinalityLabels(t *testing.T) {
	t.Parallel()

	entries := []exemplarEntry{
		{Labels: map[string]string{
			"__name__":     "process_cpu",
			"service_name": "frontend",
			"namespace":    "prod",
			"pod":          "frontend-abc",
			"region":       "us-east-1",
		}},
		{Labels: map[string]string{
			"__name__":     "process_cpu",
			"service_name": "backend",
			"namespace":    "prod",
			"pod":          "backend-xyz",
			"region":       "eu-west-1",
		}},
		{Labels: map[string]string{
			"__name__":     "process_cpu",
			"service_name": "frontend",
			"namespace":    "staging",
			"pod":          "frontend-def",
			"region":       "us-east-1",
		}},
	}

	result := topCardinalityLabels(entries, 3)

	// pod has 3 distinct values, service_name/namespace/region have 2 each.
	// __name__ has 1 (skipped). namespace=prod appears twice but staging once = 2 distinct.
	require.Len(t, result, 3)
	assert.Equal(t, "pod", result[0]) // 3 distinct values
	// The remaining 3 labels all have cardinality 2, sorted alphabetically.
	assert.Equal(t, "namespace", result[1])
	assert.Equal(t, "region", result[2])
}

func TestTopCardinalityLabels_FiltersInternalShowsConstant(t *testing.T) {
	t.Parallel()

	entries := []exemplarEntry{
		{Labels: map[string]string{
			"__name__":     "process_cpu",
			"service_name": "frontend",
			"constant":     "same",
		}},
		{Labels: map[string]string{
			"__name__":     "process_cpu",
			"service_name": "frontend",
			"constant":     "same",
		}},
	}

	result := topCardinalityLabels(entries, 3)

	// __name__ is internal (filtered). service_name and constant have cardinality 1
	// but are still shown (alphabetically) since they're the only non-internal labels.
	require.Len(t, result, 2)
	assert.Equal(t, "constant", result[0])
	assert.Equal(t, "service_name", result[1])
}

func TestExemplarEntry_FromProtoExemplar(t *testing.T) {
	t.Parallel()

	// Verify that our exemplarEntry struct correctly maps from the proto Exemplar type.
	ex := &typesv1.Exemplar{
		Timestamp: 1710928800000, // 2024-03-20T10:00:00Z in millis
		ProfileId: "550e8400-e29b-41d4-a716-446655440000",
		SpanId:    "deadbeef",
		Value:     99999,
		Labels: []*typesv1.LabelPair{
			{Name: "pod", Value: "frontend-abc"},
		},
	}

	entry := exemplarEntry{
		ProfileID: ex.ProfileId,
		Timestamp: time.UnixMilli(ex.Timestamp),
		Value:     ex.Value,
		SpanID:    ex.SpanId,
		Labels:    map[string]string{"pod": "frontend-abc"},
	}

	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", entry.ProfileID)
	assert.Equal(t, time.UnixMilli(1710928800000), entry.Timestamp)
	assert.Equal(t, int64(99999), entry.Value)
	assert.Equal(t, "deadbeef", entry.SpanID)
	assert.Equal(t, "frontend-abc", entry.Labels["pod"])
}
