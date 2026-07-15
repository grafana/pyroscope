package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
)

// symbolsForProfile writes p to a fresh in-memory partition 0 and returns
// its Symbols and an appender loaded with every resulting sample.
func symbolsForProfile(t *testing.T, p *profilev1.Profile) (*Symbols, *SampleAppender) {
	t.Helper()
	s := newMemSuite(t, nil)
	const partition = 0
	indexed := s.db.WriteProfileSymbols(partition, p)
	pr, err := s.db.Partition(context.Background(), partition)
	require.NoError(t, err)
	appender := NewSampleAppender()
	for _, ip := range indexed {
		appender.AppendMany(ip.Samples.StacktraceIDs, ip.Samples.Values)
	}
	return pr.Symbols(), appender
}

// collectStacks captures every (leaf-first stack, self) pair IterateStacks
// visits, cloning the stack slice since IterateStacks reuses its buffer.
func collectStacks(tree *model.LocationRefNameTree) (stacks [][]model.LocationRefName, selves []int64) {
	tree.IterateStacks(func(_ model.LocationRefName, self int64, stack []model.LocationRefName) {
		stacks = append(stacks, append([]model.LocationRefName{}, stack...))
		selves = append(selves, self)
	})
	return stacks, selves
}

// TestSymbols_SymbolRefTree_LinedAndUnresolved covers a stack that mixes a
// lined (resolved) frame, an unresolved line-less frame with a real,
// unambiguous mapping, and a genuine no-mapping (kernel/JIT) frame.
func TestSymbols_SymbolRefTree_LinedAndUnresolved(t *testing.T) {
	p := &profilev1.Profile{
		StringTable: []string{"", "main", "libB.so", "bidB"},
		Mapping: []*profilev1.Mapping{
			{Id: 1}, // occupies symdb index 0; unused by any line-less location.
			{Id: 2, Filename: 2, BuildId: 3},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 1},
		},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}}, // main, lined
			{Id: 2, MappingId: 2, Address: 0x2000},                          // unresolved, real mapping
			{Id: 3, MappingId: 0, Address: 0xdeadbeef},                      // kernel/JIT, no mapping
		},
		Sample: []*profilev1.Sample{
			// leaf-first: kernel/JIT frame, then the unresolved lib frame, then main at the root.
			{LocationId: []uint64{3, 2, 1}, Value: []int64{42}},
		},
		SampleType: []*profilev1.ValueType{{Type: 0, Unit: 0}},
	}
	symbols, appender := symbolsForProfile(t, p)
	table := symbolref.NewTable()

	tree, err := symbols.SymbolRefTree(context.Background(), appender, SelectStackTraces(symbols, nil), table, newUnresolvedCap(0))
	require.NoError(t, err)
	assert.Equal(t, int64(42), tree.Total())

	stacks, selves := collectStacks(tree)
	require.Len(t, stacks, 1)
	require.Len(t, stacks[0], 3)
	assert.Equal(t, int64(42), selves[0])
	kernelRef, unresolvedRef, mainRef := stacks[0][0], stacks[0][1], stacks[0][2]

	rb := table.ResultBuilder()
	mainIdx := rb.KeepRef(mainRef)
	kernelIdx := rb.KeepRef(kernelRef)
	rb.KeepRef(unresolvedRef)
	pb := &queryv1.SymbolRefTable{}
	rb.Build(pb)

	assert.Equal(t, "main", pb.Names[mainIdx])
	assert.Equal(t, ".!0xdeadbeef", pb.Names[kernelIdx],
		`legacy-parity fallback; "." is filepath.Base of the filename-less mapping the no-mapping frame indexes`)

	unresolved := symbolref.UnresolvedBinaries(pb)
	require.Len(t, unresolved, 1)
	assert.Equal(t, "bidB", unresolved[0].BuildID)
	assert.Equal(t, "libB.so", unresolved[0].BinaryName)
	assert.Equal(t, []uint64{0x2000}, unresolved[0].Addresses)
}

