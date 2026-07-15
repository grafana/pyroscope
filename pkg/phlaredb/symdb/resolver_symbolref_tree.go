package symdb

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

// SymbolRefTree resolves the tree for the samples in appender into a
// model.LocationRefNameTree whose node names are refs into table.
//
// A location with lines is interned per line into table via InternName (one
// intern per line, including inlined frames), exactly as addFunctionNames
// does for the FunctionName tree.
//
// A line-less location needs symbolization: it is interned via
// InternUnresolved, keyed on the (build ID, address) of its mapping. The
// mapping is read by direct index into symbols.Mappings, exactly as
// copyMappings (resolver_pprof.go) does — in this representation index 0
// is a real mapping, not a "no mapping" sentinel, so it must not be
// special-cased. A location whose mapping carries no build ID cannot be
// symbolized (a genuine no-mapping kernel/JIT frame, or a binary with no
// GNU build ID): it is interned via InternName with the same fallback name
// the legacy symbolizer gives unresolvable frames (binary!0xaddr, keeping
// the binary-name context; see fallbackSymbolName).
//
// capper bounds the number of distinct unresolved entries table accepts
// for this call; a location past the cap fails the call with
// ErrTooManyUnresolvedLocations (see unresolvedCap).
//
// The tree is always built without truncation (maxNodes 0): a tree with
// unresolved entries must not be truncated before those are resolved.
func (r *Symbols) SymbolRefTree(
	ctx context.Context,
	appender *SampleAppender,
	selection *SelectedStackTraces,
	table *symbolref.Table,
	capper *unresolvedCap,
) (*model.LocationRefNameTree, error) {
	if !selection.HasValidCallSite() {
		return &model.LocationRefNameTree{}, nil
	}
	samples := appender.Samples()
	b := newSymbolRefTreeBuilder(r, samples, selection, table, capper)
	if err := r.Stacktraces.ResolveStacktraceLocations(ctx, b, samples.StacktraceIDs); err != nil {
		return nil, err
	}
	if b.err != nil {
		return nil, b.err
	}
	return model.TreeFromStacktraceTree[model.LocationRefName, model.LocationRefNameI](b.tree, 0, b.lookup), nil
}

// ErrTooManyUnresolvedLocations fails a symbol-ref tree build whose number
// of distinct unresolved (build ID, address) locations exceeds the
// configured cap (WithResolverSymbolRefCap).
var ErrTooManyUnresolvedLocations = errors.New("too many unresolved locations in query")

// unresolvedCap bounds the number of distinct (build ID, address) pairs a
// single SymbolRefTree call interns as unresolved entries, so a
// pathological input (e.g. a profile referencing a huge number of distinct
// native addresses) can never make the deferred-resolution work list
// unbounded. Exceeding the cap fails the call: rendering the remainder as
// inline fallback names instead would keep growing the name table without
// bound and make the result depend on arrival order. Safe for concurrent
// use.
type unresolvedCap struct {
	mu   sync.Mutex
	max  int // <= 0 means unlimited.
	seen map[unresolvedCapKey]struct{}
}

type unresolvedCapKey struct {
	buildID string
	addr    uint64
}

func newUnresolvedCap(max int) *unresolvedCap {
	return &unresolvedCap{max: max, seen: make(map[unresolvedCapKey]struct{})}
}

// allow reports whether (buildID, addr) may be interned as an unresolved
// entry: true if the cap is unlimited, the pair was interned before the
// cap was reached, or the cap has not been reached yet; false for any
// other pair once the cap has been reached. Rejected pairs are not
// recorded: remembering them would grow memory without bound, which is
// what the cap exists to prevent.
func (c *unresolvedCap) allow(buildID string, addr uint64) bool {
	if c.max <= 0 {
		return true
	}
	key := unresolvedCapKey{buildID, addr}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seen[key]; ok {
		return true
	}
	if len(c.seen) >= c.max {
		return false
	}
	c.seen[key] = struct{}{}
	return true
}

// symbolRefTreeBuilder is a StacktraceInserter that interns every stack
// frame directly into a shared symbolref.Table and accumulates the result
// into an intermediate model.StacktraceTree.
//
// symbolref.Table refs can be negative (unresolved entries), but
// model.StacktraceTree's Location field, and TreeFromStacktraceTree's
// truncation-sentinel check in particular, treat any negative location as
// the "other" truncation marker. slotOf/slots translate refs into a dense,
// non-negative, per-builder "slot" space so the intermediate tree only
// ever sees non-negative locations; lookup translates slots back into the
// real (possibly negative) refs once the tree's final shape is known.
type symbolRefTreeBuilder struct {
	symbols *Symbols
	samples *schemav1.Samples
	table   *symbolref.Table
	capper  *unresolvedCap
	tree    *model.StacktraceTree
	cur     int
	err     error // first cap breach; set once, the whole call fails.

	refs  []int32 // per-stack ref buffer, as slots.
	lines []int32 // per-stack string-table-index buffer; only used when funcNamesMatcher is set.

	selection        *SelectedStackTraces
	funcNamesMatcher func([]int32) bool

	slotOf map[model.LocationRefName]int32
	slots  []model.LocationRefName
}

