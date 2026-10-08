package main

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
)

func TestReplayAnonymizer(t *testing.T) {
	a := replayAnonymizer("salt")
	// SHA-256("salt" + "hello"), encoded as 64 hex characters.
	require.Equal(t, "cd31b3b98ece60cb739c0bf770b2de892ae0ad133f645513c3d83f08757a843a", a.hash("hello"))
}

func TestReplayAnonymizerLabels(t *testing.T) {
	a := replayAnonymizer("test salt")
	ls := phlaremodel.Labels{
		{Name: phlaremodel.LabelNameServiceName, Value: "private-service"},
		{Name: "private-label", Value: "private-value"},
	}
	for _, name := range []string{
		phlaremodel.LabelNameProfileType, phlaremodel.LabelNameProfileName,
		phlaremodel.LabelNameType, phlaremodel.LabelNameUnit,
		phlaremodel.LabelNamePeriodType, phlaremodel.LabelNamePeriodUnit,
	} {
		ls = append(ls, &typesv1.LabelPair{Name: name, Value: "profile-type-value"})
	}
	out := a.labels(ls)
	require.True(t, sort.IsSorted(out))
	require.Equal(t, a.hash("private-service"), out.Get(phlaremodel.LabelNameServiceName))
	require.Equal(t, a.hash("private-value"), out.Get("_"+a.hash("private-label")))
	for _, l := range ls[2:] {
		require.Equal(t, l.Value, out.Get(l.Name))
	}
	require.Equal(t, "private-service", ls[0].Value, "must not mutate iterator labels")
	require.Equal(t, ls, replayAnonymizer("").labels(ls))
	require.Equal(t, out, a.labels(ls))
	require.NotEqual(t, out, replayAnonymizer("different salt").labels(ls))

}

func TestReplayAnonymizerPartition(t *testing.T) {
	a := replayAnonymizer("salt")
	original := &symdb.Symbols{Strings: []string{"", "private-function", "private-file", "private-function"}}
	source := &replayTestSymbols{}
	p := anonymizedReplayPartition{PartitionReader: source, symbols: original}
	cache := &replaySymbols{
		source:     replayTestPartitionSource{p},
		partitions: make(map[uint64]symdb.PartitionReader),
		anonymizer: a,
	}
	first, err := cache.Partition(context.Background(), 0)
	require.NoError(t, err)
	second, err := cache.Partition(context.Background(), 0)
	require.NoError(t, err)
	require.Same(t, first.Symbols(), second.Symbols(), "reuse the anonymized table")
	require.Equal(t, []string{"", a.hash("private-function"), a.hash("private-file"), a.hash("private-function")}, first.Symbols().Strings)
	require.Equal(t, "private-function", original.Strings[1])
	first.Release()
	second.Release()
	require.Zero(t, source.releases)
	cache.Close()
	require.Equal(t, 1, source.releases)
}

type replayTestPartitionSource struct{ symdb.PartitionReader }

func (s replayTestPartitionSource) Partition(context.Context, uint64) (symdb.PartitionReader, error) {
	return s.PartitionReader, nil
}