// TestSymbols_SymbolRefTree_FirstMappingResolves covers the case the old
// MappingId==0 special-case silently broke: a line-less location on the
// partition's first mapping (symdb index 0, a real mapping with a build ID)
// must reach InternUnresolved and be symbolizable, not render as bare hex.
// This is the common single-mapping native-profile shape.
//
// It also documents the residual limitation of gating on the build ID: a
// genuine no-mapping (kernel/JIT) location shares symdb index 0 with the
// first real mapping, so it is attributed to that mapping's build ID and
// falls back at resolve time (wrong binary name) rather than staying bare
// hex. Fully disambiguating the two would need a write-side change to
// reserve index 0; this read-side fix trades a rare, gracefully-degrading
// misattribution for correct symbolization of the common case. See
// SymbolRefTree's doc comment.
func TestSymbols_SymbolRefTree_FirstMappingResolves(t *testing.T) {
	p := &profilev1.Profile{
		StringTable: []string{"", "libA.so", "bidA"},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: 1, BuildId: 2}, // symdb index 0: a real, resolvable mapping.
		},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Address: 0x1234}, // line-less, on the first mapping (index 0).
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{1}, Value: []int64{1}},
		},
		SampleType: []*profilev1.ValueType{{Type: 0, Unit: 0}},
	}
	symbols, appender := symbolsForProfile(t, p)
	table := symbolref.NewTable()

	tree, err := symbols.SymbolRefTree(context.Background(), appender, SelectStackTraces(symbols, nil), table, newUnresolvedCap(0))
	require.NoError(t, err)

	rb := table.ResultBuilder()
	stacks, _ := collectStacks(tree)
	require.Len(t, stacks, 1)
	require.Len(t, stacks[0], 1)
	rb.KeepRef(stacks[0][0])
	pb := &queryv1.SymbolRefTable{}
	rb.Build(pb)

	unresolved := symbolref.UnresolvedBinaries(pb)
	require.Len(t, unresolved, 1, "a line-less location on the first mapping must be symbolizable, not bare hex")
	assert.Equal(t, "bidA", unresolved[0].BuildID)
	assert.Equal(t, "libA.so", unresolved[0].BinaryName)
	assert.Equal(t, []uint64{0x1234}, unresolved[0].Addresses)
	// Build always writes the reserved ref-0 placeholder, so a snapshot
	// with no real names is [""], not empty.
	assert.Equal(t, []string{""}, pb.Names, "nothing should render as a bare hex name")
}

// TestSymbols_SymbolRefTree_NoBuildIDRendersFallback covers line-less
// locations whose mapping carries no build ID (a binary with no GNU build
// ID, or a genuine no-mapping frame indexing a build-ID-less mapping): they
// cannot be symbolized, are not interned as unresolved, and render the same
// fallback name the legacy symbolizer gives unresolvable frames — keeping
// the binary-name context when the mapping has a filename.
func TestSymbols_SymbolRefTree_NoBuildIDRendersFallback(t *testing.T) {
	p := &profilev1.Profile{
		StringTable: []string{"", "/usr/bin/goldpinger"},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: 1}, // filename, no build ID: e.g. a Go binary without a GNU build ID.
			{Id: 2},              // neither filename nor build ID.
		},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Address: 0xdeadbeef},
			{Id: 2, MappingId: 2, Address: 0x1111},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 1}, Value: []int64{1}},
		},
		SampleType: []*profilev1.ValueType{{Type: 0, Unit: 0}},
	}
	symbols, appender := symbolsForProfile(t, p)
	table := symbolref.NewTable()

	tree, err := symbols.SymbolRefTree(context.Background(), appender, SelectStackTraces(symbols, nil), table, newUnresolvedCap(0))
	require.NoError(t, err)

	rb := table.ResultBuilder()
	stacks, _ := collectStacks(tree)
	require.Len(t, stacks, 1)
	require.Len(t, stacks[0], 2)
	rb.KeepRef(stacks[0][0])
	rb.KeepRef(stacks[0][1])
	pb := &queryv1.SymbolRefTable{}
	rb.Build(pb)

	assert.Empty(t, symbolref.UnresolvedBinaries(pb), "a mapping with no build ID is not symbolizable")
	assert.Contains(t, pb.Names, "goldpinger!0xdeadbeef", "keeps the binary-name context, like the legacy fallback")
	assert.Contains(t, pb.Names, ".!0x1111",
		`empty filename: "." is filepath.Base(""), byte-for-byte with the legacy fallback derivation`)
	assert.NotContains(t, pb.Names, "deadbeef", "no bare-hex names")
	assert.NotContains(t, pb.Names, "1111", "no bare-hex names")
}

