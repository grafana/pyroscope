package symdb

import (
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// CallTreeNodeValues represents statistics associated with a call tree node.
type CallTreeNodeValues struct {
	// Flat is the sum of sample values directly attributed to the node.
	Flat uint64
	// Total is the total sum of sample values attributed to the node and
	// its descendants.
	Total uint64
	// LocationFlat is the sum of sample values directly attributed to the
	// node location, irrespectively of the call chain.
	LocationFlat uint64
	// LocationTotal is the total sum of sample values attributed to the
	// node location and its descendants, irrespectively of the call chain.
	LocationTotal uint64
}

// stackTraceLocationRelation represents the relation between a stack trace
// and a location, according to the stack trace selector parameters.
type stackTraceLocationRelation uint8

const (
	// relationSubtree indicates whether the stack trace belongs
	// to the subtree specified by the stack trace selector.
	relationSubtree stackTraceLocationRelation = 1 << iota
	// relationLeaf specifies that the stack trace leaf is the
	// location specified by the stack trace selector,
	// irrespectively of the call chain.
	relationLeaf
	// relationNode specifies that the stack trace includes
	// the location specified by the stack trace selector,
	// irrespectively of the call chain.
	relationNode
)

type SelectedStackTraces struct {
	symbols     *Symbols
	relations   map[uint32]stackTraceLocationRelation
	subtree     []uint32
	subtreeRoot uint32
	subtreeLen  uint32
	buf         []uint64
}

func SelectStackTraces(symbols *Symbols, sts *typesv1.StackTraceSelector) *SelectedStackTraces {
	x := &SelectedStackTraces{
		symbols:   symbols,
		subtree:   findSubtreeRoot(symbols, sts.GetSubtreeRoot()),
		relations: make(map[uint32]stackTraceLocationRelation, len(symbols.Locations)),
	}
	if x.subtreeLen = uint32(len(x.subtree)); x.subtreeLen > 0 {
		x.subtreeRoot = x.subtree[x.subtreeLen-1]
	}
	return x
}

// Values writes the call tree node statistics for the
// selected stack traces and the given set of samples.
func (x *SelectedStackTraces) Values(m *CallTreeNodeValues, samples schemav1.Samples) {
	if x.subtreeLen == 0 {
		return
	}
	for i, sid := range samples.StacktraceIDs {
		v := samples.Values[i]
		r, ok := x.relations[sid]
		if !ok {
			x.buf = x.symbols.Stacktraces.LookupLocations(x.buf, sid)
			r = x.appendStackTrace(x.buf)
			x.relations[sid] = r
		}
		s := uint64(r & relationSubtree)
		l := uint64(r&relationLeaf) >> 1
		n := uint64(r&relationNode) >> 2
		m.LocationTotal += v * (n | l)
		m.LocationFlat += v * l
		m.Total += v * s
		m.Flat += v * s * l
	}
}

func (x *SelectedStackTraces) appendStackTrace(locations []uint64) stackTraceLocationRelation {
	if len(locations) == 0 {
		return 0
	}
	var n uint32 // Number of times subtree root function seen.
	var pos uint32
	var l uint32
	for i := len(locations) - 1; i >= 0; i-- {
		lines := x.symbols.Locations[locations[i]].Line
		for j := len(lines) - 1; j >= 0; j-- {
			f := lines[j].FunctionId
			n += eq(x.subtreeRoot, f)
			if pos < x.subtreeLen && pos == l {
				pos += eq(x.subtree[pos], f)
			}
			l++
		}
	}
	if n == 0 {
		return 0
	}
	leaf := x.symbols.Locations[locations[0]].Line[0]
	isLeaf := eq(x.subtreeRoot, leaf.FunctionId)
	inSubtree := ge(pos, x.subtreeLen)
	return stackTraceLocationRelation(inSubtree | isLeaf<<1 | (1-isLeaf)<<2)
}

func eq(a, b uint32) uint32 {
	if a == b {
		return 1
	}
	return 0
}

func ge(a, b uint32) uint32 {
	if a >= b {
		return 1
	}
	return 0
}

// findSubtreeRoot returns the stack trace of the subtree root for
// Each element in the stack trace is represented by the function ID,
// the root location is the last element.
func findSubtreeRoot(symbols *Symbols, locations []*typesv1.Location) []uint32 {
	if len(locations) == 0 {
		return nil
	}
	m := make(map[string]uint32, len(locations))
	for _, loc := range locations {
		m[loc.Name] = 0
	}
	c := len(m) // Only count unique names.
	for f := 0; f < len(symbols.Functions) && c > 0; f++ {
		s := symbols.Strings[symbols.Functions[f].Name]
		if _, ok := m[s]; ok {
			// We assume that no functions have the same name.
			// Otherwise, the last one takes precedence.
			m[s] = uint32(f) // f is FunctionId
			c--
		}
	}
	if c > 0 {
		return nil
	}
	subtree := make([]uint32, len(locations))
	for i, loc := range locations {
		subtree[i] = m[loc.Name]
	}
	return subtree
}
