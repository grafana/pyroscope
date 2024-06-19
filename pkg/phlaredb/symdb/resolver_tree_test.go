package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_memory_Resolver_ResolveTree(t *testing.T) {
	s := newMemSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	expectedFingerprint := pprofFingerprint(s.profiles[0], 0)
	r := NewResolver(context.Background(), s.db)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][0].Samples)
	resolved, err := r.Tree()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, treeFingerprint(resolved))
}

func Test_block_Resolver_ResolveTree(t *testing.T) {
	s := newBlockSuite(t, [][]string{{"testdata/profile.pb.gz"}})
	defer s.teardown()
	expectedFingerprint := pprofFingerprint(s.profiles[0], 1)
	r := NewResolver(context.Background(), s.reader)
	defer r.Release()
	r.AddSamples(0, s.indexed[0][1].Samples)
	resolved, err := r.Tree()
	require.NoError(t, err)
	require.Equal(t, expectedFingerprint, treeFingerprint(resolved))
}

func Benchmark_Resolver_ResolveTree_Small(b *testing.B) {
	s := newMemSuite(b, [][]string{{"testdata/profile.pb.gz"}})
	samples := s.indexed[0][0].Samples
	b.Run("0", benchmarkResolverResolveTree(s.db, samples, 0))
	b.Run("1K", benchmarkResolverResolveTree(s.db, samples, 1<<10))
	b.Run("8K", benchmarkResolverResolveTree(s.db, samples, 8<<10))
}

/*

Benchmark_Resolver_ResolveTree_Big/0-10         	       4	 299818614 ns/op	353794342 B/op	    5105 allocs/op
Benchmark_Resolver_ResolveTree_Big/8K-10        	      18	  65545331 ns/op	42066008 B/op	  211175 allocs/op
Benchmark_Resolver_ResolveTree_Big/16K-10         	      14	  81117440 ns/op	45750302 B/op	  369430 allocs/op
Benchmark_Resolver_ResolveTree_Big/32K-10         	      10	 109941121 ns/op	53830833 B/op	  636970 allocs/op
Benchmark_Resolver_ResolveTree_Big/64K-10         	       7	 163126869 ns/op	71399464 B/op	 1090633 allocs/op

Benchmark_Resolver_ResolveTree_Big/0-10           	       3	 382531792 ns/op	357809285 B/op	    5091 allocs/op
Benchmark_Resolver_ResolveTree_Big/8K-10         	       8	 143147573 ns/op	42734693 B/op	  211171 allocs/op
Benchmark_Resolver_ResolveTree_Big/16K-10         	       7	 153564345 ns/op	46436185 B/op	  369411 allocs/op
Benchmark_Resolver_ResolveTree_Big/32K-10         	       6	 198486069 ns/op	53514521 B/op	  636986 allocs/op
Benchmark_Resolver_ResolveTree_Big/64K-10         	       5	 226230125 ns/op	71400292 B/op	 1090627 allocs/op

Benchmark_Resolver_ResolveTree_Big/0-10           	       3	 369969042 ns/op	93457314 B/op	    6100 allocs/op
Benchmark_Resolver_ResolveTree_Big/8K-10         	       3	 374616931 ns/op	94364194 B/op	  106278 allocs/op
Benchmark_Resolver_ResolveTree_Big/16K-10         	       3	 404164236 ns/op	94973693 B/op	  184385 allocs/op
Benchmark_Resolver_ResolveTree_Big/32K-10         	       3	 394745694 ns/op	96144632 B/op	  313174 allocs/op
Benchmark_Resolver_ResolveTree_Big/64K-10         	       3	 410901431 ns/op	15065730 B/op	  523421 allocs/op



Benchmark_Resolver_ResolveTree_Big/0-10  	               3	 345638903 ns/op	15397480 B/op	    1502 allocs/op
Benchmark_Resolver_ResolveTree_Big/8K-10 	               3	 349856833 ns/op	99331714 B/op	  101715 allocs/op
Benchmark_Resolver_ResolveTree_Big/16K-10         	       3	 357887680 ns/op	99937378 B/op	  179784 allocs/op
Benchmark_Resolver_ResolveTree_Big/32K-10         	       3	 366743500 ns/op	101102472 B/op	  308580 allocs/op
Benchmark_Resolver_ResolveTree_Big/64K-10         	       3	 378255972 ns/op	20026909 B/op	  518855 allocs/op

*/

func Benchmark_Resolver_ResolveTree_Big(b *testing.B) {
	s := memSuite{t: b, files: [][]string{{"testdata/big-profile.pb.gz"}}}
	s.config = DefaultConfig().WithDirectory(b.TempDir())
	s.init()
	samples := s.indexed[0][0].Samples
	b.Run("0", benchmarkResolverResolveTree(s.db, samples, 0))
	b.Run("8K", benchmarkResolverResolveTree(s.db, samples, 8<<10))
	b.Run("16K", benchmarkResolverResolveTree(s.db, samples, 16<<10))
	b.Run("32K", benchmarkResolverResolveTree(s.db, samples, 32<<10))
	b.Run("64K", benchmarkResolverResolveTree(s.db, samples, 64<<10))
}

func benchmarkResolverResolveTree(sym SymbolsReader, samples v1.Samples, n int64) func(b *testing.B) {
	return func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r := NewResolver(context.Background(), sym, WithResolverMaxNodes(n))
			r.AddSamples(0, samples)
			_, _ = r.Tree()
		}
	}
}
