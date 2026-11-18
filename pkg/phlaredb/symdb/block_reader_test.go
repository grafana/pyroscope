package symdb

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	pystore "github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

var (
	testBlockMeta = &block.Meta{
		Files: []block.File{
			{RelPath: DefaultFileName},
		},
	}

	testBlockMetaV1 = &block.Meta{
		Files: []block.File{
			{RelPath: IndexFileName},
			{RelPath: StacktracesFileName},
		},
	}

	testBlockMetaV2 = &block.Meta{
		Files: []block.File{
			{RelPath: IndexFileName},
			{RelPath: StacktracesFileName},
			{RelPath: "locations.parquet"},
			{RelPath: "mappings.parquet"},
			{RelPath: "functions.parquet"},
			{RelPath: "strings.parquet"},
		},
	}
)

func Test_write_block_fixture(t *testing.T) {
	t.Skip()
	b := newBlockSuite(t, [][]string{
		{"testdata/profile.pb.gz"},
		{"testdata/profile.pb.gz"},
	})
	const fixtureDir = "testdata/symbols/v3"
	require.NoError(t, os.RemoveAll(fixtureDir))
	require.NoError(t, os.Rename(b.config.Dir, fixtureDir))
}

func Test_Reader_Open_v3(t *testing.T) {
	// The block contains two partitions (0 and 1), each partition
	// stores symbols of the testdata/profile.pb.gz profile
	b, err := filesystem.NewBucket("testdata/symbols/v3")
	require.NoError(t, err)
	x, err := Open(context.Background(), b, testBlockMeta)
	require.NoError(t, err)

	r := NewResolver(context.Background(), x)
	defer r.Release()
	r.AddSamples(0, schemav1.Samples{
		StacktraceIDs: []uint32{1, 2, 3, 4, 5},
		Values:        []uint64{1, 1, 1, 1, 1},
	})
	r.AddSamples(1, schemav1.Samples{
		StacktraceIDs: []uint32{1, 2, 3, 4, 5},
		Values:        []uint64{1, 1, 1, 1, 1},
	})

	resolved, err := r.Tree()
	require.NoError(t, err)
	expected := `.
├── github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).takeSnapshots: self 2 total 8
│   └── github.com/pyroscope-io/pyroscope/pkg/agent/gospy.(*GoSpy).Snapshot: self 2 total 6
│       └── github.com/pyroscope-io/pyroscope/pkg/convert.ParsePprof: self 0 total 4
│           └── io/ioutil.ReadAll: self 2 total 4
│               └── io.ReadAll: self 2 total 2
└── net/http.(*conn).serve: self 2 total 2
`

	require.Equal(t, expected, resolved.String())
}

func Test_Reader_Open_v3_fuzz(t *testing.T) {
	// Make sure the test is valid.
	corpus, err := os.ReadFile("testdata/symbols/v3/symbols.symdb")
	require.NoError(t, err)
	ctx := context.Background()

	bucket := pystore.NewBucket(objstore.NewInMemBucket())
	require.NoError(t, bucket.Upload(ctx, DefaultFileName, bytes.NewReader(corpus)))
	b, err := Open(ctx, bucket, testBlockMeta)
	require.NoError(t, err)

	r := NewResolver(context.Background(), b)
	defer r.Release()
	r.AddSamples(0, schemav1.Samples{})
	r.AddSamples(1, schemav1.Samples{})
	_, err = r.Pprof()
	require.NoError(t, err)
}

func Fuzz_Reader_Open_v3(f *testing.F) {
	corpus, err := os.ReadFile("testdata/symbols/v3/symbols.symdb")
	require.NoError(f, err)
	ctx := context.Background()

	f.Add(corpus)
	f.Fuzz(func(t *testing.T, data []byte) {
		bucket := pystore.NewBucket(objstore.NewInMemBucket())
		require.NoError(t, bucket.Upload(ctx, DefaultFileName, bytes.NewReader(data)))

		b, err := Open(context.Background(), bucket, testBlockMeta)
		if err != nil {
			return
		}

		r := NewResolver(context.Background(), b)
		defer r.Release()
		r.AddSamples(0, schemav1.Samples{})
		r.AddSamples(1, schemav1.Samples{})
		_, _ = r.Pprof()
	})
}