// TestResolver_SymbolRefTree_MultiPartitionRefConsistency verifies that the
// same resolved name and the same unresolved (build ID, address) pair,
// interned from two different partitions, land on the same symbolref.Table
// ref and merge into a single tree node rather than two siblings.
func TestResolver_SymbolRefTree_MultiPartitionRefConsistency(t *testing.T) {
	newPartitionProfile := func(mainValue, unresolvedValue int64) *profilev1.Profile {
		return &profilev1.Profile{
			StringTable: []string{"", "main", "libshared.so", "bid-shared"},
			Mapping: []*profilev1.Mapping{
				{Id: 1}, // dummy, occupies symdb index 0.
				{Id: 2, Filename: 2, BuildId: 3},
			},
			Function: []*profilev1.Function{
				{Id: 1, Name: 1},
			},
			Location: []*profilev1.Location{
				{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
				{Id: 2, MappingId: 2, Address: 0x9000},
			},
			Sample: []*profilev1.Sample{
				{LocationId: []uint64{1}, Value: []int64{mainValue}},
				{LocationId: []uint64{2}, Value: []int64{unresolvedValue}},
			},
			SampleType: []*profilev1.ValueType{{Type: 0, Unit: 0}},
		}
	}

	s := newMemSuite(t, nil)
	indexed0 := s.db.WriteProfileSymbols(0, newPartitionProfile(10, 5))
	indexed1 := s.db.WriteProfileSymbols(1, newPartitionProfile(20, 7))

	r := NewResolver(context.Background(), s.db)
	defer r.Release()
	r.AddSamples(0, indexed0[0].Samples)
	r.AddSamples(1, indexed1[0].Samples)

	tree, rb, err := r.SymbolRefTree()
	require.NoError(t, err)
	assert.Equal(t, int64(42), tree.Total())

	stacks, selves := collectStacks(tree)
	require.Len(t, stacks, 2, "same-content frames from both partitions must coalesce into one node each, not one per partition")

	total := make(map[model.LocationRefName]int64, 2)
	for i, s := range stacks {
		require.Len(t, s, 1)
		total[s[0]] += selves[i]
	}
	var mainSelf, unresolvedSelf int64
	var mainRef, unresolvedRef model.LocationRefName
	for ref, v := range total {
		if ref >= 0 {
			mainSelf, mainRef = v, ref
		} else {
			unresolvedSelf, unresolvedRef = v, ref
		}
	}
	assert.Equal(t, int64(30), mainSelf)
	assert.Equal(t, int64(12), unresolvedSelf)

	pb := &queryv1.SymbolRefTable{}
	mainIdx := rb.KeepRef(mainRef)
	rb.KeepRef(unresolvedRef)
	rb.Build(pb)
	assert.Equal(t, "main", pb.Names[mainIdx])
	unresolved := symbolref.UnresolvedBinaries(pb)
	require.Len(t, unresolved, 1)
	assert.Equal(t, "bid-shared", unresolved[0].BuildID)
	assert.Equal(t, []uint64{0x9000}, unresolved[0].Addresses)
}

// TestResolver_SymbolRefTree_UnresolvedCap verifies the cap guardrail: a
// query whose distinct unresolved locations exceed WithResolverSymbolRefCap
// fails with ErrTooManyUnresolvedLocations instead of returning a partial
// or mixed-representation result, while a query at the cap succeeds.
func TestResolver_SymbolRefTree_UnresolvedCap(t *testing.T) {
	const distinctAddresses = 5

	locs := make([]*profilev1.Location, 0, distinctAddresses)
	samples := make([]*profilev1.Sample, 0, distinctAddresses)
	var wantTotal int64
	for i := 0; i < distinctAddresses; i++ {
		addr := uint64(0x1000 * (i + 1))
		locs = append(locs, &profilev1.Location{Id: uint64(i + 1), MappingId: 2, Address: addr})
		samples = append(samples, &profilev1.Sample{LocationId: []uint64{uint64(i + 1)}, Value: []int64{int64(i + 1)}})
		wantTotal += int64(i + 1)
	}
	p := &profilev1.Profile{
		StringTable: []string{"", "libshared.so", "bid-shared"},
		Mapping: []*profilev1.Mapping{
			{Id: 1}, // dummy, occupies symdb index 0.
			{Id: 2, Filename: 1, BuildId: 2},
		},
		Location:   locs,
		Sample:     samples,
		SampleType: []*profilev1.ValueType{{Type: 0, Unit: 0}},
	}

	s := newMemSuite(t, nil)
	indexed := s.db.WriteProfileSymbols(0, p)

	newResolver := func(refCap int) *Resolver {
		r := NewResolver(context.Background(), s.db, WithResolverSymbolRefCap(refCap))
		t.Cleanup(r.Release)
		r.AddSamples(0, indexed[0].Samples)
		return r
	}

	t.Run("at the cap the query succeeds", func(t *testing.T) {
		tree, _, err := newResolver(distinctAddresses).SymbolRefTree()
		require.NoError(t, err)
		assert.Equal(t, wantTotal, tree.Total())
	})

	t.Run("past the cap the query fails", func(t *testing.T) {
		_, _, err := newResolver(distinctAddresses - 1).SymbolRefTree()
		require.ErrorIs(t, err, ErrTooManyUnresolvedLocations)
	})
}

// TestSymbols_SymbolRefTree_StackTraceSelector verifies call-site selection
// is respected: SymbolRefTree must not silently ignore a
// StackTraceSelector configured on the Resolver it shares state with.
func TestSymbols_SymbolRefTree_StackTraceSelector(t *testing.T) {
	p := &profilev1.Profile{
		StringTable: []string{"", "main", "helper", "other"},
		Mapping:     []*profilev1.Mapping{{Id: 1}},
		Function: []*profilev1.Function{
			{Id: 1, Name: 1},
			{Id: 2, Name: 2},
			{Id: 3, Name: 3},
		},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1}}},
			{Id: 2, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 2}}},
			{Id: 3, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 3}}},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{2, 1}, Value: []int64{5}}, // main -> helper
			{LocationId: []uint64{3}, Value: []int64{7}},    // other, unrelated root
		},
		SampleType: []*profilev1.ValueType{{Type: 0, Unit: 0}},
	}
	symbols, appender := symbolsForProfile(t, p)

	t.Run("matching selector keeps only the selected subtree", func(t *testing.T) {
		sts := &typesv1.StackTraceSelector{CallSite: []*typesv1.Location{{Name: "main"}}}
		table := symbolref.NewTable()
		tree, err := symbols.SymbolRefTree(context.Background(), appender, SelectStackTraces(symbols, sts), table, newUnresolvedCap(0))
		require.NoError(t, err)
		assert.Equal(t, int64(5), tree.Total())
	})

	t.Run("non-matching selector yields an empty tree without error", func(t *testing.T) {
		sts := &typesv1.StackTraceSelector{CallSite: []*typesv1.Location{{Name: "no-such-function"}}}
		table := symbolref.NewTable()
		tree, err := symbols.SymbolRefTree(context.Background(), appender, SelectStackTraces(symbols, sts), table, newUnresolvedCap(0))
		require.NoError(t, err)
		assert.Equal(t, int64(0), tree.Total())
	})
}

