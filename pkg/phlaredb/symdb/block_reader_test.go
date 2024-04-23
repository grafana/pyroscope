package symdb

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

var testBlockMeta = &block.Meta{
	Files: []block.File{
		{RelPath: IndexFileName},
		{RelPath: StacktracesFileName},
		{RelPath: DataFileName},
		{RelPath: "locations.parquet"},
		{RelPath: "mappings.parquet"},
		{RelPath: "functions.parquet"},
		{RelPath: "strings.parquet"},
	},
}

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

func Fuzz_ReadIndexFile(f *testing.F) {
	files := []string{
		"testdata/symbols/v3/index.symdb",
		"testdata/symbols/v2/index.symdb",
		"testdata/symbols/v1/index.symdb",
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		require.NoError(f, err)
		f.Add(data)
	}
	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _ = ReadIndexFile(b)
	})
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

func Test_Reader_Open_v2(t *testing.T) {
	// The block contains two partitions (0 and 1), each partition
	// stores symbols of the testdata/profile.pb.gz profile
	b, err := filesystem.NewBucket("testdata/symbols/v2")
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
	x, err := Open(context.Background(), b, testBlockMeta)
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

type mockStacktraceInserter struct{ mock.Mock }

func (m *mockStacktraceInserter) InsertStacktrace(stacktraceID uint32, locations []int32) {
	m.Called(stacktraceID, locations)
}

func Benchmark_Reader_ResolvePprof(b *testing.B) {
	ctx := context.Background()
	s := newBlockSuite(b, [][]string{
		{"testdata/big-profile.pb.gz"},
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := NewResolver(ctx, s.reader)
		r.AddSamples(0, s.indexed[0][0].Samples)
		_, err := r.Pprof()
		require.NoError(b, err)
		r.Release()
	}
}
