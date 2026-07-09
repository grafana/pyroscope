// Package symbolizebench benchmarks two ways of turning an unsymbolized
// profile's stack-trace samples into a symbolized model.FunctionName tree:
//
//   - pprof detour: resolver.Pprof -> Symbolizer.SymbolizePprof ->
//     model.TreeFromBackendProfile. This is what
//     pkg/frontend/readpath/queryfrontend's backendTreeSymbolizer runs
//     today for every tree query against an unsymbolized dataset.
//   - symbol ref: symdb.Resolver.SymbolRefTree -> tree.Bytes (the bridge to
//     wire bytes ResultBuilder.Build/symbolref.Rebuild need) ->
//     ResultBuilder.Build -> symbolref.UnresolvedBinaries ->
//     Symbolizer.Resolve per binary -> symbolref.Rebuild. This is what
//     pkg/querybackend's queryTreeSymbolRefs and
//     pkg/frontend/readpath/queryfrontend's resolveSymbolRefs run together
//     for a dataset labeled unsymbolized once symbol-ref trees are enabled.
//
// Both paths in a given sub-benchmark are driven from literally the same
// symdb.Symbols and SampleAppender (see newFixtureDB) and the same warm
// *symbolizer.Symbolizer (see newFixtureSymbolizer): the backing lidia
// table is pre-warmed into an in-memory object-store bucket once, outside
// every timed loop, from a real ELF fixture
// (pkg/symbolizer/testdata/symbols.debug) reached only through the bucket's
// "already uploaded debug info" path (debuginfo.ObjectPath) — the
// benchmark's DebuginfodURL deliberately points nowhere
// (http://127.0.0.1:0), so any further debuginfod fetch during a timed
// section would come back empty rather than silently succeed over the
// network. newFixtureSymbolizer verifies the warm-up actually resolved a
// known real symbol before any benchmark runs. This holds per-address
// symbol RESOLUTION cost identical and warm for both paths, so the ns/op
// delta reported here is attributable to the surrounding tree/pprof
// machinery, not to debuginfod I/O.
//
// LIMITATION: this is a single-dataset comparison. It does not, and cannot,
// capture the symbol-ref design's structural "mixed query" win, where only
// datasets actually labeled unsymbolized pay any of this cost at all, and a
// single unsymbolized dataset in a query no longer forces every other,
// already-symbolized dataset in that same query onto the pprof detour (see
// pkg/querybackend/query_tree.go's queryTree/queryTreeSymbolRefs split).
// That is a query-planning effect one layer above what is exercised here,
// and this file cannot measure it.
package symbolizebench

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/debuginfo"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/v2/pkg/symbolizer"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

const (
	benchTenantID = "symbolizebench-tenant"

	// benchBuildID is the same build ID pkg/test/integration/symbolization_test.go
	// pairs with symbols.debug; reused here for consistency, though the
	// value itself is arbitrary (it is only ever used as a cache key).
	benchBuildID   = "2fa2055ef20fabc972d5751147e093275514b142"
	benchBinary    = "libtarget.so"
	benchRootFrame = "root"

	// Addresses for the unresolved locations start well past any address
	// symbols.debug actually covers (see its documented symbols in
	// pkg/symbolizer/symbolizer_test.go: 0x1500, 0x2745, 0x3c5a, all under
	// 0x10000), so every one of them is a genuine, deterministic lidia
	// miss. A miss still pays the real lidia.Table.Lookup binary-search
	// cost (see lidia.Table.Lookup), and it is the same cost for both
	// paths, so this does not favor either one; it just keeps the tree
	// shape simple to reason about (every unresolved address renders as
	// its own distinct fallback frame, never coalescing with another).
	benchAddrBase   = uint64(0x50000000)
	benchAddrStride = uint64(0x40)

	// warmUpAddr is a real symbol boundary in symbols.debug (resolves to
	// "main"); used to prove the lidia table actually loaded and resolves
	// correctly before any benchmark runs.
	warmUpAddr    = uint64(0x1500)
	warmUpSymName = "main"

	// benchPartition is the single symdb partition every fixture in this
	// file writes to and reads from.
	benchPartition = uint64(0)
)

// testdataDir returns pkg/symbolizer/testdata, reusing its real fixtures
// instead of duplicating them.
func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "symbolizer", "testdata")
}