func Test_Reader_OpenObject_prefetch(t *testing.T) {
	// The block contains two partitions (0 and 1); see Test_Reader_Open_v3.
	ctx := context.Background()

	b, err := filesystem.NewBucket("testdata/symbols/v3")
	require.NoError(t, err)
	x, err := Open(ctx, b, testBlockMeta)
	require.NoError(t, err)
	// The index section with the footer: the minimal prefetch
	// window that covers the index starts exactly at the index offset.
	indexSize := x.file.SizeBytes - x.footer.IndexOffset
	fileSize := x.file.SizeBytes

	for _, tc := range []struct {
		name         string
		prefetchSize uint64
		reads        int64
	}{
		{"window does not cover the index", uint64(FooterSize), 2},
		{"window starts at the index offset", indexSize, 1},
		{"window covers the index", fileSize, 1},
		{"window exceeds the file size", fileSize + 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bucket := &testBucket{}
			b, err := filesystem.NewBucket("testdata/symbols/v3", func(x objstore.Bucket) (objstore.Bucket, error) {
				bucket.Bucket = x
				return bucket, nil
			})
			require.NoError(t, err)
			r, err := OpenObject(ctx, b, DefaultFileName, 0, int64(fileSize), WithPrefetchSize(tc.prefetchSize))
			require.NoError(t, err)
			require.Len(t, r.partitions, 2)
			require.Equal(t, tc.reads, bucket.getRangeCount.Load())

			res := NewResolver(ctx, r)
			defer res.Release()
			res.AddSamples(0, schemav1.Samples{
				StacktraceIDs: []uint32{1, 2, 3, 4, 5},
				Values:        []uint64{1, 1, 1, 1, 1},
			})
			res.AddSamples(1, schemav1.Samples{
				StacktraceIDs: []uint32{1, 2, 3, 4, 5},
				Values:        []uint64{1, 1, 1, 1, 1},
			})

			resolved, err := res.Tree()
			require.NoError(t, err)
			expected := `.
├── github.com/pyroscope-io/pyroscope/pkg/agent.(*ProfileSession).takeSnapshots: self 2 total 8
│   └── github.com/pyroscope-io/pyroscope/pkg/agent/gospy.(*GoSpy).Snapshot: self 2 total 6
│       └── github.com/pyroscope-io/pyroscope/pkg/convert.ParsePprof: self 0 total 4
│           └── io/ioutil.ReadAll: self 2 total 4
│               └── io.ReadAll: self 2 total 2
└── net/http.(*conn).serve: self 2 total 2
`

			require.Equal(t, expected, resolved.String())
		})
	}
}

func Test_Reader_OpenObject_prefetch_invalid_index_offset(t *testing.T) {
	// The footer CRC is valid, but the index offset is not.
	corpus, err := os.ReadFile("testdata/symbols/v3/symbols.symdb")
	require.NoError(t, err)

	var footer Footer
	require.NoError(t, footer.UnmarshalBinary(corpus[len(corpus)-FooterSize:]))

	fileSize := uint64(len(corpus))
	for _, tc := range []struct {
		name        string
		indexOffset uint64
	}{
		{"index offset overlaps the footer", fileSize - uint64(FooterSize) + 1},
		{"index offset is the file size", fileSize},
		{"index offset is beyond the file size", ^uint64(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := append([]byte(nil), corpus...)
			footer.IndexOffset = tc.indexOffset
			copy(data[len(data)-FooterSize:], footer.MarshalBinary())

			ctx := context.Background()
			bucket := pystore.NewBucket(objstore.NewInMemBucket())
			require.NoError(t, bucket.Upload(ctx, DefaultFileName, bytes.NewReader(data)))
			_, err := OpenObject(ctx, bucket, DefaultFileName, 0, int64(fileSize), WithPrefetchSize(uint64(FooterSize)))
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidSize)
		})
	}
}

