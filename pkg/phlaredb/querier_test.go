package phlaredb

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	v1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/phlaredb/tsdb/index"
)

func TestQueryIndex(t *testing.T) {
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

	p, err := PostingsForMatchers(r, nil, labels.MustNewMatcher(labels.MatchRegexp, "bar", "(1|2)"))
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