// newFixtureSymbolizer builds one *symbolizer.Symbolizer backed by an
// in-memory object-store bucket, and warms its lidia cache for benchBuildID
// from the real symbols.debug ELF fixture before returning it. The warm-up
// is the only point in this file that parses ELF or converts to lidia;
// every Resolve call made afterward, including every one made inside a
// timed loop, is served from the bucket's cached lidia bytes.
func newFixtureSymbolizer(tb testing.TB) (*symbolizer.Symbolizer, context.Context) {
	tb.Helper()
	ctx := tenant.InjectTenantID(context.Background(), benchTenantID)

	elfData, err := os.ReadFile(filepath.Join(testdataDir(), "symbols.debug"))
	if err != nil {
		tb.Fatalf("read symbols.debug fixture: %v", err)
	}

	validBuildID, err := debuginfo.ValidateGnuBuildID(benchBuildID)
	if err != nil {
		tb.Fatalf("validate build id: %v", err)
	}

	bucket := objstore.NewBucket(memory.NewInMemBucket())
	if err := bucket.Upload(ctx, debuginfo.ObjectPath(benchTenantID, validBuildID), bytes.NewReader(elfData)); err != nil {
		tb.Fatalf("seed uploaded debug info: %v", err)
	}

	sym, err := symbolizer.New(
		log.NewNopLogger(),
		symbolizer.Config{
			MaxDebuginfodConcurrency: 1,
			// Deliberately unroutable: a debuginfod fetch must never
			// happen in this benchmark (see package doc comment). If the
			// uploaded-debug-info seed above were ever missed, Resolve
			// would fail to reach a symbol here (not hang, not reach the
			// real network) and the warm-up check below would catch it.
			DebuginfodURL: "http://127.0.0.1:0",
		},
		prometheus.NewRegistry(),
		bucket,
		validation.MockDefaultOverrides(),
	)
	if err != nil {
		tb.Fatalf("build symbolizer: %v", err)
	}

	frames, err := sym.Resolve(ctx, benchBuildID, benchBinary, []uint64{warmUpAddr})
	if err != nil {
		tb.Fatalf("warm-up resolve: %v", err)
	}
	if len(frames) != 1 || len(frames[0]) == 0 || frames[0][0].FunctionName != warmUpSymName {
		tb.Fatalf("warm-up resolve did not hit a real symbol: got %#v, want a %q frame at 0x%x", frames, warmUpSymName, warmUpAddr)
	}

	return sym, ctx
}

// newUnsymbolizedProfile builds a synthetic pprof profile shaped like a
// native profile with partial debug info: one resolved frame (benchRootFrame,
// via Function/Line, no symbolization needed) as the common root, and n
// distinct line-less leaf locations on a single real mapping
// (benchBinary/benchBuildID), each its own one-sample stack. This is the
// "same unsymbolized input" both paths in a given sub-benchmark run against.
func newUnsymbolizedProfile(n int) *profilev1.Profile {
	const rootLocID = uint64(1)
	p := &profilev1.Profile{
		StringTable: []string{"", benchBinary, benchBuildID, benchRootFrame},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: 1, BuildId: 2},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 3},
		},
		SampleType: []*profilev1.ValueType{{Type: 0, Unit: 0}},
		Location:   make([]*profilev1.Location, 0, n+1),
		Sample:     make([]*profilev1.Sample, 0, n),
	}
	p.Location = append(p.Location, &profilev1.Location{
		Id:        rootLocID,
		MappingId: 1,
		Line:      []*profilev1.Line{{FunctionId: 1}},
	})
	for i := 0; i < n; i++ {
		locID := rootLocID + 1 + uint64(i)
		p.Location = append(p.Location, &profilev1.Location{
			Id:        locID,
			MappingId: 1,
			Address:   benchAddrBase + uint64(i)*benchAddrStride,
		})
		p.Sample = append(p.Sample, &profilev1.Sample{
			LocationId: []uint64{locID, rootLocID}, // leaf-first, per pprof convention.
			Value:      []int64{1},
		})
	}
	return p
}

// newFixtureDB writes newUnsymbolizedProfile(n) into a fresh, real symdb
// partition (benchPartition) and returns it alongside the resulting
// samples, ready for newLoadedResolver. Building this is the "same symdb
// Symbols + a SampleAppender of samples" setup the package doc comment
// describes; it happens once per size, outside every timed loop.
func newFixtureDB(tb testing.TB, n int) (db *symdb.SymDB, samples schemav1.Samples) {
	tb.Helper()
	db = symdb.NewSymDB(symdb.DefaultConfig().WithDirectory(tb.TempDir()))
	indexed := db.WriteProfileSymbols(benchPartition, newUnsymbolizedProfile(n))
	if len(indexed) != 1 {
		tb.Fatalf("expected exactly one indexed profile, got %d", len(indexed))
	}
	return db, indexed[0].Samples
}

// newLoadedResolver wraps db/samples in a fresh *symdb.Resolver, the shared
// starting point the package doc comment's two call chains begin from.
// Both resolver.Pprof and resolver.SymbolRefTree only read their
// resolver's Symbols/SampleAppender, so calling either repeatedly against
// the resolver returned here (e.g. once per b.Loop iteration) is safe:
// each call builds a fresh result from unmodified source data.
func newLoadedResolver(ctx context.Context, tb testing.TB, db *symdb.SymDB, samples schemav1.Samples) *symdb.Resolver {
	tb.Helper()
	r := symdb.NewResolver(ctx, db)
	r.AddSamples(benchPartition, samples)
	tb.Cleanup(r.Release)
	return r
}