func newSymbolRefTreeBuilder(
	symbols *Symbols,
	samples schemav1.Samples,
	selection *SelectedStackTraces,
	table *symbolref.Table,
	capper *unresolvedCap,
) *symbolRefTreeBuilder {
	b := &symbolRefTreeBuilder{
		symbols:   symbols,
		samples:   &samples,
		table:     table,
		capper:    capper,
		tree:      model.NewStacktraceTree(samples.Len() * 2),
		selection: selection,
		slotOf:    make(map[model.LocationRefName]int32),
	}
	if selection != nil && len(selection.callSite) > 0 {
		b.funcNamesMatcher = b.funcNamesMatchSelection
	}
	return b
}

func (b *symbolRefTreeBuilder) InsertStacktrace(_ uint32, locations []int32) {
	if b.err != nil {
		b.cur++
		return
	}
	if b.funcNamesMatcher != nil {
		// The call-site selection is matched against locally resolved
		// function names only, mirroring the FunctionName tree path: a
		// line-less (unsymbolized) location contributes no name here, so a
		// stack whose match depends on such a frame is not selected.
		// Lifting that requires propagating every stack that contains an
		// unsymbolized location and filtering only after symbolization.
		b.lines = b.lines[:0]
		for i := 0; i < len(locations); i++ {
			b.lines = addFunctionNames(b.lines, locations[i], b.symbols)
		}
		if !b.funcNamesMatcher(b.lines) {
			b.cur++
			return
		}
	}
	b.refs = b.refs[:0]
	for i := 0; i < len(locations); i++ {
		b.refs = b.appendLocationRefs(b.refs, locations[i])
	}
	if b.err != nil {
		b.cur++
		return
	}
	b.tree.Insert(b.refs, int64(b.samples.Values[b.cur]))
	b.cur++
}

// funcNamesMatchSelection mirrors treeSymbols.funcNamesMatchSelection
// (resolver_tree.go): funcNames is leaf-first (addFunctionNames' order),
// so the selector's call site (root-first) is checked against funcNames'
// tail.
func (b *symbolRefTreeBuilder) funcNamesMatchSelection(funcNames []int32) bool {
	if len(funcNames) < int(b.selection.depth) {
		return false
	}
	for i := 0; i < int(b.selection.depth); i++ {
		if b.symbols.Strings[funcNames[len(funcNames)-1-i]] != b.selection.callSite[i] {
			return false
		}
	}
	return true
}

// appendLocationRefs appends the slot(s) for locID's symbolref.Table ref(s)
// to dst: see SymbolRefTree's doc comment for the per-location rules, and
// symbolRefTreeBuilder's doc comment for why slots (not raw refs) are used
// here.
func (b *symbolRefTreeBuilder) appendLocationRefs(dst []int32, locID int32) []int32 {
	loc := b.symbols.Locations[locID]
	if len(loc.Line) > 0 {
		for l := 0; l < len(loc.Line); l++ {
			name := b.symbols.Strings[b.symbols.Functions[loc.Line[l].FunctionId].Name]
			dst = append(dst, b.toSlot(b.table.InternName(name)))
		}
		return dst
	}
	var buildID, binaryName string
	if int(loc.MappingId) < len(b.symbols.Mappings) {
		mapping := b.symbols.Mappings[loc.MappingId]
		buildID = b.symbols.Strings[mapping.BuildId]
		binaryName = filepath.Base(b.symbols.Strings[mapping.Filename])
	}
	if buildID == "" {
		return append(dst, b.toSlot(b.table.InternName(fallbackSymbolName(binaryName, loc.Address))))
	}
	if !b.capper.allow(buildID, loc.Address) {
		b.err = fmt.Errorf("%w (limit %d)", ErrTooManyUnresolvedLocations, b.capper.max)
		return dst
	}
	return append(dst, b.toSlot(b.table.InternUnresolved(buildID, binaryName, loc.Address)))
}

func (b *symbolRefTreeBuilder) toSlot(ref model.LocationRefName) int32 {
	if slot, ok := b.slotOf[ref]; ok {
		return slot
	}
	slot := int32(len(b.slots))
	b.slotOf[ref] = slot
	b.slots = append(b.slots, ref)
	return slot
}

func (b *symbolRefTreeBuilder) lookup(slot int32) model.LocationRefName {
	return b.slots[slot]
}

// fallbackSymbolName matches Symbolizer.createFallbackSymbol
// (pkg/symbolizer/symbolizer.go) byte for byte.
func fallbackSymbolName(binaryName string, addr uint64) string {
	prefix := "unknown"
	if binaryName != "" {
		prefix = binaryName
	}
	return fmt.Sprintf("%s!0x%x", prefix, addr)
}
