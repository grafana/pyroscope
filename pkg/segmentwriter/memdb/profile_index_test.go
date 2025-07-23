package memdb

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

func TestIndex(t *testing.T) {
	a := newProfileIndex(NewHeadMetricsWithPrefix(prometheus.NewRegistry(), ""))
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				lb1 := phlaremodel.Labels([]*typesv1.LabelPair{
					{Name: "__name__", Value: "memory"},
					{Name: "__sample__type__", Value: "bytes"},
					{Name: "__profile_type__", Value: "::::"},
					{Name: "bar", Value: fmt.Sprint(j)},
				})
				sort.Sort(lb1)
				lb2 := phlaremodel.Labels([]*typesv1.LabelPair{
					{Name: "__name__", Value: "memory"},
					{Name: "__sample__type__", Value: "count"},
					{Name: "__profile_type__", Value: "::::"},
					{Name: "bar", Value: fmt.Sprint(j)},
				})
				sort.Sort(lb2)

				for k := int64(0); k < 10; k++ {
					id := uuid.New()
					a.Add(&v1.InMemoryProfile{
						ID:                id,
						TimeNanos:         k,
						SeriesFingerprint: model.Fingerprint(lb1.Hash()),
					}, lb1, "memory")
					a.Add(&v1.InMemoryProfile{
						ID:                id,
						TimeNanos:         k,
						SeriesFingerprint: model.Fingerprint(lb2.Hash()),
					}, lb2, "memory")
				}
			}
		}()
	}
	wg.Wait()

	// Testing Matching
	fps, err := selectMatchingFPs(a, &ingestv1.SelectProfilesRequest{
		LabelSelector: `memory{bar=~"[0-9]", buzz!="bar"}`,
		Type:          &typesv1.ProfileType{},
	})
	require.NoError(t, err)
	require.Len(t, fps, 20)

	names, err := a.ix.LabelNames(nil)
	require.NoError(t, err)
	require.Equal(t, []string{"__name__", "__profile_type__", "__sample__type__", "bar"}, names)

	values, err := a.ix.LabelValues("__sample__type__", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"bytes", "count"}, values)
	values, err = a.ix.LabelValues("bar", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}, values)
}

func selectMatchingFPs(pi *profilesIndex, params *ingestv1.SelectProfilesRequest) ([]model.Fingerprint, error) {
	selectors, err := parser.ParseMetricSelector(params.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to parse label selectors: "+err.Error())
	}
	if params.Type == nil {
		return nil, errors.New("no profileType given")
	}
	selectors = append(selectors, phlaremodel.SelectorFromProfileType(params.Type))

	filters, matchers := phlaredb.SplitFiltersAndMatchers(selectors)
	ids, err := pi.ix.Lookup(matchers, nil)
	if err != nil {
		return nil, err
	}

	pi.mutex.RLock()
	defer pi.mutex.RUnlock()

	// filter fingerprints that no longer exist or don't match the filters
	var idx int
outer:
	for _, fp := range ids {
		profile, ok := pi.profilesPerFP[fp]
		if !ok {
			// If a profile labels is missing here, it has already been flushed
			// and is supposed to be picked up from storage by querier
			continue
		}
		for _, filter := range filters {
			if !filter.Matches(profile.lbs.Get(filter.Name)) {
				continue outer
			}
		}

		// keep this one
		ids[idx] = fp
		idx++
	}

	return ids[:idx], nil
}

func TestWriteRead(t *testing.T) {
	a := newProfileIndex(NewHeadMetricsWithPrefix(prometheus.NewRegistry(), ""))

	for j := 0; j < 10; j++ {
		lb1 := phlaremodel.Labels([]*typesv1.LabelPair{
			{Name: "__name__", Value: "memory"},
			{Name: "__sample__type__", Value: "bytes"},
			{Name: "bar", Value: fmt.Sprint(j)},
		})
		sort.Sort(lb1)
		lb2 := phlaremodel.Labels([]*typesv1.LabelPair{
			{Name: "__name__", Value: "memory"},
			{Name: "__sample__type__", Value: "count"},
			{Name: "bar", Value: fmt.Sprint(j)},
		})
		sort.Sort(lb2)

		for k := int64(0); k < 10; k++ {
			id := uuid.New()
			a.Add(&v1.InMemoryProfile{
				ID:                id,
				TimeNanos:         k,
				SeriesFingerprint: model.Fingerprint(lb1.Hash()),
			}, lb1, "memory")
			a.Add(&v1.InMemoryProfile{
				ID:                id,
				TimeNanos:         k,
				SeriesFingerprint: model.Fingerprint(lb2.Hash()),
			}, lb2, "memory")
		}
	}

	indexData, _, err := a.Flush(context.Background())
	require.NoError(t, err)

	r, err := index.NewReader(index.RealByteSlice(indexData))
	require.NoError(t, err)

	names, err := r.LabelNames()
	require.NoError(t, err)
	require.Equal(t, []string{"__name__", "__sample__type__", "bar"}, names)

	values, err := r.LabelValues("bar")
	require.NoError(t, err)
	require.Equal(t, []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}, values)

	from, through := r.Bounds()
	require.Equal(t, int64(0), from)
	require.Equal(t, int64(9), through)
	p, err := r.Postings("__name__", nil, "memory")
	lbls := make(phlaremodel.Labels, 2)
	chks := make([]index.ChunkMeta, 1)
	require.NoError(t, err)
	for p.Next() {
		fp, err := r.Series(p.At(), &lbls, &chks)
		require.NoError(t, err)
		require.Equal(t, lbls.Hash(), fp)
		require.Equal(t, "memory", lbls.Get("__name__"))
		require.Equal(t, 3, len(lbls))
		require.Equal(t, 1, len(chks))
		require.Equal(t, int64(0), chks[0].MinTime)
		require.Equal(t, int64(9), chks[0].MaxTime)
	}
}

