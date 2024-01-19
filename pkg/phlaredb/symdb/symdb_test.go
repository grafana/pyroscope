package symdb

import (
	"context"
	"sort"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type memSuite struct {
	t testing.TB

	config   *Config
	db       *SymDB
	files    [][]string
	profiles map[uint64]*googlev1.Profile
	indexed  map[uint64][]v1.InMemoryProfile
}

type blockSuite struct {
	*memSuite
	reader *Reader
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
		s.config = &Config{
			Dir: s.t.TempDir(),
			Stacktraces: StacktracesConfig{
				MaxNodesPerChunk: 1 << 10,
			},
			Parquet: ParquetConfig{
				MaxBufferRowCount: 512,
			},
		}
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
	s.profiles[p] = x.Profile.CloneVT()
	x.Normalize()
	w := s.db.PartitionWriter(p)
	s.indexed[p] = w.WriteProfileSymbols(x.Profile)
}

func (s *blockSuite) flush() {
	require.NoError(s.t, s.db.Flush())
	b, err := filesystem.NewBucket(s.config.Dir)
	require.NoError(s.t, err)
	s.reader, err = Open(context.Background(), b, testBlockMeta)
	require.NoError(s.t, err)
}

func (s *blockSuite) teardown() {
	require.NoError(s.t, s.reader.Close())
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
