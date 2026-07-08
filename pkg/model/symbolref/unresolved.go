package symbolref

import (
	"cmp"
	"slices"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// UnresolvedBinary is one binary's worth of unresolved addresses to resolve.
type UnresolvedBinary struct {
	BuildID    string
	BinaryName string
	Addresses  []uint64 // sorted ascending, deduplicated
}

// UnresolvedBinaries groups a table's unresolved references by build ID,
// one entry per distinct binary referenced. ResultBuilder.Build already
// sorts unresolved entries by (buildID, address), making contiguous-run
// grouping a single forward pass; a table from a different producer may not
// be grouped that way, so UnresolvedBinaries falls back to sorting first
// when it detects that.
func UnresolvedBinaries(pb *queryv1.SymbolRefTable) []UnresolvedBinary {
	buildIdx := pb.GetUnresolvedBuildId()
	addrs := pb.GetUnresolvedAddress()
	if len(buildIdx) == 0 {
		return nil
	}

	if !groupedByBuildID(buildIdx) {
		buildIdx, addrs = sortByBuildIDAndAddress(buildIdx, addrs)
	}

	binaries := make([]UnresolvedBinary, 0, len(pb.GetBuildIds()))
	start := 0
	for i := 1; i <= len(buildIdx); i++ {
		if i < len(buildIdx) && buildIdx[i] == buildIdx[start] {
			continue
		}
		binaries = append(binaries, newUnresolvedBinary(pb, buildIdx[start], addrs[start:i]))
		start = i
	}
	return binaries
}

// groupedByBuildID reports whether equal build ID indices in buildIdx always
// occur in one contiguous run.
func groupedByBuildID(buildIdx []uint32) bool {
	seen := make(map[uint32]struct{}, len(buildIdx))
	for i, bi := range buildIdx {
		if i > 0 && buildIdx[i-1] == bi {
			continue
		}
		if _, ok := seen[bi]; ok {
			return false
		}
		seen[bi] = struct{}{}
	}
	return true
}

func sortByBuildIDAndAddress(buildIdx []uint32, addrs []uint64) ([]uint32, []uint64) {
	type entry struct {
		buildID uint32
		addr    uint64
	}
	entries := make([]entry, len(buildIdx))
	for i := range buildIdx {
		entries[i] = entry{buildIdx[i], addrs[i]}
	}
	slices.SortFunc(entries, func(a, b entry) int {
		return cmp.Or(cmp.Compare(a.buildID, b.buildID), cmp.Compare(a.addr, b.addr))
	})

	outBuildIdx := make([]uint32, len(entries))
	outAddrs := make([]uint64, len(entries))
	for i, e := range entries {
		outBuildIdx[i] = e.buildID
		outAddrs[i] = e.addr
	}
	return outBuildIdx, outAddrs
}

// newUnresolvedBinary builds one UnresolvedBinary from a contiguous run of
// addresses sharing build ID index bi, sorting and deduplicating the
// addresses regardless of the order they arrived in.
func newUnresolvedBinary(pb *queryv1.SymbolRefTable, bi uint32, addrs []uint64) UnresolvedBinary {
	sorted := slices.Clone(addrs)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)

	var buildID, binaryName string
	if int(bi) < len(pb.GetBuildIds()) {
		buildID = pb.GetBuildIds()[bi]
	}
	if int(bi) < len(pb.GetBinaryNames()) {
		binaryName = pb.GetBinaryNames()[bi]
	}

	return UnresolvedBinary{
		BuildID:    buildID,
		BinaryName: binaryName,
		Addresses:  sorted,
	}
}