func TestQueryIndex(t *testing.T) {
	a := newProfileIndex(NewHeadMetricsWithPrefix(prometheus.NewRegistry(), ""))

	for j := 0; j < 10; j++ {
		lb1 := phlaremodel.Labels([]*typesv1.LabelPair{
			{Name: "__name__", Value: "memory"},
			{Name: "__sample__type__", Value: "bytes"},
			{Name: "bar", Value: fmt.Sprint(j)},
		})
		sort.Sort(lb1)
		lb2 := phlaremodel.Labels([]*typesv1.LabelPair{
			{Name: "__name__", Value: "memory"},
			{Name: "__sample__type__", Value: "count"},
			{Name: "bar", Value: fmt.Sprint(j)},
		})
		sort.Sort(lb2)

		for k := int64(0); k < 10; k++ {
			id := uuid.New()
			a.Add(&v1.InMemoryProfile{
				ID:                id,
				TimeNanos:         k,
				SeriesFingerprint: model.Fingerprint(lb1.Hash()),
			}, lb1, "memory")
			a.Add(&v1.InMemoryProfile{
				ID:                id,
				TimeNanos:         k,
				SeriesFingerprint: model.Fingerprint(lb2.Hash()),
			}, lb2, "memory")
		}
	}

	indexData, _, err := a.Flush(context.Background())
	require.NoError(t, err)

	r, err := index.NewReader(index.RealByteSlice(indexData))
	require.NoError(t, err)

	p, err := phlaredb.PostingsForMatchers(r, nil, labels.MustNewMatcher(labels.MatchRegexp, "bar", "(1|2)"))
	require.NoError(t, err)

	lbls := make(phlaremodel.Labels, 3)
	chks := make([]index.ChunkMeta, 1)
	for p.Next() {
		fp, err := r.Series(p.At(), &lbls, &chks)
		require.NoError(t, err)
		require.Equal(t, lbls.Hash(), fp)
		require.Equal(t, 3, len(lbls))

		require.Equal(t, "memory", lbls.Get("__name__"))
		require.True(t, lbls.Get("bar") == "1" || lbls.Get("bar") == "2")

		require.Equal(t, 1, len(chks))
		require.Equal(t, int64(0), chks[0].MinTime)
		require.Equal(t, int64(9), chks[0].MaxTime)
	}
}

func TestProfileTypeNames(t *testing.T) {
	a := newProfileIndex(NewHeadMetricsWithPrefix(prometheus.NewRegistry(), ""))

	for j := 0; j < 5; j++ {
		lb1 := phlaremodel.Labels([]*typesv1.LabelPair{
			{Name: "__name__", Value: "cpu"},
			{Name: phlaremodel.LabelNameProfileType, Value: fmt.Sprintf("test-profile-type-%d", j)},
		})
		sort.Sort(lb1)
		a.Add(&v1.InMemoryProfile{
			ID:                uuid.New(),
			TimeNanos:         0,
			SeriesFingerprint: model.Fingerprint(lb1.Hash()),
		}, lb1, "cpu")
	}
	names, err := a.profileTypeNames()
	require.NoError(t, err)
	require.Equal(t, []string{"test-profile-type-0", "test-profile-type-1", "test-profile-type-2", "test-profile-type-3", "test-profile-type-4"}, names)
}

func TestIndexAddOutOfOrder(t *testing.T) {
	a := newProfileIndex(NewHeadMetricsWithPrefix(prometheus.NewRegistry(), ""))

	lb1 := phlaremodel.Labels([]*typesv1.LabelPair{
		{Name: "__name__", Value: "memory"},
		{Name: "__sample__type__", Value: "bytes"},
		{Name: "bar", Value: "1"},
	})
	sort.Sort(lb1)

	lb2 := phlaremodel.Labels([]*typesv1.LabelPair{
		{Name: "__name__", Value: "memory"},
		{Name: "__sample__type__", Value: "bytes"},
		{Name: "bar", Value: "2"},
	})
	sort.Sort(lb2)

	a.Add(&v1.InMemoryProfile{
		ID:                uuid.New(),
		TimeNanos:         239,
		SeriesFingerprint: model.Fingerprint(lb2.Hash()),
	}, lb2, "memory")

	ts := []uint64{10, 20, 0}

	for _, t := range ts {
		a.Add(&v1.InMemoryProfile{
			ID:                uuid.New(),
			TimeNanos:         int64(t),
			SeriesFingerprint: model.Fingerprint(lb1.Hash()),
		}, lb1, "memory")
	}

	a.Add(&v1.InMemoryProfile{
		ID:                uuid.New(),
		TimeNanos:         238,
		SeriesFingerprint: model.Fingerprint(lb2.Hash()),
	}, lb2, "memory")

	_, profiles, err := a.Flush(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 5, len(profiles))
	expectedTS := []int64{0, 10, 20, 238, 239}
	expectedSeriesIndex := []uint32{0, 0, 0, 1, 1}
	for i := range profiles {
		assert.Equal(t, expectedTS[i], profiles[i].TimeNanos)
		assert.Equal(t, expectedSeriesIndex[i], profiles[i].SeriesIndex)
	}
}