// runPprofDetour runs the pprof-detour path exactly as backendTreeSymbolizer
// (pkg/frontend/readpath/queryfrontend/symbolizer.go) invokes it today.
func runPprofDetour(ctx context.Context, resolver *symdb.Resolver, sym *symbolizer.Symbolizer) ([]byte, error) {
	prof, err := resolver.Pprof()
	if err != nil {
		return nil, fmt.Errorf("resolver.Pprof: %w", err)
	}
	if err := sym.SymbolizePprof(ctx, prof); err != nil {
		return nil, fmt.Errorf("SymbolizePprof: %w", err)
	}
	treeBytes, err := model.TreeFromBackendProfile(prof, 0)
	if err != nil {
		return nil, fmt.Errorf("TreeFromBackendProfile: %w", err)
	}
	return treeBytes, nil
}

// runSymbolRef runs the symbol-ref tree path exactly as queryTreeSymbolRefs
// (pkg/querybackend/query_tree.go) and resolveSymbolRefs
// (pkg/frontend/readpath/queryfrontend/symbol_ref_resolve.go) run it
// together today, minus the intermediate report/RPC plumbing between the
// two (see resolveBinaries).
func runSymbolRef(ctx context.Context, resolver *symdb.Resolver, sym *symbolizer.Symbolizer) ([]byte, error) {
	tree, rb, _, err := resolver.SymbolRefTree()
	if err != nil {
		return nil, fmt.Errorf("resolver.SymbolRefTree: %w", err)
	}
	// tree.Bytes must run before rb.Build: Build only reports the refs
	// KeepRef (invoked from Bytes as it marshals) has observed as
	// reachable. This mirrors queryTreeSymbolRefs exactly.
	treeBytes := tree.Bytes(0, rb.KeepRef)
	pb := new(queryv1.SymbolRefTable)
	rb.Build(pb)

	binaries := symbolref.UnresolvedBinaries(pb)
	lookup, err := resolveBinaries(ctx, sym, binaries)
	if err != nil {
		return nil, fmt.Errorf("resolveBinaries: %w", err)
	}

	rebuilt, err := symbolref.Rebuild(treeBytes, pb, lookup.resolve, 0)
	if err != nil {
		return nil, fmt.Errorf("symbolref.Rebuild: %w", err)
	}
	return rebuilt, nil
}

// symbolRefAddr identifies one (build ID, address) location to resolve.
type symbolRefAddr struct {
	buildID string
	addr    uint64
}

// symbolRefLookup maps a resolved location to its frame chain; an absent
// entry means the location has no resolution, and Rebuild renders the
// binary!0xaddr fallback for it.
type symbolRefLookup map[symbolRefAddr][]symbolref.Frame

func (l symbolRefLookup) resolve(buildID string, addr uint64) []symbolref.Frame {
	return l[symbolRefAddr{buildID, addr}]
}

