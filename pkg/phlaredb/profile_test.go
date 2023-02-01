package phlaredb

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	v1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/phlaredb/tsdb/index"
)

func TestIndex(t *testing.T) {
	a, err := newProfileIndex(16, newHeadMetrics(prometheus.NewRegistry()))
	require.NoError(t, err)
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
					a.Add(&v1.Profile{
						ID:                id,
						TimeNanos:         k,
						SeriesFingerprint: model.Fingerprint(lb1.Hash()),
					}, lb1, "memory")
					a.Add(&v1.Profile{
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
	ctx := testContext(t)
	fps, err := a.selectMatchingFPs(ctx, &ingestv1.SelectProfilesRequest{
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

func TestWriteRead(t *testing.T) {
	a, err := newProfileIndex(32, newHeadMetrics(prometheus.NewRegistry()))
	require.NoError(t, err)

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
			a.Add(&v1.Profile{
				ID:                id,
				TimeNanos:         k,
				SeriesFingerprint: model.Fingerprint(lb1.Hash()),
			}, lb1, "memory")
			a.Add(&v1.Profile{
				ID:                id,
				TimeNanos:         k,
				SeriesFingerprint: model.Fingerprint(lb2.Hash()),
			}, lb2, "memory")
		}
	}

	tmpFile := t.TempDir() + "/test.db"
	_, err = a.writeTo(context.Background(), tmpFile)
	require.NoError(t, err)

	r, err := index.NewFileReader(tmpFile)
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

func Test_rowRangeIter(t *testing.T) {
	for _, tc := range []struct {
		name     string
		r        *rowRange
		expected []int64
	}{
		{"empty", &rowRange{}, []int64{}},
		{"first-element", &rowRange{0, 1}, []int64{0}},
		{"first-3-elements", &rowRange{0, 3}, []int64{0, 1, 2}},
		{"empty-offset", &rowRange{10, 0}, []int64{}},
		{"one-element-offset", &rowRange{10, 1}, []int64{10}},
		{"two elements-offset", &rowRange{10, 2}, []int64{10, 11}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := rowRanges{tc.r: 0xff}.iter()
			var result = []int64{}
			for it.Next() {
				result = append(result, it.At().RowNumber())
			}
			assert.Equal(t, tc.expected, result)
		})
	}

}
