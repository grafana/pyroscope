package symdb

import (
	"context"
	"sort"
	"testing"

	"github.com/cespare/xxhash/v2"
	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
)

func Test_symdb_memory_Resolver_ResolveProfile(t *testing.T) {
	s := newResolverSuite(t, "testdata/profile.pb.gz")
	expectedFingerprint := pprofFingerprint(s.profiles[0].Profile, 0)
	resolved, err := s.resolver.ResolveProfile(context.Background(), s.indexed[0][0].Samples)
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, profileFingerprint(resolved, 0))
}

func Test_symdb_memory_Resolver_ResolveTree(t *testing.T) {
	s := newResolverSuite(t, "testdata/profile.pb.gz")
	expectedFingerprint := pprofFingerprint(s.profiles[0].Profile, 0)
	tree, err := s.resolver.ResolveTree(context.Background(), s.indexed[0][0].Samples)
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, treeFingerprint(tree))
}

func Test_symdb_block_Resolver_ResolveProfile(t *testing.T) {
	s := newBlockResolverSuite(t, "testdata/profile.pb.gz")
	defer s.teardown()
	expectedFingerprint := pprofFingerprint(s.profiles[0].Profile, 0)
	resolved, err := s.resolver.ResolveProfile(context.Background(), s.indexed[0][0].Samples)
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, profileFingerprint(resolved, 0))
}

func Test_symdb_block_Resolver_ResolveTree(t *testing.T) {
	s := newBlockResolverSuite(t, "testdata/profile.pb.gz")
	defer s.teardown()
	expectedFingerprint := pprofFingerprint(s.profiles[0].Profile, 0)
	tree, err := s.resolver.ResolveTree(context.Background(), s.indexed[0][0].Samples)
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, treeFingerprint(tree))
}

func Benchmark_symdb_block_Resolver_ResolveProfile(t *testing.B) {
	s := newBlockResolverSuite(t, "testdata/profile.pb.gz")
	defer s.teardown()
	t.ResetTimer()
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		_, err := s.resolver.ResolveProfile(context.Background(), s.indexed[0][0].Samples)
		require.NoError(t, err)
	}
}

func Benchmark_symdb_block_Resolver_ResolveTree(t *testing.B) {
	s := newBlockResolverSuite(t, "testdata/profile.pb.gz")
	defer s.teardown()
	t.ResetTimer()
	t.ReportAllocs()
	for i := 0; i < t.N; i++ {
		_, err := s.resolver.ResolveTree(context.Background(), s.indexed[0][0].Samples)
		require.NoError(t, err)
	}
}

type resolverSuite struct {
	t testing.TB

	config   *Config
	db       *SymDB
	files    []string
	profiles []*pprof.Profile
	indexed  [][]v1.InMemoryProfile
	resolver *Resolver
}

type blockResolverSuite struct {
	*resolverSuite

	reader    *Reader
	partition *PartitionReader
}

func newResolverSuite(t testing.TB, files ...string) *resolverSuite {
	s := resolverSuite{t: t}
	for _, f := range files {
		s.files = append(s.files, f)
	}
	s.init()
	r, err := s.db.SymbolsReader(1)
	require.NoError(t, err)
	s.resolver = &Resolver{
		Stacktraces: r,
		Locations:   r.locations.slice,
		Mappings:    r.mappings.slice,
		Functions:   r.functions.slice,
		Strings:     r.strings.slice,
	}
	return &s
}

func newBlockResolverSuite(t testing.TB, files ...string) *blockResolverSuite {
	b := blockResolverSuite{resolverSuite: newResolverSuite(t, files...)}
	b.flush()
	return &b
}

func (s *resolverSuite) init() {
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

	w := s.db.SymbolsWriter(1)
	for _, f := range s.files {
		p, err := pprof.OpenFile(f)
		require.NoError(s.t, err)
		s.profiles = append(s.profiles, p)
		s.indexed = append(s.indexed, w.WriteProfileSymbols(p.Profile))
	}
}

func (s *blockResolverSuite) flush() {
	require.NoError(s.t, s.db.Flush())
	b, err := filesystem.NewBucket(s.config.Dir)
	require.NoError(s.t, err)
	s.reader, err = Open(context.Background(), b, testBlockMeta)
	require.NoError(s.t, err)

	s.partition, err = s.reader.SymbolsReader(context.Background(), 1)
	require.NoError(s.t, err)
	s.resolver = &Resolver{
		Stacktraces: s.partition,
		Locations:   s.partition.locations.s,
		Mappings:    s.partition.mappings.s,
		Functions:   s.partition.functions.s,
		Strings:     s.partition.strings.s,
	}
}

func (s *blockResolverSuite) teardown() {
	s.partition.Release()
	require.NoError(s.t, s.reader.Close())
}

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
			for _, line := range p.Location[loc].Line {
				f := p.Function[line.FunctionId-1].Name
				_, _ = h.WriteString(p.StringTable[f])
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

func profileFingerprint(p *profile.Profile, typ int) [][2]uint64 {
	m := make(map[uint64]uint64, len(p.Sample))
	h := xxhash.New()
	for _, s := range p.Sample {
		h.Reset()
		for _, loc := range s.Location {
			for _, line := range loc.Line {
				_, _ = h.WriteString(line.Function.Name)
			}
		}
		m[h.Sum64()] += uint64(s.Value[typ])
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
