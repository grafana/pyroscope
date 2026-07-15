package symbolref

import (
	"cmp"
	"fmt"
	"slices"
	"sync"

	"github.com/cespare/xxhash/v2"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/util/hashedslice"
)

// Table is a content-addressed intern table pairing a model.LocationRefNameTree
// with the wire-format queryv1.SymbolRefTable it represents. Safe for
// concurrent use: every method locks an internal mutex before touching its
// state.
type Table struct {
	mu   sync.Mutex
	core tableCore
}

// tableCore is Table's lock-free state; it must only be reached through
// Table, which guards every access with its mutex.
type tableCore struct {
	names   *hashedslice.Slice[string]
	buildID *hashedslice.Slice[string]

	binaryNames []string // parallel to buildID.Values; first-writer-wins

	unresolved   map[unresolvedKey]int32 // (buildID, addr) -> unresolved index
	unresolvedBI []int32                 // parallel slices, in intern order
	unresolvedAd []uint64
}

// unresolvedKey identifies a distinct unresolved location by its interned
// build ID index and address.
type unresolvedKey struct {
	buildID int32
	addr    uint64
}

// NewTable returns an empty table. Ref 0 is reserved as an unused sentinel:
// pkg/model's Tree marshal format introduces a spurious zero-valued frame at
// the root of every stack in a tree that has gone through a merge, and
// reserving ref 0 keeps that frame distinguishable from a genuine name — so
// InternName's first call for real content returns 1, not 0.
func NewTable() *Table {
	return &Table{core: newTableCore()}
}

func newTableCore() tableCore {
	c := tableCore{
		names:      hashedslice.New(stringEq),
		buildID:    hashedslice.New(stringEq),
		unresolved: make(map[unresolvedKey]int32),
	}
	c.internName("")
	return c
}

func stringEq(a, b string) bool { return a == b }

// InternName idempotently interns a resolved frame name: the same name
// always returns the same ref, and a name not seen before is assigned a new
// one.
func (t *Table) InternName(name string) model.LocationRefName {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.core.internName(name)
}

func (c *tableCore) internName(name string) model.LocationRefName {
	return model.LocationRefName(c.names.Add(xxhash.Sum64String(name), name))
}

// InternUnresolved idempotently interns an unresolved (buildID, addr)
// location, keyed on the pair. binaryName is recorded the first time a
// given buildID is seen; later calls for the same buildID keep that first
// binaryName even if a different value is passed.
func (t *Table) InternUnresolved(buildID, binaryName string, addr uint64) model.LocationRefName {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.core.internUnresolved(buildID, binaryName, addr)
}

func (c *tableCore) internUnresolved(buildID, binaryName string, addr uint64) model.LocationRefName {
	bi := c.internBuildID(buildID, binaryName)
	return c.internUnresolvedRef(bi, addr)
}

// internUnresolvedRef interns (bi, addr), where bi is already an interned
// build ID index.
func (c *tableCore) internUnresolvedRef(bi int32, addr uint64) model.LocationRefName {
	key := unresolvedKey{buildID: bi, addr: addr}
	if idx, ok := c.unresolved[key]; ok {
		return c.unresolvedRef(idx)
	}
	idx := int32(len(c.unresolvedBI))
	c.unresolved[key] = idx
	c.unresolvedBI = append(c.unresolvedBI, bi)
	c.unresolvedAd = append(c.unresolvedAd, addr)
	return c.unresolvedRef(idx)
}

// internBuildID interns buildID (deduplicated) and records binaryName the
// first time a given buildID is seen; later calls for the same buildID
// keep the first binaryName.
func (c *tableCore) internBuildID(buildID, binaryName string) int32 {
	before := c.buildID.Len()
	bi := c.buildID.Add(xxhash.Sum64String(buildID), buildID)
	if int(bi) == before {
		c.binaryNames = append(c.binaryNames, binaryName)
	}
	return bi
}

// unresolvedRef converts an unresolved entry's slot index into Table's
// internal ref representation, model.LocationRefName(-2-idx) — i.e. -2, -3,
// -4, ... — disjoint from resolved refs (always >= 0) and from
// model.OtherLocationRef (-1), and stable regardless of how many more names
// are interned afterward. This is Table's internal encoding; ResultBuilder
// translates it into the wire encoding once the table is frozen.
func (c *tableCore) unresolvedRef(idx int32) model.LocationRefName {
	return model.LocationRefName(-2 - int(idx))
}

