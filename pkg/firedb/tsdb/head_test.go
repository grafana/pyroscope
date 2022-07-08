package index

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"

	"github.com/grafana/fire/pkg/firedb/tsdb/index"
	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	firemodel "github.com/grafana/fire/pkg/model"
)

func TestHeadIndex(t *testing.T) {
	a, err := NewHead(32)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
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
		}()
	}
	wg.Wait()

	// Testing Matching
	total := 0
	err = a.ForMatchingProfiles([]*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
		labels.MustNewMatcher(labels.MatchRegexp, "bar", "[0-9]"),
		labels.MustNewMatcher(labels.MatchNotEqual, "buzz", "bar"),
	}, func(lbs firemodel.Labels, fp model.Fingerprint, mint, maxt int64) error {
		total++
		require.Equal(t, int64(0), mint)
		require.Equal(t, int64(9), maxt)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 10, total)

	names, err := a.LabelNames()
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "foo"}, names)

	values, err := a.LabelValues("foo")
	require.NoError(t, err)
	require.Equal(t, []string{"bar"}, values)
	values, err = a.LabelValues("bar")
	require.NoError(t, err)
	require.Equal(t, []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}, values)
}

func TestWriteRead(t *testing.T) {
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

	names, err := r.LabelNames()
	require.NoError(t, err)
	require.Equal(t, []string{"bar", "foo"}, names)

	values, err := r.LabelValues("foo")
	require.NoError(t, err)
	require.Equal(t, []string{"bar"}, values)
	values, err = r.LabelValues("bar")
	require.NoError(t, err)
	require.Equal(t, []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}, values)

	from, through := r.Bounds()
	require.Equal(t, int64(0), from)
	require.Equal(t, int64(9), through)
	p, err := r.Postings("foo", nil, "bar")
	lbls := make(firemodel.Labels, 2)
	chks := make([]index.ChunkMeta, 1)
	require.NoError(t, err)
	for p.Next() {
		fp, err := r.Series(p.At(), &lbls, &chks)
		require.NoError(t, err)
		require.Equal(t, lbls.Hash(), fp)
		require.Equal(t, "bar", lbls.Get("foo"))
		require.Equal(t, 2, len(lbls))
		require.Equal(t, 1, len(chks))
		require.Equal(t, int64(0), chks[0].MinTime)
		require.Equal(t, int64(9), chks[0].MaxTime)
	}
}