func Test_Reader_Open_v2(t *testing.T) {
	// The block contains two partitions (0 and 1), each partition
	// stores symbols of the testdata/profile.pb.gz profile
	b, err := filesystem.NewBucket("testdata/symbols/v2")
	require.NoError(t, err)
	x, err := Open(context.Background(), b, testBlockMetaV2)
	require.NoError(t, err)

	r := NewResolver(context.Background(), x)
	defer r.Release()
	r.AddSamples(0, schemav1.Samples{
		StacktraceIDs: []uint32{1, 2, 3, 4, 5},
		Values:        []uint64{1, 1, 1, 1, 1},
	})
	r.AddSamples(1, schemav1.Samples{
		StacktraceIDs: []uint32{1, 2, 3, 4, 5},
		Values:        []uint64{1, 1, 1, 1, 1},
	})

	resolved, err := r.Tree()
	require.NoError(t, err)
	expected := `.
└── github.com/pyroscope-io/pyroscope/pkg/scrape.(*scrapeLoop).run: self 2 total 10
    └── github.com/pyroscope-io/pyroscope/pkg/scrape.(*Target).report: self 2 total 8
        └── github.com/pyroscope-io/pyroscope/pkg/scrape.(*scrapeLoop).scrape: self 2 total 6
            └── github.com/pyroscope-io/pyroscope/pkg/scrape.(*pprofWriter).writeProfile: self 2 total 4
                └── google.golang.org/protobuf/proto.Unmarshal: self 2 total 2
`

	require.Equal(t, expected, resolved.String())
}

func Test_Reader_Open_v1(t *testing.T) {
	b, err := filesystem.NewBucket("testdata/symbols/v1")
	require.NoError(t, err)
	x, err := Open(context.Background(), b, testBlockMetaV1)
	require.NoError(t, err)
	r, err := x.partition(context.Background(), 1)
	require.NoError(t, err)

	dst := new(mockStacktraceInserter)
	dst.On("InsertStacktrace", uint32(2), []int32{2, 1})
	dst.On("InsertStacktrace", uint32(3), []int32{3, 2, 1})
	dst.On("InsertStacktrace", uint32(11), []int32{4, 3, 2, 1})
	dst.On("InsertStacktrace", uint32(16), []int32{3, 1})
	dst.On("InsertStacktrace", uint32(18), []int32{5, 2, 1})

	err = r.ResolveStacktraceLocations(context.Background(), dst, []uint32{3, 2, 11, 16, 18})
	require.NoError(t, err)
}

func Fuzz_ReadIndexFile_v12(f *testing.F) {
	files := []string{
		"testdata/symbols/v2/index.symdb",
		"testdata/symbols/v1/index.symdb",
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		require.NoError(f, err)
		f.Add(data)
	}
	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _ = OpenIndex(b)
	})
}

type mockStacktraceInserter struct{ mock.Mock }

func (m *mockStacktraceInserter) InsertStacktrace(stacktraceID uint32, locations []int32) {
	m.Called(stacktraceID, locations)
}

func Benchmark_Reader_ResolvePprof(b *testing.B) {
	ctx := context.Background()
	s := memSuite{t: b, files: [][]string{{"testdata/big-profile.pb.gz"}}}
	s.config = DefaultConfig().WithDirectory(b.TempDir())
	s.init()
	bs := blockSuite{memSuite: &s}
	bs.flush()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := NewResolver(ctx, bs.reader)
		r.AddSamples(0, schemav1.Samples{})
		_, err := r.Pprof()
		require.NoError(b, err)
		r.Release()
	}

	b.ReportMetric(float64(bs.getRangeCount.Load())/float64(b.N), "get_range_calls/op")
	b.ReportMetric(float64(bs.getRangeSize.Load())/float64(b.N), "get_range_bytes/op")
}
