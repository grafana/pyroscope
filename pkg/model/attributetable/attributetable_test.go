package attributetable

import (
	"testing"
	"unique"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

func TestTable_LookupOrAdd(t *testing.T) {
	table := New()

	// Add first key
	key1 := Key{Key: unique.Make("foo"), Value: unique.Make("bar")}
	ref1 := table.LookupOrAdd(key1)
	require.Equal(t, int64(0), ref1)

	// Add second key
	key2 := Key{Key: unique.Make("baz"), Value: unique.Make("qux")}
	ref2 := table.LookupOrAdd(key2)
	require.Equal(t, int64(1), ref2)

	// Lookup existing key
	ref1Again := table.LookupOrAdd(key1)
	require.Equal(t, ref1, ref1Again)
}

func TestTable_Refs(t *testing.T) {
	table := New()

	labels := model.Labels{
		{Name: "foo", Value: "bar"},
		{Name: "baz", Value: "qux"},
	}

	refs := table.Refs(labels, nil)
	require.Len(t, refs, 2)
	require.Equal(t, int64(0), refs[0])
	require.Equal(t, int64(1), refs[1])

	// Reuse same labels
	refs2 := table.Refs(labels, refs[:0])
	require.Len(t, refs2, 2)
	require.Equal(t, refs, refs2)
}

func TestTable_Build(t *testing.T) {
	table := New()

	key1 := Key{Key: unique.Make("service.name"), Value: unique.Make("my-service")}
	key2 := Key{Key: unique.Make("environment"), Value: unique.Make("production")}

	table.LookupOrAdd(key1)
	table.LookupOrAdd(key2)

	result := table.Build(nil)

	require.NotNil(t, result)
	require.Len(t, result.Keys, 2)
	require.Len(t, result.Values, 2)

	require.Equal(t, "service.name", result.Keys[0])
	require.Equal(t, "my-service", result.Values[0])
	require.Equal(t, "environment", result.Keys[1])
	require.Equal(t, "production", result.Values[1])
}

func TestTable_BuildReusesSlices(t *testing.T) {
	table := New()

	key1 := Key{Key: unique.Make("foo"), Value: unique.Make("bar")}
	table.LookupOrAdd(key1)

	// First build
	result1 := &queryv1.AttributeTable{
		Keys:   make([]string, 0, 10),
		Values: make([]string, 0, 10),
	}
	result1 = table.Build(result1)

	// Verify capacity is reused
	require.Equal(t, 10, cap(result1.Keys))
	require.Equal(t, 10, cap(result1.Values))
	require.Len(t, result1.Keys, 1)
	require.Len(t, result1.Values, 1)
}

func TestTable_EmptyLabels(t *testing.T) {
	table := New()

	labels := model.Labels{}
	refs := table.Refs(labels, nil)
	require.Empty(t, refs)

	result := table.Build(nil)
	require.Empty(t, result.Keys)
	require.Empty(t, result.Values)
}