// resolveBinaries mirrors QueryFrontend.resolveBinaries
// (pkg/frontend/readpath/queryfrontend/symbol_ref_resolve.go): it resolves
// every UnresolvedBinary through the symbolizer, bounded by the
// symbolizer's own configured concurrency, and flattens the results into a
// lookup table for symbolref.Rebuild. Per-tenant resolve timeboxing and
// metrics are query-frontend concerns orthogonal to the tree/pprof
// machinery this benchmark measures, so they are omitted; with the single
// binary this file's fixtures ever produce, that omission does not change
// the shape of the work done.
func resolveBinaries(ctx context.Context, sym *symbolizer.Symbolizer, binaries []symbolref.UnresolvedBinary) (symbolRefLookup, error) {
	lookup := make(symbolRefLookup)
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(sym.ResolveConcurrency())
	for _, binary := range binaries {
		g.Go(func() error {
			frames, err := sym.Resolve(gctx, binary.BuildID, binary.BinaryName, binary.Addresses)
			if err != nil {
				return fmt.Errorf("resolve build id %s: %w", binary.BuildID, err)
			}
			mu.Lock()
			for i, addr := range binary.Addresses {
				if len(frames[i]) == 0 {
					continue
				}
				out := make([]symbolref.Frame, len(frames[i]))
				for j, f := range frames[i] {
					out[j] = symbolref.Frame{Name: f.FunctionName}
				}
				lookup[symbolRefAddr{binary.BuildID, addr}] = out
			}
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return lookup, nil
}

// verifyEquivalentOutput fails tb unless oldBytes and newBytes represent the
// same symbolized tree: same total value and the same set of (stack, self)
// pairs. This is the fairness check backing the package doc comment's claim
// that both paths do genuinely equivalent work: if it ever fails, the
// benchmark below is not comparing like with like.
func verifyEquivalentOutput(tb testing.TB, pprofDetourBytes, symbolRefBytes []byte) {
	tb.Helper()
	pprofDetourStacks, pprofDetourTotal := stacksOf(tb, pprofDetourBytes)
	symbolRefStacks, symbolRefTotal := stacksOf(tb, symbolRefBytes)

	if pprofDetourTotal != symbolRefTotal {
		tb.Fatalf("pprof detour and symbol ref totals differ: %d vs %d", pprofDetourTotal, symbolRefTotal)
	}
	if len(pprofDetourStacks) != len(symbolRefStacks) {
		tb.Fatalf("pprof detour and symbol ref produced different stack counts: %d vs %d", len(pprofDetourStacks), len(symbolRefStacks))
	}
	for stack, self := range pprofDetourStacks {
		got, ok := symbolRefStacks[stack]
		if !ok {
			tb.Fatalf("stack %q present in pprof detour output but missing from symbol ref output", stack)
		}
		if got != self {
			tb.Fatalf("stack %q self value differs: pprof detour=%d symbol ref=%d", stack, self, got)
		}
	}
}

// stacksOf unmarshals treeBytes as a model.FunctionNameTree and returns a
// map from a canonical root-first stack key to its self value, alongside
// the tree's total value. TreeFromBackendProfile's output and Rebuild's
// output are both model.Tree[FunctionName, FunctionNameI] marshal bytes
// (pkg/model's tree format, reused unchanged by pkg/model/symbolref per its
// doc comment), so both unmarshal through the same call.
func stacksOf(tb testing.TB, treeBytes []byte) (stacks map[string]int64, total int64) {
	tb.Helper()
	tree, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](treeBytes)
	if err != nil {
		tb.Fatalf("unmarshal tree: %v", err)
	}
	stacks = make(map[string]int64)
	tree.IterateStacks(func(_ model.FunctionName, self int64, stack []model.FunctionName) {
		cp := slices.Clone(stack)
		slices.Reverse(cp)
		if len(cp) > 0 && cp[0] == "" {
			// A single marshal/unmarshal round trip leaves one synthetic,
			// empty-named ancestor frame at the root (see
			// pkg/model/symbolref's Rebuild and absorbPlainTree doc
			// comments for the same artifact). Both paths here go through
			// exactly one such round trip, so stripping it is symmetric.
			cp = cp[1:]
		}
		parts := make([]string, len(cp))
		for i, name := range cp {
			parts[i] = string(name)
		}
		stacks[strings.Join(parts, "\x00")] += self
	})
	return stacks, tree.Total()
}

// BenchmarkSymbolizeTree compares the pprof-detour and symbol-ref paths (see
// package doc comment) at small, medium and large distinct-unresolved-
// location counts. Run with:
//
//	go test -run '^$' -bench=BenchmarkSymbolizeTree -benchmem ./pkg/test/symbolizebench
func BenchmarkSymbolizeTree(b *testing.B) {
	sym, ctx := newFixtureSymbolizer(b)

	sizes := []struct {
		name string
		n    int
	}{
		{"small", 100},
		{"medium", 5_000},
		{"large", 50_000},
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			db, samples := newFixtureDB(b, sz.n)

			pprofDetourBytes, err := runPprofDetour(ctx, newLoadedResolver(ctx, b, db, samples), sym)
			if err != nil {
				b.Fatalf("pprof detour: %v", err)
			}
			symbolRefBytes, err := runSymbolRef(ctx, newLoadedResolver(ctx, b, db, samples), sym)
			if err != nil {
				b.Fatalf("symbol ref: %v", err)
			}
			verifyEquivalentOutput(b, pprofDetourBytes, symbolRefBytes)

			b.Run("pprof_detour", func(b *testing.B) {
				resolver := newLoadedResolver(ctx, b, db, samples)
				b.ReportAllocs()
				var result []byte
				for b.Loop() {
					var err error
					result, err = runPprofDetour(ctx, resolver, sym)
					if err != nil {
						b.Fatal(err)
					}
				}
				if len(result) == 0 {
					b.Fatal("empty result")
				}
				b.ReportMetric(float64(sz.n), "locations/op")
			})

			b.Run("symbol_ref", func(b *testing.B) {
				resolver := newLoadedResolver(ctx, b, db, samples)
				b.ReportAllocs()
				var result []byte
				for b.Loop() {
					var err error
					result, err = runSymbolRef(ctx, resolver, sym)
					if err != nil {
						b.Fatal(err)
					}
				}
				if len(result) == 0 {
					b.Fatal("empty result")
				}
				b.ReportMetric(float64(sz.n), "locations/op")
			})
		})
	}
}
