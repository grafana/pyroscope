package phlaredb

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingesterv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func TestCompact(t *testing.T) {
	ctx := context.Background()
	b := newBlock(t, func() []*testhelper.ProfileBuilder {
		return []*testhelper.ProfileBuilder{
			testhelper.NewProfileBuilder(int64(time.Second*1)).
				CPUProfile().
				WithLabels(
					"job", "a",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(time.Second*2)).
				CPUProfile().
				WithLabels(
					"job", "b",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(time.Second*3)).
				CPUProfile().
				WithLabels(
					"job", "c",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
		}
	})
	dst := t.TempDir()
	compacted, err := Compact(ctx, []BlockReader{b, b, b, b}, dst)
	require.NoError(t, err)
	require.Equal(t, uint64(3), compacted.Stats.NumProfiles)
	require.Equal(t, uint64(3), compacted.Stats.NumSamples)
	require.Equal(t, uint64(3), compacted.Stats.NumSeries)
	require.Equal(t, model.TimeFromUnix(1), compacted.MinTime)
	require.Equal(t, model.TimeFromUnix(3), compacted.MaxTime)
	querier := blockQuerierFromMeta(t, dst, compacted)

	matchAll := &ingesterv1.SelectProfilesRequest{
		LabelSelector: "{}",
		Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
		Start:         0,
		End:           40000,
	}
	it, err := querier.SelectMatchingProfiles(ctx, matchAll)
	require.NoError(t, err)
	series, err := querier.MergeByLabels(ctx, it, nil, "job")
	require.NoError(t, err)
	require.Equal(t, []*typesv1.Series{
		{Labels: phlaremodel.LabelsFromStrings("job", "a"), Points: []*typesv1.Point{{Value: float64(1), Timestamp: int64(1000)}}},
		{Labels: phlaremodel.LabelsFromStrings("job", "b"), Points: []*typesv1.Point{{Value: float64(1), Timestamp: int64(2000)}}},
		{Labels: phlaremodel.LabelsFromStrings("job", "c"), Points: []*typesv1.Point{{Value: float64(1), Timestamp: int64(3000)}}},
	}, series)

	it, err = querier.SelectMatchingProfiles(ctx, matchAll)
	require.NoError(t, err)
	res, err := querier.MergeByStacktraces(ctx, it)
	require.NoError(t, err)
	require.NotNil(t, res)

	expected := new(phlaremodel.Tree)
	expected.InsertStack(3, "baz", "bar", "foo")
	require.Equal(t, expected.String(), res.String())
}

func TestCompactWithDownsampling(t *testing.T) {
	ctx := context.Background()
	b := newBlock(t, func() []*testhelper.ProfileBuilder {
		return []*testhelper.ProfileBuilder{
			testhelper.NewProfileBuilder(int64(time.Hour-time.Minute)).
				CPUProfile().
				WithLabels(
					"job", "a",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(time.Hour+time.Minute)).
				CPUProfile().
				WithLabels(
					"job", "b",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(time.Hour+6*time.Minute)).
				CPUProfile().
				WithLabels(
					"job", "c",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
		}
	})
	dst := t.TempDir()
	b.meta.Compaction.Level = 2
	compacted, err := Compact(ctx, []BlockReader{b, b, b, b}, dst)
	require.NoError(t, err)
	require.Equal(t, uint64(3), compacted.Stats.NumProfiles)
	require.Equal(t, uint64(3), compacted.Stats.NumSamples)
	require.Equal(t, uint64(3), compacted.Stats.NumSeries)
	require.Equal(t, model.Time((time.Hour - time.Minute).Milliseconds()), compacted.MinTime)
	require.Equal(t, model.Time((time.Hour + 6*time.Minute).Milliseconds()), compacted.MaxTime)

	for _, f := range []*block.File{
		compacted.FileByRelPath("profiles_5m_sum.parquet"),
		compacted.FileByRelPath("profiles_1h_sum.parquet"),
	} {
		require.NotNil(t, f)
		assert.NotZero(t, f.SizeBytes)
	}

	querier := blockQuerierFromMeta(t, dst, compacted)
	matchAll := &ingesterv1.SelectProfilesRequest{
		LabelSelector: "{}",
		Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
		Start:         0,
		End:           (time.Hour + 7*time.Minute - time.Millisecond).Milliseconds(),
	}
	it, err := querier.SelectMatchingProfiles(ctx, matchAll)
	require.NoError(t, err)
	series, err := querier.MergeByLabels(ctx, it, nil, "job")
	require.NoError(t, err)
	require.Equal(t, []*typesv1.Series{
		{Labels: phlaremodel.LabelsFromStrings("job", "a"), Points: []*typesv1.Point{{Value: float64(1), Timestamp: (time.Hour - time.Minute).Milliseconds()}}},
		{Labels: phlaremodel.LabelsFromStrings("job", "b"), Points: []*typesv1.Point{{Value: float64(1), Timestamp: (time.Hour + time.Minute).Milliseconds()}}},
		{Labels: phlaremodel.LabelsFromStrings("job", "c"), Points: []*typesv1.Point{{Value: float64(1), Timestamp: (time.Hour + 6*time.Minute).Milliseconds()}}},
	}, series)

	it, err = querier.SelectMatchingProfiles(ctx, matchAll)
	require.NoError(t, err)
	res, err := querier.MergeByStacktraces(ctx, it)
	require.NoError(t, err)
	require.NotNil(t, res)

	expected := new(phlaremodel.Tree)
	expected.InsertStack(3, "baz", "bar", "foo")
	require.Equal(t, expected.String(), res.String())

	res, err = querier.SelectMergeByStacktraces(ctx, matchAll)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, expected.String(), res.String())
	assert.False(t, querier.metrics.profileTableAccess.DeleteLabelValues(""))
	assert.True(t, querier.metrics.profileTableAccess.DeleteLabelValues("profiles_5m_sum.parquet"))
	assert.True(t, querier.metrics.profileTableAccess.DeleteLabelValues("profiles_1h_sum.parquet"))
	assert.True(t, querier.metrics.profileTableAccess.DeleteLabelValues("profiles.parquet"))
}

func TestCompactWithSplitting(t *testing.T) {
	ctx := context.Background()

	b1 := newBlock(t, func() []*testhelper.ProfileBuilder {
		return append(
			profileSeriesGenerator(t, time.Unix(1, 0), time.Unix(10, 0), time.Second, "job", "a"),
			profileSeriesGenerator(t, time.Unix(11, 0), time.Unix(20, 0), time.Second, "job", "b")...,
		)
	})
	b2 := newBlock(t, func() []*testhelper.ProfileBuilder {
		return append(
			append(
				append(
					profileSeriesGenerator(t, time.Unix(1, 0), time.Unix(10, 0), time.Second, "job", "c"),
					profileSeriesGenerator(t, time.Unix(11, 0), time.Unix(20, 0), time.Second, "job", "d")...,
				), profileSeriesGenerator(t, time.Unix(1, 0), time.Unix(10, 0), time.Second, "job", "a")...,
			),
			profileSeriesGenerator(t, time.Unix(11, 0), time.Unix(20, 0), time.Second, "job", "b")...,
		)
	})
	dst := t.TempDir()
	compacted, err := CompactWithSplitting(ctx, CompactWithSplittingOpts{
		Src:                []BlockReader{b1, b2, b2, b1},
		Dst:                dst,
		SplitCount:         16,
		StageSize:          8,
		SplitBy:            SplitByFingerprint,
		DownsamplerEnabled: true,
		Logger:             log.NewNopLogger(),
	})
	require.NoError(t, err)

	require.NoDirExists(t, filepath.Join(dst, symdb.DefaultDirName))

	// 4 shards one per series.
	require.Equal(t, 4, len(compacted))
	require.Equal(t, "1_of_16", compacted[0].Labels[sharding.CompactorShardIDLabel])
	require.Equal(t, "6_of_16", compacted[1].Labels[sharding.CompactorShardIDLabel])
	require.Equal(t, "7_of_16", compacted[2].Labels[sharding.CompactorShardIDLabel])
	require.Equal(t, "14_of_16", compacted[3].Labels[sharding.CompactorShardIDLabel])

	require.Equal(t, model.TimeFromUnix(1), compacted[1].MinTime)
	require.Equal(t, model.TimeFromUnix(20), compacted[1].MaxTime)

	// We first verify we have all series and timestamps across querying all blocks.
	queriers := make(Queriers, len(compacted))
	for i, blk := range compacted {
		queriers[i] = blockQuerierFromMeta(t, dst, blk)
	}

	err = queriers.Open(context.Background())
	require.NoError(t, err)
	matchAll := &ingesterv1.SelectProfilesRequest{
		LabelSelector: "{}",
		Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
		Start:         0,
		End:           40000,
	}
	it, err := queriers.SelectMatchingProfiles(context.Background(), matchAll)
	require.NoError(t, err)

	seriesMap := make(map[model.Fingerprint]lo.Tuple2[phlaremodel.Labels, []model.Time])
	for it.Next() {
		r := it.At()
		seriesMap[r.Fingerprint()] = lo.T2(r.Labels().WithoutPrivateLabels(), append(seriesMap[r.Fingerprint()].B, r.Timestamp()))
	}
	require.NoError(t, it.Err())
	require.NoError(t, it.Close())
	series := lo.Values(seriesMap)
	sort.Slice(series, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(series[i].A, series[j].A) < 0
	})
	require.Equal(t, []lo.Tuple2[phlaremodel.Labels, []model.Time]{
		lo.T2(phlaremodel.LabelsFromStrings("job", "a"),
			generateTimes(t, model.TimeFromUnix(1), model.TimeFromUnix(10)),
		),
		lo.T2(phlaremodel.LabelsFromStrings("job", "b"),
			generateTimes(t, model.TimeFromUnix(11), model.TimeFromUnix(20)),
		),
		lo.T2(phlaremodel.LabelsFromStrings("job", "c"),
			generateTimes(t, model.TimeFromUnix(1), model.TimeFromUnix(10)),
		),
		lo.T2(phlaremodel.LabelsFromStrings("job", "d"),
			generateTimes(t, model.TimeFromUnix(11), model.TimeFromUnix(20)),
		),
	}, series)

	// Then we query 2 different shards and verify we have a subset of series.
	it, err = queriers[0].SelectMatchingProfiles(ctx, matchAll)
	require.NoError(t, err)
	seriesResult, err := queriers[0].MergeByLabels(context.Background(), it, nil, "job")
	require.NoError(t, err)
	require.Equal(t,
		[]*typesv1.Series{
			{
				Labels: phlaremodel.LabelsFromStrings("job", "a"),
				Points: generatePoints(t, model.TimeFromUnix(1), model.TimeFromUnix(10)),
			},
		}, seriesResult)

	it, err = queriers[1].SelectMatchingProfiles(ctx, matchAll)
	require.NoError(t, err)
	seriesResult, err = queriers[1].MergeByLabels(context.Background(), it, nil, "job")
	require.NoError(t, err)
	require.Equal(t,
		[]*typesv1.Series{
			{
				Labels: phlaremodel.LabelsFromStrings("job", "b"),
				Points: generatePoints(t, model.TimeFromUnix(11), model.TimeFromUnix(20)),
			},
		}, seriesResult)

	// Finally test some stacktraces resolution.
	it, err = queriers[1].SelectMatchingProfiles(ctx, matchAll)
	require.NoError(t, err)
	res, err := queriers[1].MergeByStacktraces(ctx, it)
	require.NoError(t, err)

	expected := new(phlaremodel.Tree)
	expected.InsertStack(10, "baz", "bar", "foo")
	require.Equal(t, expected.String(), res.String())
}

