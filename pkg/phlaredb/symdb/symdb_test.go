package symdb

import (
	"bytes"
	"context"
	"io"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	pprofth "github.com/grafana/pyroscope/pkg/pprof/testhelper"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type memSuite struct {
	t testing.TB

	config *Config
	db     *SymDB

	// partition => sample type index => object
	files    [][]string
	profiles map[uint64]*googlev1.Profile
	indexed  map[uint64][]v1.InMemoryProfile
}

type blockSuite struct {
	*memSuite
	reader *Reader
	testBucket
}

func newMemSuite(t testing.TB, files [][]string) *memSuite {
	s := memSuite{t: t, files: files}
	s.init()
	return &s
}

func newBlockSuite(t testing.TB, files [][]string) *blockSuite {
	b := blockSuite{memSuite: newMemSuite(t, files)}
	b.flush()
	return &b
}

func (s *memSuite) init() {
	if s.config == nil {
		s.config = DefaultConfig().WithDirectory(s.t.TempDir())
	}
	if s.db == nil {
		s.db = NewSymDB(s.config)
	}
	s.profiles = make(map[uint64]*googlev1.Profile)
	s.indexed = make(map[uint64][]v1.InMemoryProfile)
	for p, files := range s.files {
		for _, f := range files {
			s.writeProfileFromFile(uint64(p), f)
		}
	}
}

func (s *memSuite) writeProfileFromFile(p uint64, f string) {
	x, err := pprof.OpenFile(f)
	require.NoError(s.t, err)
	s.profiles[p] = x.CloneVT()
	x.Normalize()
	w := s.db.PartitionWriter(p)
	s.indexed[p] = w.WriteProfileSymbols(x.Profile)
}

func (s *blockSuite) flush() {
	require.NoError(s.t, s.db.Flush())
	b, err := filesystem.NewBucket(s.config.Dir, func(x objstore.Bucket) (objstore.Bucket, error) {
		s.Bucket = x
		return &s.testBucket, nil
	})
	require.NoError(s.t, err)
	s.reader, err = Open(context.Background(), b, &block.Meta{Files: s.db.Files()})
	require.NoError(s.t, err)
}

func (s *blockSuite) teardown() {
	require.NoError(s.t, s.reader.Close())
}

type testBucket struct {
	getRangeCount atomic.Int64
	getRangeSize  atomic.Int64
	objstore.Bucket
}

func (b *testBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	b.getRangeCount.Add(1)
	b.getRangeSize.Add(length)
	return b.Bucket.GetRange(ctx, name, off, length)
}

func newTestFileWriter(w io.Writer) *writerOffset {
	return &writerOffset{Writer: w}
}

//nolint:unparam
func pprofFingerprint(p *googlev1.Profile, typ int) [][2]uint64 {
	m := make(map[uint64]uint64, len(p.Sample))
	h := xxhash.New()
	for _, s := range p.Sample {
		v := uint64(s.Value[typ])
		if v == 0 {
			continue
		}
		h.Reset()
		for _, loc := range s.LocationId {
			for _, line := range p.Location[loc-1].Line {
				f := p.Function[line.FunctionId-1]
				_, _ = h.WriteString(p.StringTable[f.Name])
			}
		}
		m[h.Sum64()] += v
	}
	s := make([][2]uint64, 0, len(p.Sample))
	for k, v := range m {
		s = append(s, [2]uint64{k, v})
	}
	sort.Slice(s, func(i, j int) bool { return s[i][0] < s[j][0] })
	return s
}

func treeFingerprint(t *phlaremodel.Tree) [][2]uint64 {
	m := make([][2]uint64, 0, 1<<10)
	h := xxhash.New()
	t.IterateStacks(func(_ string, self int64, stack []string) {
		h.Reset()
		for _, loc := range stack {
			_, _ = h.WriteString(loc)
		}
		m = append(m, [2]uint64{h.Sum64(), uint64(self)})
	})
	sort.Slice(m, func(i, j int) bool { return m[i][0] < m[j][0] })
	return m
}

func Test_Stats(t *testing.T) {
	s := memSuite{
		t:     t,
		files: [][]string{{"testdata/profile.pb.gz"}},
		config: &Config{
			Dir: t.TempDir(),
			Stacktraces: StacktracesConfig{
				MaxNodesPerChunk: 4 << 20,
			},
			Parquet: ParquetConfig{
				MaxBufferRowCount: 100 << 10,
			},
		},
	}

	s.init()
	bs := blockSuite{memSuite: &s}
	bs.flush()
	defer bs.teardown()

	p, err := bs.reader.Partition(context.Background(), 0)
	require.NoError(t, err)

	var actual PartitionStats
	p.WriteStats(&actual)
	expected := PartitionStats{
		StacktracesTotal: 561,
		MaxStacktraceID:  1713,
		LocationsTotal:   718,
		MappingsTotal:    3,
		FunctionsTotal:   506,
		StringsTotal:     699,
	}
	require.Equal(t, expected, actual)
}

func TestWritePartition(t *testing.T) {
	p := NewPartitionWriter(0, &Config{
		Version: FormatV3,
		Stacktraces: StacktracesConfig{
			MaxNodesPerChunk: 4 << 20,
		},
		Parquet: ParquetConfig{
			MaxBufferRowCount: 100 << 10,
		},
	})
	profile := pprofth.NewProfileBuilder(time.Now().UnixNano()).
		CPUProfile().
		WithLabels(phlaremodel.LabelNameServiceName, "svc").
		ForStacktraceString("foo", "bar").
		AddSamples(1).
		ForStacktraceString("qwe", "foo", "bar").
		AddSamples(2)

	profiles := p.WriteProfileSymbols(profile.Profile)
	symdbBlob := bytes.NewBuffer(nil)
	err := WritePartition(p, symdbBlob)
	require.NoError(t, err)

	bucket := phlareobj.NewBucket(memory.NewInMemBucket())
	require.NoError(t, bucket.Upload(context.Background(), DefaultFileName, bytes.NewReader(symdbBlob.Bytes())))
	reader, err := Open(context.Background(), bucket, testBlockMeta)
	require.NoError(t, err)

	r := NewResolver(context.Background(), reader)
	defer r.Release()
	r.AddSamples(0, profiles[0].Samples)
	resolved, err := r.Tree()
	require.NoError(t, err)
	expected := `.
└── bar: self 0 total 3
    └── foo: self 1 total 3
        └── qwe: self 2 total 2
`
	require.Equal(t, expected, resolved.String())
}

func BenchmarkPartitionWriter_WriteProfileSymbols(b *testing.B) {
	b.ReportAllocs()

	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(b, err)
	p.Normalize()
	cfg := DefaultConfig().WithDirectory(b.TempDir())

	db := NewSymDB(cfg)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		newP := p.CloneVT()
		pw := db.PartitionWriter(uint64(i))
		b.StartTimer()

		pw.WriteProfileSymbols(newP)
	}
}
