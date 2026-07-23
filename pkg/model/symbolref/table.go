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
	names    *hashedslice.Slice[string]
	binaries *hashedslice.Slice[binaryKey]

	unresolved    map[unresolvedKey]int32 // (binary, addr) -> unresolved index
	unresolvedBin []int32                 // parallel slices, in intern order
	unresolvedAd  []uint64
}

// binaryKey identifies a binary exactly as stored in the profile data. The
// same build ID may appear under several binary names (e.g. a binary renamed
// between deployments); every (build ID, name) combination is a distinct
// row, so no arrival order can make one stored name overwrite another.
type binaryKey struct {
	buildID string
	name    string
}

func binaryKeyEq(a, b binaryKey) bool { return a == b }

var binaryKeySep = []byte{0}

func (k binaryKey) hash() uint64 {
	var d xxhash.Digest
	d.Reset()
	_, _ = d.WriteString(k.buildID)
	_, _ = d.Write(binaryKeySep) // so ("ab","c") and ("a","bc") hash apart
	_, _ = d.WriteString(k.name)
	return d.Sum64()
}

// unresolvedKey identifies a distinct unresolved location by its interned
// binary row index and address.
type unresolvedKey struct {
	binary int32
	addr   uint64
}

// NewTable returns an empty table. Ref 0 is reserved as an unused sentinel:
// the tree-merge machinery invokes remap functions on synthetic zero-valued
// names (model.Tree.FormatNodeNames visits its virtual root node), and Add
// remaps refs its input does not describe to the reserved ref — so a real
// name must never land on ref 0. InternName's first call for real content
// returns 1, not 0.
func NewTable() *Table {
	return &Table{core: newTableCore()}
}

