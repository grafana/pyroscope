package querybackend

import (
	"context"
	"math"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	parquetquery "github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
)

func collectProfileEntries(t *testing.T, entries iter.Iterator[ProfileEntry]) []ProfileEntry {
	t.Helper()
	defer func() { require.NoError(t, entries.Close()) }()
	var result []ProfileEntry
	for entries.Next() {
		result = append(result, entries.At())
	}
	require.NoError(t, entries.Err())
	return result
}

// This exercises the actual V2 shared entry factory, not just the low-level
// Parquet scanner. The suite's Invoke tests also cover downstream reports.
func (s *testSuite) Test_ProfileEntryBatchEquivalence() {
	tested := 0
	for _, md := range s.blocks {
		for _, dataset := range md.Datasets {
			if block.DatasetFormat(dataset.Format) != block.DatasetFormat0 {
				continue
			}
			ds := block.NewDataset(dataset, block.NewObject(objstore.NewBucket(s.bucket), md))
			s.Require().NoError(ds.Open(s.ctx, block.SectionTSDB, block.SectionProfiles))
			q := &queryContext{ctx: s.ctx, ds: ds, blockContext: &blockContext{req: &request{startTime: 0, endTime: math.MaxInt64}}}
			options := [][]profileIteratorOption{
				nil,
				{withAllLabels()},
				{withFetchPartition(false), withGroupByLabels("service_name")},
				{withFetchProfileIDs(true), withAllLabels()},
				{withFetchPartition(false), withFetchProfileIDs(true), withAllLabels()},
				{withExcludeSampled(), withAllLabels()},
			}
			for _, opts := range options {
				parsed := iteratorOptsFromOptions(opts)
				var matchers []*labels.Matcher
				if parsed.excludeSampled {
					matchers = append(matchers, labels.MustNewMatcher(labels.MatchNotEqual, phlaremodel.LabelNameSampled, "true"))
				}
				selected, err := getSeries(ds.Index(), matchers, opts...)
				s.Require().NoError(err)
				want := collectProfileEntries(s.T(), scalarProfileEntryIterator(q, selected, parsed))
				actual, err := profileEntryIterator(q, opts...)
				s.Require().NoError(err)
				// Guard against a silent fallback making the equivalence test vacuous.
				s.Require().IsType(&iter.AsyncBatchIterator[ProfileEntry, ProfileEntry]{}, actual)
				got := collectProfileEntries(s.T(), actual)
				s.Require().Equal(want, got)
				if len(want) > 0 && parsed.fetchProfileIDs {
					selector, err := withProfileIDSelector(want[len(want)/2].ID)
					s.Require().NoError(err)
					idOpts := append(append([]profileIteratorOption(nil), opts...), selector)
					filtered, err := profileEntryIterator(q, idOpts...)
					s.Require().NoError(err)
					s.Require().IsType(&iter.AsyncBatchIterator[*parquetquery.IteratorResult, ProfileEntry]{}, filtered, "ID selectors must retain the ID-first plan")
					entries := collectProfileEntries(s.T(), filtered)
					s.Require().NotEmpty(entries)
					for _, entry := range entries {
						s.Require().Equal(want[len(want)/2].ID, entry.ID)
					}
				}
			}
			// No-match and narrow timestamp windows still use the batch path.
			for _, bounds := range [][2]int64{{1, 1}, {startTime.UnixNano(), startTime.UnixNano()}} {
				q.req.startTime, q.req.endTime = bounds[0], bounds[1]
				selected, err := getSeries(ds.Index(), nil)
				s.Require().NoError(err)
				want := collectProfileEntries(s.T(), scalarProfileEntryIterator(q, selected, iteratorOptsFromOptions(nil)))
				actual, err := profileEntryIterator(q)
				s.Require().NoError(err)
				s.Require().Equal(want, collectProfileEntries(s.T(), actual))
			}
			// Cancellation and early close must unwind the async adapter as well.
			q.req.startTime, q.req.endTime = 0, math.MaxInt64
			ctx, cancel := context.WithCancel(s.ctx)
			q.ctx = ctx
			cancel()
			entries, err := profileEntryIterator(q)
			s.Require().NoError(err)
			s.Require().False(entries.Next())
			s.Require().ErrorIs(entries.Err(), context.Canceled)
			s.Require().NoError(entries.Close())
			q.ctx = s.ctx
			entries, err = profileEntryIterator(q, withFetchProfileIDs(true))
			s.Require().NoError(err)
			entries.Next()
			s.Require().NoError(entries.Close())
			s.Require().NoError(ds.Close())
			tested++
		}
	}
	s.Require().Positive(tested)
}

func TestBatchProfileEntryIDSelectorFallback(t *testing.T) {
	entries, supported, err := batchProfileEntryIterator(nil, nil, iteratorOpts{profileIDSelector: []string{"id"}})
	require.NoError(t, err)
	require.False(t, supported)
	require.Nil(t, entries)
}
