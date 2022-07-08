package tsdb

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"

	"github.com/grafana/fire/pkg/firedb/tsdb/index"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	firemodel "github.com/grafana/fire/pkg/model"
)

func TestQueryIndex(t *testing.T) {
	a, err := NewHead(32)
	require.NoError(t, err)

	for j := 0; j < 10; j++ {
		lbs := firemodel.Labels([]*commonv1.LabelPair{
			{Name: "foo", Value: "bar"},
			{Name: "bar", Value: fmt.Sprint(j)},
		})
		sort.Sort(lbs)
		for k := int64(0); k < 10; k++ {
			a.Add(k, lbs)
		}
	}

	tmpFile := t.TempDir() + "/test.db"
	err = a.WriteTo(context.Background(), tmpFile)
	require.NoError(t, err)

	r, err := index.NewFileReader(tmpFile)
	require.NoError(t, err)

	p, err := PostingsForMatchers(r, nil, labels.MustNewMatcher(labels.MatchRegexp, "bar", "(1|2)"))
	require.NoError(t, err)

	lbls := make(firemodel.Labels, 2)
	chks := make([]index.ChunkMeta, 1)
	for p.Next() {
		fp, err := r.Series(p.At(), &lbls, &chks)
		require.NoError(t, err)
		require.Equal(t, lbls.Hash(), fp)
		require.Equal(t, 2, len(lbls))

		require.Equal(t, "bar", lbls.Get("foo"))
		require.True(t, lbls.Get("bar") == "1" || lbls.Get("bar") == "2")

		require.Equal(t, 1, len(chks))
		require.Equal(t, int64(0), chks[0].MinTime)
		require.Equal(t, int64(9), chks[0].MaxTime)
	}
}