// nolint:unparam
func profileSeriesGenerator(t *testing.T, from, through time.Time, interval time.Duration, lbls ...string) []*testhelper.ProfileBuilder {
	t.Helper()
	var builders []*testhelper.ProfileBuilder
	for ts := from; ts.Before(through) || ts.Equal(through); ts = ts.Add(interval) {
		builders = append(builders,
			testhelper.NewProfileBuilder(ts.UnixNano()).
				CPUProfile().
				WithLabels(
					lbls...,
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1))
	}
	return builders
}

func generatePoints(t *testing.T, from, through model.Time) []*typesv1.Point {
	t.Helper()
	var points []*typesv1.Point
	for ts := from; ts.Before(through) || ts.Equal(through); ts = ts.Add(time.Second) {
		points = append(points, &typesv1.Point{Timestamp: int64(ts), Value: 1})
	}
	return points
}

func generateTimes(t *testing.T, from, through model.Time) []model.Time {
	t.Helper()
	var times []model.Time
	for ts := from; ts.Before(through) || ts.Equal(through); ts = ts.Add(time.Second) {
		times = append(times, ts)
	}
	return times
}

func TestProfileRowIterator(t *testing.T) {
	b := newBlock(t, func() []*testhelper.ProfileBuilder {
		return []*testhelper.ProfileBuilder{
			testhelper.NewProfileBuilder(int64(1)).
				CPUProfile().
				WithLabels(
					"job", "a",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(2)).
				CPUProfile().
				WithLabels(
					"job", "b",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(3)).
				CPUProfile().
				WithLabels(
					"job", "c",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
		}
	})

	it, err := newProfileRowIterator(b)
	require.NoError(t, err)

	assert.True(t, it.Next())
	require.Equal(t, it.At().labels.WithoutPrivateLabels(), phlaremodel.Labels{
		&typesv1.LabelPair{Name: "job", Value: "a"},
	})
	require.Equal(t, it.At().timeNanos, int64(1))

	assert.True(t, it.Next())
	require.Equal(t, it.At().labels.WithoutPrivateLabels(), phlaremodel.Labels{
		&typesv1.LabelPair{Name: "job", Value: "b"},
	})
	require.Equal(t, it.At().timeNanos, int64(2))

	assert.True(t, it.Next())
	require.Equal(t, it.At().labels.WithoutPrivateLabels(), phlaremodel.Labels{
		&typesv1.LabelPair{Name: "job", Value: "c"},
	})
	require.Equal(t, it.At().timeNanos, int64(3))

	assert.False(t, it.Next())
	require.NoError(t, it.Err())
	require.NoError(t, it.Close())
}

func TestMergeRowProfileIterator(t *testing.T) {
	type profile struct {
		timeNanos int64
		labels    phlaremodel.Labels
	}

	a, b, c := phlaremodel.Labels{
		&typesv1.LabelPair{Name: "job", Value: "a"},
	}, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "job", Value: "b"},
	}, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "job", Value: "c"},
	}

	for _, tc := range []struct {
		name     string
		in       [][]profile
		expected []profile
	}{
		{
			name: "only duplicates",
			in: [][]profile{
				{
					{timeNanos: 1, labels: a}, {timeNanos: 2, labels: b}, {timeNanos: 3, labels: c},
				},
				{
					{timeNanos: 1, labels: a}, {timeNanos: 2, labels: b}, {timeNanos: 3, labels: c},
				},
				{
					{timeNanos: 1, labels: a}, {timeNanos: 2, labels: b}, {timeNanos: 3, labels: c},
				},
				{
					{timeNanos: 1, labels: a}, {timeNanos: 2, labels: b}, {timeNanos: 3, labels: c},
				},
			},
			expected: []profile{
				{timeNanos: 1, labels: a}, {timeNanos: 2, labels: b}, {timeNanos: 3, labels: c},
			},
		},
		{
			name: "missing some",
			in: [][]profile{
				{
					{timeNanos: 2, labels: b}, {timeNanos: 3, labels: c}, {timeNanos: 4, labels: c},
				},
				{
					{timeNanos: 1, labels: a},
				},
				{
					{timeNanos: 2, labels: b}, {timeNanos: 3, labels: c},
				},
			},
			expected: []profile{
				{timeNanos: 1, labels: a}, {timeNanos: 2, labels: b}, {timeNanos: 3, labels: c}, {timeNanos: 4, labels: c},
			},
		},
		{
			name: "no duplicates",
			in: [][]profile{
				{
					{timeNanos: 2, labels: b},
				},
				{
					{timeNanos: 1, labels: a},
				},
				{
					{timeNanos: 3, labels: c},
				},
			},
			expected: []profile{
				{timeNanos: 1, labels: a}, {timeNanos: 2, labels: b}, {timeNanos: 3, labels: c},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocks := make([]BlockReader, len(tc.in))
			for i, profiles := range tc.in {
				blocks[i] = newBlock(t, func() []*testhelper.ProfileBuilder {
					var builders []*testhelper.ProfileBuilder
					for _, p := range profiles {
						prof := testhelper.NewProfileBuilder(p.timeNanos).
							CPUProfile().ForStacktraceString("foo").AddSamples(1)
						for _, l := range p.labels {
							prof.WithLabels(l.Name, l.Value)
						}
						builders = append(builders, prof)
					}
					return builders
				})
			}
			it, err := newMergeRowProfileIterator(blocks)
			require.NoError(t, err)
			actual := []profile{}
			for it.Next() {
				actual = append(actual, profile{
					timeNanos: it.At().timeNanos,
					labels:    it.At().labels.WithoutPrivateLabels(),
				})
				require.Equal(t, model.Fingerprint(it.At().labels.Hash()), it.At().fp)
			}
			require.NoError(t, it.Err())
			require.NoError(t, it.Close())
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestSeriesRewriter(t *testing.T) {
	type profile struct {
		timeNanos int64
		labels    phlaremodel.Labels
	}

	in := []profile{
		{1, phlaremodel.LabelsFromStrings("job", "a")},
		{2, phlaremodel.LabelsFromStrings("job", "a")},
		{3, phlaremodel.LabelsFromStrings("job", "a")},
		{2, phlaremodel.LabelsFromStrings("job", "b")},
		{1, phlaremodel.LabelsFromStrings("job", "c")},
		{2, phlaremodel.LabelsFromStrings("job", "c")},
	}

	blk := newBlock(t, func() []*testhelper.ProfileBuilder {
		var builders []*testhelper.ProfileBuilder
		for _, p := range in {
			prof := testhelper.NewProfileBuilder(p.timeNanos).
				CPUProfile().ForStacktraceString("foo").AddSamples(1)
			for _, l := range p.labels {
				prof.WithLabels(l.Name, l.Value)
			}
			builders = append(builders, prof)
		}
		return builders
	})
	rows, err := newProfileRowIterator(blk)
	require.NoError(t, err)
	path := t.TempDir()
	filePath := filepath.Join(path, block.IndexFilename)
	idxw := newIndexRewriter(path)
	seriesIdx := []uint32{}
	for rows.Next() {
		r := rows.At()
		require.NoError(t, idxw.ReWriteRow(r))
		seriesIdx = append(seriesIdx, r.row.SeriesIndex())
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	require.Equal(t, []uint32{0, 0, 0, 1, 2, 2}, seriesIdx)

	err = idxw.Close(context.Background())
	require.NoError(t, err)

	idxr, err := index.NewFileReader(filePath)
	require.NoError(t, err)
	defer idxr.Close()

	k, v := index.AllPostingsKey()
	p, err := idxr.Postings(k, nil, v)
	require.NoError(t, err)

	chunks := make([]index.ChunkMeta, 1)
	var lbs phlaremodel.Labels

	require.True(t, p.Next())
	fp, err := idxr.Series(p.At(), &lbs, &chunks)
	require.NoError(t, err)
	require.Equal(t, model.Fingerprint(lbs.Hash()), model.Fingerprint(fp))
	require.Equal(t, lbs.WithoutPrivateLabels(), phlaremodel.LabelsFromStrings("job", "a"))
	require.Equal(t, []index.ChunkMeta{{
		SeriesIndex: 0,
		MinTime:     int64(1),
		MaxTime:     int64(3),
	}}, chunks)

	require.True(t, p.Next())
	fp, err = idxr.Series(p.At(), &lbs, &chunks)
	require.NoError(t, err)
	require.Equal(t, model.Fingerprint(lbs.Hash()), model.Fingerprint(fp))
	require.Equal(t, lbs.WithoutPrivateLabels(), phlaremodel.LabelsFromStrings("job", "b"))
	require.Equal(t, []index.ChunkMeta{{
		SeriesIndex: 1,
		MinTime:     int64(2),
		MaxTime:     int64(2),
	}}, chunks)

	require.True(t, p.Next())
	fp, err = idxr.Series(p.At(), &lbs, &chunks)
	require.NoError(t, err)
	require.Equal(t, model.Fingerprint(lbs.Hash()), model.Fingerprint(fp))
	require.Equal(t, lbs.WithoutPrivateLabels(), phlaremodel.LabelsFromStrings("job", "c"))
	require.Equal(t, []index.ChunkMeta{{
		SeriesIndex: 2,
		MinTime:     int64(1),
		MaxTime:     int64(2),
	}}, chunks)
}

func TestCompactOldBlock(t *testing.T) {
	meta, err := block.ReadMetaFromDir("./testdata/01HD3X85G9BGAG4S3TKPNMFG4Z")
	require.NoError(t, err)
	dst := t.TempDir()
	ctx := context.Background()
	t.Log(meta)
	bkt, err := client.NewBucket(ctx, client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: "./testdata/",
			},
		},
	}, "test")
	require.NoError(t, err)
	br := NewSingleBlockQuerierFromMeta(context.Background(), bkt, meta)
	require.NoError(t, br.Open(ctx))
	_, err = CompactWithSplitting(ctx, CompactWithSplittingOpts{
		Src:                []BlockReader{br},
		Dst:                dst,
		SplitCount:         2,
		StageSize:          0,
		SplitBy:            SplitByFingerprint,
		DownsamplerEnabled: true,
	})
	require.NoError(t, err)
}

func TestFlushMeta(t *testing.T) {
	b := newBlock(t, func() []*testhelper.ProfileBuilder {
		return []*testhelper.ProfileBuilder{
			testhelper.NewProfileBuilder(int64(time.Second*1)).
				CPUProfile().
				WithLabels(
					"job", "a",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(time.Second*2)).
				CPUProfile().
				WithLabels(
					"job", "b",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
			testhelper.NewProfileBuilder(int64(time.Second*3)).
				CPUProfile().
				WithLabels(
					"job", "c",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
		}
	})

	require.Equal(t, []ulid.ULID{b.Meta().ULID}, b.Meta().Compaction.Sources)
	require.Equal(t, 1, b.Meta().Compaction.Level)
	require.Equal(t, false, b.Meta().Compaction.Deletable)
	require.Equal(t, false, b.Meta().Compaction.Failed)
	require.Equal(t, []string(nil), b.Meta().Compaction.Hints)
	require.Equal(t, []block.BlockDesc(nil), b.Meta().Compaction.Parents)
	require.Equal(t, block.MetaVersion3, b.Meta().Version)
	require.Equal(t, model.Time(1000), b.Meta().MinTime)
	require.Equal(t, model.Time(3000), b.Meta().MaxTime)
	require.Equal(t, uint64(3), b.Meta().Stats.NumSeries)
	require.Equal(t, uint64(3), b.Meta().Stats.NumSamples)
	require.Equal(t, uint64(3), b.Meta().Stats.NumProfiles)
	require.Len(t, b.Meta().Files, 8)
	require.Equal(t, "index.tsdb", b.Meta().Files[0].RelPath)
	require.Equal(t, "profiles.parquet", b.Meta().Files[1].RelPath)
	require.Equal(t, "symbols/functions.parquet", b.Meta().Files[2].RelPath)
	require.Equal(t, "symbols/index.symdb", b.Meta().Files[3].RelPath)
	require.Equal(t, "symbols/locations.parquet", b.Meta().Files[4].RelPath)
	require.Equal(t, "symbols/mappings.parquet", b.Meta().Files[5].RelPath)
	require.Equal(t, "symbols/stacktraces.symdb", b.Meta().Files[6].RelPath)
	require.Equal(t, "symbols/strings.parquet", b.Meta().Files[7].RelPath)
}

func newBlock(t testing.TB, generator func() []*testhelper.ProfileBuilder) *singleBlockQuerier {
	t.Helper()
	dir := t.TempDir()
	ctx := phlarecontext.WithLogger(context.Background(), log.NewNopLogger())
	h, err := NewHead(ctx, Config{
		DataPath:         dir,
		MaxBlockDuration: 24 * time.Hour,
		Parquet: &ParquetConfig{
			MaxBufferRowCount: 10,
		},
	}, NoLimit)
	require.NoError(t, err)

	// ingest.
	for _, p := range generator() {
		require.NoError(t, h.Ingest(ctx, p.Profile, p.UUID, p.Labels...))
	}

	require.NoError(t, h.Flush(ctx))
	require.NoError(t, h.Move())

	bkt, err := client.NewBucket(ctx, client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: dir,
			},
		},
		StoragePrefix: "local",
	}, "test")
	require.NoError(t, err)
	metaMap, err := block.ListBlocks(filepath.Join(dir, PathLocal), time.Time{})
	require.NoError(t, err)
	require.Len(t, metaMap, 1)
	var meta *block.Meta
	for _, m := range metaMap {
		meta = m
	}
	blk := NewSingleBlockQuerierFromMeta(ctx, bkt, meta)
	require.NoError(t, blk.Open(ctx))
	require.NoError(t, blk.symbols.Load(ctx))
	return blk
}

