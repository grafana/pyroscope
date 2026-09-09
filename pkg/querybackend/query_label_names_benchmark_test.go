package querybackend

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/tsdb/index"
)

type legacyLabelNamesReader struct {
	phlaredb.IndexReader
}

func TestLabelNamesForMatchersBatched(t *testing.T) {
	reader := buildLabelNamesTestIndex(t, 10)
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "service_name", "api"),
	}

	got, err := labelNamesForMatchers(reader, matchers)
	require.NoError(t, err)

	want, err := labelNamesForMatchers(legacyLabelNamesReader{reader}, matchers)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, []string{"instance", "region", "service_name"}, got)
}

func BenchmarkLabelNamesForMatchers(b *testing.B) {
	reader := buildLabelNamesTestIndex(b, 50_000)
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchRegexp, "service_name", ".*"),
	}

	for _, benchmark := range []struct {
		name   string
		reader phlaredb.IndexReader
	}{
		{name: "per_series", reader: legacyLabelNamesReader{reader}},
		{name: "batched", reader: reader},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := labelNamesForMatchers(benchmark.reader, matchers)
				require.NoError(b, err)
			}
		})
	}
}

func buildLabelNamesTestIndex(tb testing.TB, numSeries int) *index.Reader {
	tb.Helper()

	path := filepath.Join(tb.TempDir(), index.IndexFilename)
	w, err := index.NewWriter(tb.Context(), path)
	require.NoError(tb, err)

	symbols := map[string]struct{}{
		"instance": {}, "region": {}, "service_name": {},
		"api": {}, "worker": {}, "us-east": {}, "us-west": {},
	}
	for i := 0; i < numSeries; i++ {
		symbols[fmt.Sprintf("instance-%05d", i)] = struct{}{}
	}
	orderedSymbols := make([]string, 0, len(symbols))
	for symbol := range symbols {
		orderedSymbols = append(orderedSymbols, symbol)
	}
	sort.Strings(orderedSymbols)
	for _, symbol := range orderedSymbols {
		require.NoError(tb, w.AddSymbol(symbol))
	}

	for i := 0; i < numSeries; i++ {
		service := "api"
		if i%2 == 1 {
			service = "worker"
		}
		series := phlaremodel.LabelsFromStrings(
			"instance", fmt.Sprintf("instance-%05d", i),
			"region", []string{"us-east", "us-west"}[i%2],
			"service_name", service,
		)
		require.NoError(tb, w.AddSeries(storage.SeriesRef(i), series, prommodel.Fingerprint(series.Hash())))
	}
	require.NoError(tb, w.Close())

	reader, err := index.NewFileReader(path)
	require.NoError(tb, err)
	tb.Cleanup(func() { require.NoError(tb, reader.Close()) })
	return reader
}