func newTableCore() tableCore {
	c := tableCore{
		names:      hashedslice.New(stringEq),
		binaries:   hashedslice.New(binaryKeyEq),
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

// InternUnresolved idempotently interns an unresolved location, keyed on the
// (buildID, binaryName, addr) triple. The binary name is part of the key —
// the same build ID under two different names yields two distinct refs — so
// every name is retained exactly as stored and the table's content is
// independent of intern or merge arrival order.
func (t *Table) InternUnresolved(buildID, binaryName string, addr uint64) model.LocationRefName {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.core.internUnresolved(buildID, binaryName, addr)
}

func (c *tableCore) internUnresolved(buildID, binaryName string, addr uint64) model.LocationRefName {
	bin := c.internBinary(binaryKey{buildID: buildID, name: binaryName})
	return c.internUnresolvedRef(bin, addr)
}

// internUnresolvedRef interns (bin, addr), where bin is already an interned
// binary row index.
func (c *tableCore) internUnresolvedRef(bin int32, addr uint64) model.LocationRefName {
	key := unresolvedKey{binary: bin, addr: addr}
	if idx, ok := c.unresolved[key]; ok {
		return c.unresolvedRef(idx)
	}
	idx := int32(len(c.unresolvedBin))
	c.unresolved[key] = idx
	c.unresolvedBin = append(c.unresolvedBin, bin)
	c.unresolvedAd = append(c.unresolvedAd, addr)
	return c.unresolvedRef(idx)
}

func (c *tableCore) internBinary(k binaryKey) int32 {
	return c.binaries.Add(k.hash(), k)
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
	return len(c.unresolvedBin) > 0
}

// UnresolvedCount reports the number of distinct unresolved locations
// interned in t, whether via InternUnresolved directly or absorbed via Add.
func (t *Table) UnresolvedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.core.unresolvedBin)
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

	binRemap := make([]int32, len(pb.GetBuildIds()))
	binaryNames := pb.GetBinaryNames()
	for i, buildID := range pb.GetBuildIds() {
		binRemap[i] = c.internBinary(binaryKey{buildID: buildID, name: binaryNames[i]})
	}

	numNames := len(pb.GetNames())
	unresolvedAddr := pb.GetUnresolvedAddress()
	unresolvedRemap := make([]model.LocationRefName, len(unresolvedAddr))
	for i, addr := range unresolvedAddr {
		bin := binRemap[pb.GetUnresolvedBuildId()[i]]
		unresolvedRemap[i] = c.internUnresolvedRef(bin, addr)
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
// (buildID, binaryName, address) sort order over every unresolved entry
// interned so far — so that ResultBuilder holds no reference back to
// tableCore and can safely be used without holding Table's lock.
func (c *tableCore) newResultBuilder() *ResultBuilder {
	n := len(c.unresolvedBin)
	sorted := make([]int32, n)
	for i := range sorted {
		sorted[i] = int32(i)
	}
	slices.SortFunc(sorted, func(a, b int32) int {
		ka, kb := c.binaries.Values[c.unresolvedBin[a]], c.binaries.Values[c.unresolvedBin[b]]
		return cmp.Or(
			cmp.Compare(ka.buildID, kb.buildID),
			cmp.Compare(ka.name, kb.name),
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
		binaries:         c.binaries.Values,
		unresolvedBin:    c.unresolvedBin,
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

	namesSl  []string    // snapshot of the table's interned names
	binaries []binaryKey // snapshot of the table's interned (build ID, binary name) rows

	unresolvedBin []int32  // snapshot: binary row index per unresolved entry
	unresolvedAd  []uint64 // snapshot: address per unresolved entry

	sortedUnresolved []int32 // permutation of [0, len(unresolvedBin)), sorted by (buildID, binaryName, address)
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
// pass without breaking the (buildID, binaryName, address) wire order.
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

// NameOf returns ref's display name from the snapshot: the interned name of
// a resolved ref, or the FallbackSymbolName rendering of an unresolved
// entry's (binary name, address). Refs the snapshot does not describe —
// model.OtherLocationRef and out-of-range values — render as the empty
// string; a caller that special-cases the truncation sentinel must do so
// before calling. ref is Table's internal encoding, as with KeepRef.
func (rb *ResultBuilder) NameOf(ref model.LocationRefName) string {
	switch {
	case ref >= 0 && int(ref) < rb.namesLen:
		return rb.namesSl[ref]
	case ref <= -2:
		if idx := -2 - int(ref); idx < len(rb.unresolvedBin) {
			b := rb.binaries[rb.unresolvedBin[idx]]
			return FallbackSymbolName(b.name, rb.unresolvedAd[idx])
		}
	}
	return ""
}

// Build writes pb.Names, pb.BuildIds, pb.BinaryNames, pb.UnresolvedBuildId
// and pb.UnresolvedAddress from the snapshot and returns pb, allocating the
// returned queryv1.SymbolRefTable when pb == nil. Every interned name is
// written, in intern order, so resolved wire refs are the table's own name
// indices and len(pb.Names) always equals the offset KeepRef applies to
// unresolved refs; unresolved entries are sorted by (buildID, binaryName,
// address), which is what makes the unresolved side of the wire encoding
// independent of intern or merge arrival order. Equal binary rows form one
// contiguous run each under that order, so build_ids/binary_names carry
// exactly one row per distinct (build ID, binary name) pair referenced.
func (rb *ResultBuilder) Build(pb *queryv1.SymbolRefTable) *queryv1.SymbolRefTable {
	if pb == nil {
		pb = &queryv1.SymbolRefTable{}
	}

	pb.Names = make([]string, rb.namesLen)
	copy(pb.Names, rb.namesSl)

	n := len(rb.sortedUnresolved)
	// Reset any rows a reused pb may carry.
	pb.BuildIds = nil
	pb.BinaryNames = nil
	pb.UnresolvedBuildId = make([]uint32, n)
	pb.UnresolvedAddress = make([]uint64, n)

	last := int32(-1)
	var row uint32
	for out, idx := range rb.sortedUnresolved {
		if bin := rb.unresolvedBin[idx]; bin != last {
			row = uint32(len(pb.BuildIds))
			b := rb.binaries[bin]
			pb.BuildIds = append(pb.BuildIds, b.buildID)
			pb.BinaryNames = append(pb.BinaryNames, b.name)
			last = bin
		}
		pb.UnresolvedBuildId[out] = row
		pb.UnresolvedAddress[out] = rb.unresolvedAd[idx]
	}

	return pb
}