func blockQuerierFromMeta(t *testing.T, dir string, m block.Meta) *singleBlockQuerier {
	t.Helper()
	ctx := context.Background()
	bkt, err := client.NewBucket(ctx, client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: dir,
			},
		},
		StoragePrefix: "",
	}, "test")
	require.NoError(t, err)
	blk := NewSingleBlockQuerierFromMeta(ctx, bkt, &m)
	require.NoError(t, blk.Open(ctx))
	//	require.NoError(t, blk.symbols.Load(ctx))
	return blk
}

func TestCompactMetas(t *testing.T) {
	actual := compactMetas([]block.Meta{
		{
			ULID:    ulid.MustParse("00000000000000000000000001"),
			MinTime: model.TimeFromUnix(0),
			MaxTime: model.TimeFromUnix(100),
			Compaction: block.BlockMetaCompaction{
				Level:   1,
				Sources: []ulid.ULID{ulid.MustParse("00000000000000000000000001")},
			},
			Labels: map[string]string{"foo": "bar"},
		},
		{
			ULID:    ulid.MustParse("00000000000000000000000002"),
			MinTime: model.TimeFromUnix(50),
			MaxTime: model.TimeFromUnix(100),
			Compaction: block.BlockMetaCompaction{
				Level:   0,
				Sources: []ulid.ULID{ulid.MustParse("00000000000000000000000002")},
			},
			Labels: map[string]string{"bar": "buzz"},
		},
		{
			ULID:    ulid.MustParse("00000000000000000000000003"),
			MinTime: model.TimeFromUnix(50),
			MaxTime: model.TimeFromUnix(200),
			Compaction: block.BlockMetaCompaction{
				Level:   3,
				Sources: []ulid.ULID{ulid.MustParse("00000000000000000000000003")},
			},
		},
	}...)
	labels := map[string]string{"foo": "bar", "bar": "buzz"}
	require.Equal(t, model.TimeFromUnix(0), actual.MinTime)
	require.Equal(t, model.TimeFromUnix(200), actual.MaxTime)
	require.Equal(t, block.BlockMetaCompaction{
		Level: 4,
		Sources: []ulid.ULID{
			ulid.MustParse("00000000000000000000000001"),
			ulid.MustParse("00000000000000000000000002"),
			ulid.MustParse("00000000000000000000000003"),
		},
		Parents: []block.BlockDesc{
			{
				ULID:    ulid.MustParse("00000000000000000000000001"),
				MinTime: 0,
				MaxTime: 100000,
			},
			{
				ULID:    ulid.MustParse("00000000000000000000000002"),
				MinTime: 50000,
				MaxTime: 100000,
			},
			{
				ULID:    ulid.MustParse("00000000000000000000000003"),
				MinTime: 50000,
				MaxTime: 200000,
			},
		},
	}, actual.Compaction)
	require.Equal(t, labels, actual.Labels)
	require.Equal(t, block.CompactorSource, actual.Source)
}

