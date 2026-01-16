package profileid

import (
	"testing"

	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestGenerateFromRequest_WithTimeNanos(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "service", Value: "test"},
	}
	profile := []byte("profile-data")

	// Same TimeNanos should produce same ID
	id1 := GenerateFromRequest("tenant", labels, profile, 1000, "")
	id2 := GenerateFromRequest("tenant", labels, profile, 1000, "")
	require.Equal(t, id1, id2, "Same TimeNanos should produce same ID")

	// Different TimeNanos should produce different ID
	id3 := GenerateFromRequest("tenant", labels, profile, 2000, "")
	require.NotEqual(t, id1, id3, "Different TimeNanos should produce different ID")
}

func TestGenerateFromRequest_WithTraceID(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "service", Value: "test"},
	}
	profile := []byte("profile-data")

	// TimeNanos=0 should fall back to traceID
	id1 := GenerateFromRequest("tenant", labels, profile, 0, "trace-123")
	id2 := GenerateFromRequest("tenant", labels, profile, 0, "trace-123")
	require.Equal(t, id1, id2, "Same traceID should produce same ID")

	// Different traceID should produce different ID
	id3 := GenerateFromRequest("tenant", labels, profile, 0, "trace-456")
	require.NotEqual(t, id1, id3, "Different traceID should produce different ID")
}

func TestGenerateFromRequest_NoTimeNoTrace(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "service", Value: "test"},
	}
	profile := []byte("profile-data")

	// Pure content hash
	id1 := GenerateFromRequest("tenant", labels, profile, 0, "")
	id2 := GenerateFromRequest("tenant", labels, profile, 0, "")
	require.Equal(t, id1, id2, "Content-only hash should be consistent")
}

func TestGenerateFromRequest_TimeOverridesTrace(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "service", Value: "test"},
	}
	profile := []byte("profile-data")

	// TimeNanos should take precedence over traceID
	id1 := GenerateFromRequest("tenant", labels, profile, 1000, "trace-123")
	id2 := GenerateFromRequest("tenant", labels, profile, 1000, "trace-456")
	require.Equal(t, id1, id2, "TimeNanos should take precedence, ignore traceID difference")
}

func TestGenerateFromRequest_LabelOrdering(t *testing.T) {
	labels1 := []*typesv1.LabelPair{
		{Name: "a", Value: "1"},
		{Name: "b", Value: "2"},
	}
	labels2 := []*typesv1.LabelPair{
		{Name: "b", Value: "2"},
		{Name: "a", Value: "1"},
	}
	profile := []byte("profile-data")

	// Different label order should produce same ID
	id1 := GenerateFromRequest("tenant", labels1, profile, 1000, "")
	id2 := GenerateFromRequest("tenant", labels2, profile, 1000, "")
	require.Equal(t, id1, id2, "Label order should not affect ID")
}

func TestGenerateFromRequest_DifferentTenants(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "service", Value: "test"},
	}
	profile := []byte("profile-data")

	// Different tenants should produce different IDs
	id1 := GenerateFromRequest("tenant1", labels, profile, 1000, "")
	id2 := GenerateFromRequest("tenant2", labels, profile, 1000, "")
	require.NotEqual(t, id1, id2, "Different tenants should produce different IDs")
}

func TestGenerateFromRequest_DifferentLabels(t *testing.T) {
	labels1 := []*typesv1.LabelPair{
		{Name: "service", Value: "test1"},
	}
	labels2 := []*typesv1.LabelPair{
		{Name: "service", Value: "test2"},
	}
	profile := []byte("profile-data")

	// Different labels should produce different IDs
	id1 := GenerateFromRequest("tenant", labels1, profile, 1000, "")
	id2 := GenerateFromRequest("tenant", labels2, profile, 1000, "")
	require.NotEqual(t, id1, id2, "Different labels should produce different IDs")
}

func TestGenerateFromRequest_DifferentContent(t *testing.T) {
	labels := []*typesv1.LabelPair{
		{Name: "service", Value: "test"},
	}

	// Different profile content should produce different IDs
	id1 := GenerateFromRequest("tenant", labels, []byte("profile-1"), 1000, "")
	id2 := GenerateFromRequest("tenant", labels, []byte("profile-2"), 1000, "")
	require.NotEqual(t, id1, id2, "Different content should produce different IDs")
}

func TestGenerateRandom(t *testing.T) {
	// Should produce different IDs
	id1 := GenerateRandom()
	id2 := GenerateRandom()
	require.NotEqual(t, id1, id2, "Random IDs should be different")
}
