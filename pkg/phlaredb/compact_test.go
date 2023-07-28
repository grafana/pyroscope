package phlaredb

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingesterv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
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
	new, err := Compact(ctx, []BlockReader{b, b, b, b}, dst)
	require.NoError(t, err)
	require.Equal(t, uint64(3), new.Stats.NumProfiles)
	require.Equal(t, uint64(3), new.Stats.NumSamples)
	require.Equal(t, uint64(3), new.Stats.NumSeries)
	require.Equal(t, model.TimeFromUnix(1), new.MinTime)
	require.Equal(t, model.TimeFromUnix(3), new.MaxTime)
	querier := blockQuerierFromMeta(t, dst, new)

	matchAll := &ingesterv1.SelectProfilesRequest{
		LabelSelector: "{}",
		Type:          mustParseProfileSelector(t, "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
		Start:         0,
		End:           40000,
	}
	it, err := querier.SelectMatchingProfiles(ctx, matchAll)
	require.NoError(t, err)
	series, err := querier.MergeByLabels(ctx, it, "job")
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
	require.Equal(t, &ingesterv1.MergeProfilesStacktracesResult{
		Stacktraces: []*ingesterv1.StacktraceSample{
			{
				FunctionIds: []int32{0, 1, 2},
				Value:       3,
			},
		},
		FunctionNames: []string{"foo", "bar", "baz"},
	}, res)
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
	filePath := filepath.Join(t.TempDir(), block.IndexFilename)
	idxw, err := prepareIndexWriter(context.Background(), filePath, []BlockReader{blk})
	require.NoError(t, err)
	it := newSeriesRewriter(rows, idxw)
	// tests that all rows are written to the correct series index
	require.True(t, it.Next())
	require.Equal(t, uint32(0), it.At().row.SeriesIndex())
	require.True(t, it.Next())
	require.Equal(t, uint32(0), it.At().row.SeriesIndex())
	require.True(t, it.Next())
	require.Equal(t, uint32(0), it.At().row.SeriesIndex())
	require.True(t, it.Next())
	require.Equal(t, uint32(1), it.At().row.SeriesIndex())
	require.True(t, it.Next())
	require.Equal(t, uint32(2), it.At().row.SeriesIndex())
	require.True(t, it.Next())
	require.Equal(t, uint32(2), it.At().row.SeriesIndex())
	require.False(t, it.Next())

	require.NoError(t, it.Err())
	require.NoError(t, it.Close())
	require.NoError(t, idxw.Close())

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

func newBlock(t *testing.T, generator func() []*testhelper.ProfileBuilder) BlockReader {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
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
	metaMap, err := block.ListBlocks(filepath.Join(dir, pathLocal), time.Time{})
	require.NoError(t, err)
	require.Len(t, metaMap, 1)
	var meta *block.Meta
	for _, m := range metaMap {
		meta = m
	}
	blk := NewSingleBlockQuerierFromMeta(ctx, bkt, meta)
	require.NoError(t, blk.Open(ctx))
	require.NoError(t, blk.stacktraces.Load(ctx))
	return blk
}

func blockQuerierFromMeta(t *testing.T, dir string, m block.Meta) Querier {
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
	require.NoError(t, blk.stacktraces.Load(ctx))
	return blk
}

func TestCompactMetas(t *testing.T) {
	actual := compactMetas([]block.Meta{
		{
			ULID:    ulid.MustParse("00000000000000000000000001"),
			MinTime: model.TimeFromUnix(0),
			MaxTime: model.TimeFromUnix(100),
			Compaction: tsdb.BlockMetaCompaction{
				Level:   1,
				Sources: []ulid.ULID{ulid.MustParse("00000000000000000000000001")},
			},
			Labels: map[string]string{"foo": "bar"},
		},
		{
			ULID:    ulid.MustParse("00000000000000000000000002"),
			MinTime: model.TimeFromUnix(50),
			MaxTime: model.TimeFromUnix(100),
			Compaction: tsdb.BlockMetaCompaction{
				Level:   0,
				Sources: []ulid.ULID{ulid.MustParse("00000000000000000000000002")},
			},
			Labels: map[string]string{"bar": "buzz"},
		},
		{
			ULID:    ulid.MustParse("00000000000000000000000003"),
			MinTime: model.TimeFromUnix(50),
			MaxTime: model.TimeFromUnix(200),
			Compaction: tsdb.BlockMetaCompaction{
				Level:   3,
				Sources: []ulid.ULID{ulid.MustParse("00000000000000000000000003")},
			},
		},
	}...)
	labels := map[string]string{"foo": "bar", "bar": "buzz"}
	if hostname, err := os.Hostname(); err == nil {
		labels[block.HostnameLabel] = hostname
	}
	require.Equal(t, model.TimeFromUnix(0), actual.MinTime)
	require.Equal(t, model.TimeFromUnix(200), actual.MaxTime)
	require.Equal(t, tsdb.BlockMetaCompaction{
		Level: 4,
		Sources: []ulid.ULID{
			ulid.MustParse("00000000000000000000000001"),
			ulid.MustParse("00000000000000000000000002"),
			ulid.MustParse("00000000000000000000000003"),
		},
		Parents: []tsdb.BlockDesc{
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

func Test_lookupTable(t *testing.T) {
	// Given the source data set.
	// Copy arbitrary subsets of items from src to dst.
	var dst []string
	src := []string{
		"zero",
		"one",
		"two",
		"three",
		"four",
		"five",
		"six",
		"seven",
	}

	type testCase struct {
		description string
		input       []uint32
		expected    []string
	}

	testCases := []testCase{
		{
			description: "empty table",
			input:       []uint32{5, 0, 3, 1, 2, 2, 4},
			expected:    []string{"five", "zero", "three", "one", "two", "two", "four"},
		},
		{
			description: "no new values",
			input:       []uint32{2, 1, 2, 3},
			expected:    []string{"two", "one", "two", "three"},
		},
		{
			description: "new value mixed",
			input:       []uint32{2, 1, 6, 2, 3},
			expected:    []string{"two", "one", "six", "two", "three"},
		},
	}

	// Try to lookup values in src lazily.
	// Table size must be greater or equal
	// to the source data set.
	l := newLookupTable[string](10)

	populate := func(t *testing.T, x []uint32) {
		for i, v := range x {
			x[i] = l.tryLookup(v)
		}
		// Resolve unknown yet values.
		// Mind the order and deduplication.
		p := -1
		for it := l.iter(); it.Err() == nil && it.Next(); {
			m := int(it.At())
			if m <= p {
				t.Fatal("iterator order invalid")
			}
			p = m
			it.setValue(src[m])
		}
	}

	resolveAppend := func() {
		// Populate dst with the newly resolved values.
		// Note that order in dst does not have to match src.
		for i, v := range l.values {
			l.storeResolved(i, uint32(len(dst)))
			dst = append(dst, v)
		}
	}

	resolve := func(x []uint32) []string {
		// Lookup resolved values.
		var resolved []string
		for _, v := range x {
			resolved = append(resolved, dst[l.lookupResolved(v)])
		}
		return resolved
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			l.reset()
			populate(t, tc.input)
			resolveAppend()
			assert.Equal(t, tc.expected, resolve(tc.input))
		})
	}

	assert.Len(t, dst, 7)
	assert.NotContains(t, dst, "seven")
}