func TestMetaFilesFromDir(t *testing.T) {
	dst := t.TempDir()
	generateParquetFile(t, filepath.Join(dst, "foo.parquet"))
	generateParquetFile(t, filepath.Join(dst, "symbols", "bar.parquet"))
	generateFile(t, filepath.Join(dst, "symbols", "index.symdb"), 100)
	generateFile(t, filepath.Join(dst, "symbols", "stacktraces.symdb"), 200)
	generateIndexFile(t, dst)
	actual, err := metaFilesFromDir(dst)

	require.NoError(t, err)
	require.Equal(t, 5, len(actual))
	require.Equal(t, []block.File{
		{
			Parquet: &block.ParquetFile{
				NumRows:      100,
				NumRowGroups: 10,
			},
			RelPath:   "foo.parquet",
			SizeBytes: fileSize(t, filepath.Join(dst, "foo.parquet")),
		},
		{
			RelPath:   block.IndexFilename,
			SizeBytes: fileSize(t, filepath.Join(dst, block.IndexFilename)),
			TSDB: &block.TSDBFile{
				NumSeries: 3,
			},
		},
		{
			Parquet: &block.ParquetFile{
				NumRows:      100,
				NumRowGroups: 10,
			},
			RelPath:   filepath.Join("symbols", "bar.parquet"),
			SizeBytes: fileSize(t, filepath.Join(dst, "symbols", "bar.parquet")),
		},
		{
			RelPath:   filepath.Join("symbols", "index.symdb"),
			SizeBytes: fileSize(t, filepath.Join(dst, "symbols", "index.symdb")),
		},
		{
			RelPath:   filepath.Join("symbols", "stacktraces.symdb"),
			SizeBytes: fileSize(t, filepath.Join(dst, "symbols", "stacktraces.symdb")),
		},
	}, actual)
}