// HasUnresolved reports whether any unresolved entry has ever landed in t,
// whether via InternUnresolved directly or absorbed via Add.
func (t *Table) HasUnresolved() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.core.hasUnresolved()
}

func (c *tableCore) hasUnresolved() bool {
	return len(c.unresolvedBI) > 0
}

// Add merges pb into t, returning a remap function from pb's ref space into
// t's ref space, suitable for model.WithTreeMergeFormatNodeNames. The
// returned remap passes model.OtherLocationRef through unchanged and maps
// every other ref pb does not describe — any other negative, or any
// non-negative when pb is nil or empty — to the reserved ref 0; err is
// non-nil only for structurally malformed input, never to signal "nothing
// to merge".
func (t *Table) Add(pb *queryv1.SymbolRefTable) (func(model.LocationRefName) model.LocationRefName, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.core.add(pb)
}

func (c *tableCore) add(pb *queryv1.SymbolRefTable) (func(model.LocationRefName) model.LocationRefName, error) {
	if err := validateSymbolRefTable(pb); err != nil {
		return nil, err
	}

	nameRemap := make([]int32, len(pb.GetNames()))
	for i, name := range pb.GetNames() {
		nameRemap[i] = c.names.Add(xxhash.Sum64String(name), name)
	}

	buildIDRemap := make([]int32, len(pb.GetBuildIds()))
	binaryNames := pb.GetBinaryNames()
	for i, buildID := range pb.GetBuildIds() {
		buildIDRemap[i] = c.internBuildID(buildID, binaryNames[i])
	}

	numNames := len(pb.GetNames())
	unresolvedAddr := pb.GetUnresolvedAddress()
	unresolvedRemap := make([]model.LocationRefName, len(unresolvedAddr))
	for i, addr := range unresolvedAddr {
		bi := buildIDRemap[pb.GetUnresolvedBuildId()[i]]
		unresolvedRemap[i] = c.internUnresolvedRef(bi, addr)
	}

	return func(in model.LocationRefName) model.LocationRefName {
		if in == model.OtherLocationRef {
			return in
		}
		if in >= 0 && int(in) < numNames {
			return model.LocationRefName(nameRemap[in])
		}
		if u := int(in) - numNames; in >= 0 && u < len(unresolvedRemap) {
			return unresolvedRemap[u]
		}
		// A ref pb does not describe degrades to the reserved ref 0: a
		// non-negative ref beyond pb's ranges (nil/empty or truncated input
		// from a skewed peer) would panic, and a negative ref other than
		// OtherLocationRef would alias t's internal unresolved encoding.
		return 0
	}, nil
}

// validateSymbolRefTable rejects structurally malformed input: mismatched
// parallel-slice lengths or an out-of-range build ID index.
func validateSymbolRefTable(pb *queryv1.SymbolRefTable) error {
	if pb == nil {
		return nil
	}
	if len(pb.GetBinaryNames()) != len(pb.GetBuildIds()) {
		return fmt.Errorf("symbolref: binary_names length %d does not match build_ids length %d",
			len(pb.GetBinaryNames()), len(pb.GetBuildIds()))
	}
	if len(pb.GetUnresolvedBuildId()) != len(pb.GetUnresolvedAddress()) {
		return fmt.Errorf("symbolref: unresolved_build_id length %d does not match unresolved_address length %d",
			len(pb.GetUnresolvedBuildId()), len(pb.GetUnresolvedAddress()))
	}
	for _, bi := range pb.GetUnresolvedBuildId() {
		if int(bi) >= len(pb.GetBuildIds()) {
			return fmt.Errorf("symbolref: unresolved_build_id %d out of range (len=%d)", bi, len(pb.GetBuildIds()))
		}
	}
	return nil
}

// ResultBuilder returns a fresh ResultBuilder snapshotting t for one marshal
// pass.
func (t *Table) ResultBuilder() *ResultBuilder {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.core.newResultBuilder()
}

// newResultBuilder snapshots the state a ResultBuilder needs — including the
// (buildID, address) sort order over every unresolved entry interned so
// far — so that ResultBuilder holds no reference back to tableCore and can
// safely be used without holding Table's lock.
func (c *tableCore) newResultBuilder() *ResultBuilder {
	n := len(c.unresolvedBI)
	sorted := make([]int32, n)
	for i := range sorted {
		sorted[i] = int32(i)
	}
	slices.SortFunc(sorted, func(a, b int32) int {
		return cmp.Or(
			cmp.Compare(c.buildID.Values[c.unresolvedBI[a]], c.buildID.Values[c.unresolvedBI[b]]),
			cmp.Compare(c.unresolvedAd[a], c.unresolvedAd[b]),
		)
	})
	rank := make([]int32, n)
	for pos, idx := range sorted {
		rank[idx] = int32(pos)
	}

	return &ResultBuilder{
		namesLen:         c.names.Len(),
		namesSl:          c.names.Values,
		buildIDSl:        c.buildID.Values,
		binaryNames:      c.binaryNames,
		unresolvedBI:     c.unresolvedBI,
		unresolvedAd:     c.unresolvedAd,
		sortedUnresolved: sorted,
		rank:             rank,
	}
}