// TestSymbols_SymbolRefTree_UnresolvableStacktrace covers
// StacktraceResolver's documented contract that an unresolvable stacktrace
// resolves to zero locations: SymbolRefTree must return an empty tree, not
// panic or error.
func TestSymbols_SymbolRefTree_UnresolvableStacktrace(t *testing.T) {
	p := &profilev1.Profile{
		StringTable: []string{""},
		Mapping:     []*profilev1.Mapping{{Id: 1}},
		Sample:      []*profilev1.Sample{{LocationId: nil, Value: []int64{1}}},
		SampleType:  []*profilev1.ValueType{{Type: 0, Unit: 0}},
	}
	symbols, appender := symbolsForProfile(t, p)
	table := symbolref.NewTable()
	tree, err := symbols.SymbolRefTree(context.Background(), appender, SelectStackTraces(symbols, nil), table, newUnresolvedCap(0))
	require.NoError(t, err)
	assert.Equal(t, int64(0), tree.Total())
}

// A pair interned before the cap filled stays allowed; any new pair past
// the cap is rejected, and rejected pairs are not recorded (that would
// grow memory without bound).
func TestUnresolvedCap_Allow(t *testing.T) {
	c := newUnresolvedCap(1)
	assert.True(t, c.allow("bid", 0x1))
	assert.True(t, c.allow("bid", 0x1), "an interned pair stays allowed after the cap fills")
	assert.False(t, c.allow("bid", 0x2))
	assert.False(t, c.allow("bid", 0x2))
}