func fileSize(t *testing.T, path string) uint64 {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return uint64(fi.Size())
}

func generateFile(t *testing.T, path string, size int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, f.Truncate(int64(size)))
}

func generateIndexFile(t *testing.T, dir string) {
	t.Helper()
	filePath := filepath.Join(dir, block.IndexFilename)
	idxw, err := index.NewWriter(context.Background(), filePath)
	require.NoError(t, err)
	require.NoError(t, idxw.AddSymbol("a"))
	require.NoError(t, idxw.AddSymbol("b"))
	require.NoError(t, idxw.AddSymbol("c"))
	addSeries(t, idxw, 0, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "a", Value: "b"},
	})
	addSeries(t, idxw, 1, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "a", Value: "c"},
	})
	addSeries(t, idxw, 2, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "b", Value: "a"},
	})
	require.NoError(t, idxw.Close())
}

func addSeries(t *testing.T, idxw *index.Writer, idx int, labels phlaremodel.Labels) {
	t.Helper()
	require.NoError(t, idxw.AddSeries(storage.SeriesRef(idx), labels, model.Fingerprint(labels.Hash()), index.ChunkMeta{SeriesIndex: uint32(idx)}))
}

func generateParquetFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	require.NoError(t, err)
	defer file.Close()

	writer := parquet.NewGenericWriter[struct{ Name string }](file, parquet.MaxRowsPerRowGroup(10))
	defer writer.Close()
	for i := 0; i < 100; i++ {
		_, err := writer.Write([]struct{ Name string }{
			{Name: fmt.Sprintf("name-%d", i)},
		})
		require.NoError(t, err)
	}
}