// ResultBuilder encodes a Table's contents into a queryv1.SymbolRefTable at
// marshal time. It is built from a point-in-time snapshot of its Table's
// state, taken under the Table's lock (see tableCore.newResultBuilder);
// ResultBuilder itself holds no reference back to the Table and takes no
// lock of its own, so it is not safe for concurrent use — it is meant to be
// driven single-threaded, once, after every Add/Intern* call on its Table
// has completed.
type ResultBuilder struct {
	namesLen int // snapshot of len(names) at construction; the resolved/unresolved dividing line

	namesSl     []string // snapshot of the table's interned names
	buildIDSl   []string // snapshot of the table's interned build IDs
	binaryNames []string // snapshot of the table's per-build-ID binary names

	unresolvedBI []int32  // snapshot: build ID index per unresolved entry
	unresolvedAd []uint64 // snapshot: address per unresolved entry

	sortedUnresolved []int32 // permutation of [0, len(unresolvedBI)), sorted by (buildID, address)
	rank             []int32 // rank[idx] = position of idx within sortedUnresolved
}

// KeepRef returns ref's wire encoding, for use as the keepName callback to
// Tree.Bytes/MarshalTruncate. ref is Table's internal encoding (resolved:
// >= 0; model.OtherLocationRef (-1): passed through unchanged; unresolved:
// <= -2, see tableCore.unresolvedRef).
//
// Wire refs are assigned from the snapshot alone — resolved refs keep their
// table index, unresolved refs are offset by the snapshot's name count —
// and Build writes the full snapshot, so the encoding does not depend on
// which refs the marshaled tree keeps. Unresolved wire refs must be final
// the moment they are first returned, while the kept set is still unknown,
// so a kept-set-compacted encoding could not be assigned in this single
// pass without breaking the (buildID, address) wire order.
func (rb *ResultBuilder) KeepRef(ref model.LocationRefName) model.LocationRefName {
	switch {
	case ref == model.OtherLocationRef:
		return ref
	case ref >= 0:
		return ref
	default:
		idx := -2 - int(ref)
		return model.LocationRefName(rb.namesLen + int(rb.rank[idx]))
	}
}

// Build writes pb.Names, pb.BuildIds, pb.BinaryNames, pb.UnresolvedBuildId
// and pb.UnresolvedAddress from the snapshot and returns pb, allocating the
// returned queryv1.SymbolRefTable when pb == nil. Every interned name is
// written, in intern order, so resolved wire refs are the table's own name
// indices and len(pb.Names) always equals the offset KeepRef applies to
// unresolved refs; unresolved entries are sorted by (buildID, address),
// which is what makes the unresolved side of the wire encoding independent
// of intern or merge arrival order.
func (rb *ResultBuilder) Build(pb *queryv1.SymbolRefTable) *queryv1.SymbolRefTable {
	if pb == nil {
		pb = &queryv1.SymbolRefTable{}
	}

	pb.Names = make([]string, rb.namesLen)
	copy(pb.Names, rb.namesSl)

	n := len(rb.sortedUnresolved)
	pb.BuildIds = make([]string, 0, n)
	pb.BinaryNames = make([]string, 0, n)
	pb.UnresolvedBuildId = make([]uint32, n)
	pb.UnresolvedAddress = make([]uint64, n)

	buildIDOut := make(map[int32]int32, n)
	for out, idx := range rb.sortedUnresolved {
		bi := rb.unresolvedBI[idx]
		biOut, ok := buildIDOut[bi]
		if !ok {
			biOut = int32(len(pb.BuildIds))
			buildIDOut[bi] = biOut
			pb.BuildIds = append(pb.BuildIds, rb.buildIDSl[bi])
			pb.BinaryNames = append(pb.BinaryNames, rb.binaryNames[bi])
		}
		pb.UnresolvedBuildId[out] = uint32(biOut)
		pb.UnresolvedAddress[out] = rb.unresolvedAd[idx]
	}

	return pb
}
