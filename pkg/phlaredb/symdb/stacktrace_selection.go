package symdb

import (
	"github.com/parquet-go/parquet-go"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

// CallSiteValues represents statistics associated with a call tree node.
type CallSiteValues struct {
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
	// to the callSite specified by the stack trace selector.
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
	symbols *Symbols
	// Go PGO filter.
	gopgo *typesv1.GoPGO
	// Call site filter
	relations        map[uint32]stackTraceLocationRelation
	callSiteSelector []*typesv1.Location
	callSite         []string // call site strings in the original order.
	location         string   // stack trace leaf function.
	depth            uint32
	buf              []uint64
	// Function ID => name. The lookup table is used to
	// avoid unnecessary indirect accesses through the
	// strings[functions[id].Name] path. Instead, the
	// name can be resolved directly funcNames[id].
	funcNames []string
}

func SelectStackTraces(symbols *Symbols, selector *typesv1.StackTraceSelector) *SelectedStackTraces {
	x := &SelectedStackTraces{
		symbols:          symbols,
		callSiteSelector: selector.GetCallSite(),
		gopgo:            selector.GetGoPgo(),
	}
	x.callSite = callSiteFunctions(x.callSiteSelector)
	if x.depth = uint32(len(x.callSite)); x.depth > 0 {
		x.location = x.callSite[x.depth-1]
	}
	x.funcNames = make([]string, len(symbols.Functions))
	for i, f := range symbols.Functions {
		x.funcNames[i] = symbols.Strings[f.Name]
	}
	return x
}

// HasValidCallSite reports whether any stack traces match the selector.
// An empty selector results in a valid empty selection.
func (x *SelectedStackTraces) HasValidCallSite() bool {
	return len(x.callSiteSelector) == 0 || len(x.callSiteSelector) != 0 && len(x.callSite) != 0
}

// CallSiteValues writes the call site statistics for
// the selected stack traces and the given set of samples.
func (x *SelectedStackTraces) CallSiteValues(values *CallSiteValues, samples schemav1.Samples) {
	*values = CallSiteValues{}
	if x.depth == 0 {
		return
	}
	if x.relations == nil {
		// relations will grow to the size of the number of stack traces:
		// if a stack trace does not belong to the selection, we still
		// have to memoize the negative result.
		x.relations = make(map[uint32]stackTraceLocationRelation, len(x.symbols.Locations))
	}
	for i, sid := range samples.StacktraceIDs {
		v := samples.Values[i]
		r, ok := x.relations[sid]
		if !ok {
			x.buf = x.symbols.Stacktraces.LookupLocations(x.buf, sid)
			r = x.appendStackTrace(x.buf)
			x.relations[sid] = r
		}
		x.write(values, v, r)
	}
}

// CallSiteValuesParquet is identical to CallSiteValues
// but accepts raw parquet values instead of samples.
func (x *SelectedStackTraces) CallSiteValuesParquet(values *CallSiteValues, stacktraceID, value []parquet.Value) {
	*values = CallSiteValues{}
	if x.depth == 0 {
		return
	}
	if x.relations == nil {
		// relations will grow to the size of the number of stack traces:
		// if a stack trace does not belong to the selection, we still
		// have to memoize the negative result.
		x.relations = make(map[uint32]stackTraceLocationRelation, len(x.symbols.Locations))
	}
	for i, pv := range stacktraceID {
		sid := pv.Uint32()
		v := value[i].Uint64()
		r, ok := x.relations[sid]
		if !ok {
			x.buf = x.symbols.Stacktraces.LookupLocations(x.buf, sid)
			r = x.appendStackTrace(x.buf)
			x.relations[sid] = r
		}
		x.write(values, v, r)
	}
}

func (x *SelectedStackTraces) write(m *CallSiteValues, v uint64, r stackTraceLocationRelation) {
	s := uint64(r & relationSubtree)
	l := uint64(r&relationLeaf) >> 1
	n := uint64(r&relationNode) >> 2
	m.LocationTotal += v * (n | l)
	m.LocationFlat += v * l
	m.Total += v * s
	m.Flat += v * s * l
}

func (x *SelectedStackTraces) appendStackTrace(locations []uint64) stackTraceLocationRelation {
	if len(locations) == 0 {
		return 0
	}
	var n uint32 // Number of times callSite root function seen.
	var pos uint32
	var l uint32
	for i := len(locations) - 1; i >= 0; i-- {
		lines := x.symbols.Locations[locations[i]].Line
		for j := len(lines) - 1; j >= 0; j-- {
			f := lines[j].FunctionId
			if x.location == x.funcNames[f] {
				n++
			}
			if pos < x.depth && pos == l && x.callSite[pos] == x.funcNames[f] {
				pos++
			}
			l++
		}
	}
	if n == 0 {
		return 0
	}
	var isLeaf uint32
	leaf := x.symbols.Locations[locations[0]].Line[0]
	if x.location == x.funcNames[leaf.FunctionId] {
		isLeaf = 1
	}
	var inSubtree uint32
	if pos >= x.depth {
		inSubtree = 1
	}
	return stackTraceLocationRelation(inSubtree | isLeaf<<1 | (1-isLeaf)<<2)
}

func callSiteFunctions(locations []*typesv1.Location) []string {
	callSite := make([]string, len(locations))
	for i, loc := range locations {
		callSite[i] = loc.Name
	}
	return callSite
}