func Test_SplitStages(t *testing.T) {
	tests := []struct {
		n, s   int
		result [][]int
	}{
		{12, 3, [][]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, {9, 10, 11}}},
		{7, 3, [][]int{{0, 1, 2}, {3, 4, 5}, {6}}},
		{10, 2, [][]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}, {8, 9}}},
		{5, 5, [][]int{{0, 1, 2, 3, 4}}},
	}

	for _, test := range tests {
		assert.Equal(t, test.result, splitStages(test.n, test.s))
	}
}

func Benchmark_CompactSplit(b *testing.B) {
	ctx := phlarecontext.WithLogger(context.Background(), log.NewNopLogger())

	bkt, err := client.NewBucket(ctx, client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: "./testdata/",
			},
		},
		StoragePrefix: "",
	}, "test")
	require.NoError(b, err)
	meta, err := block.ReadMetaFromDir("./testdata/01HHYG6245NWHZWVP27V8WJRT7")
	require.NoError(b, err)
	bl := NewSingleBlockQuerierFromMeta(ctx, bkt, meta)
	require.NoError(b, bl.Open(ctx))
	require.NoError(b, bl.Symbols().Load(ctx))
	dst := b.TempDir()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err = CompactWithSplitting(ctx, CompactWithSplittingOpts{
			Src:                []BlockReader{bl},
			Dst:                dst,
			SplitCount:         32,
			StageSize:          32,
			SplitBy:            SplitByFingerprint,
			DownsamplerEnabled: true,
			Logger:             log.NewNopLogger(),
		})
		require.NoError(b, err)
	}
}
