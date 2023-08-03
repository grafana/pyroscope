package symdb

import (
	"context"

	"github.com/grafana/pyroscope/pkg/iter"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type SymbolsWriter interface {
	// AppendStacktraces appends the stack traces,
	// and writes the allocated identifiers into dst:
	// len(dst) must be equal to len(s).
	// The leaf is at locations[0].
	AppendStacktraces([]uint32, []*schemav1.Stacktrace)
	AppendLocations([]uint32, []*schemav1.InMemoryLocation)
	AppendMappings([]uint32, []*schemav1.InMemoryMapping)
	AppendFunctions([]uint32, []*schemav1.InMemoryFunction)
	AppendStrings([]uint32, []string)
}

type SymbolsReader interface {
	// ResolveStacktraces resolves locations for each stack trace
	// and inserts it to the StacktraceInserter provided.
	//
	// The stacktraces must be ordered in the ascending order.
	// If a stacktrace can't be resolved, dst receives an empty
	// array of locations.
	//
	// Stacktraces slice might be modified during the call.
	ResolveStacktraces(ctx context.Context, dst StacktraceInserter, stacktraces []uint32) error
	Locations(context.Context, iter.Iterator[uint32]) (iter.Iterator[*schemav1.InMemoryLocation], error)
	Mappings(context.Context, iter.Iterator[uint32]) (iter.Iterator[*schemav1.InMemoryMapping], error)
	Functions(context.Context, iter.Iterator[uint32]) (iter.Iterator[*schemav1.InMemoryFunction], error)
	Strings(context.Context, iter.Iterator[uint32]) (iter.Iterator[string], error)
	WriteStats(*Stats)
}

type Stats struct {
	StacktracesTotal int
	MaxStacktraceID  int
	LocationsTotal   int
	MappingsTotal    int
	FunctionsTotal   int
	StringsTotal     int
}

// StacktraceInserter accepts resolved locations for a given stack trace.
// The leaf is at locations[0].
//
// Locations slice must not be retained by implementation.
// It is guaranteed, that for a given stacktrace ID
// InsertStacktrace is called not more than once.
type StacktraceInserter interface {
	InsertStacktrace(stacktraceID uint32, locations []int32)
}

type StacktraceInserterFn func(stacktraceID uint32, locations []int32)

func (fn StacktraceInserterFn) InsertStacktrace(stacktraceID uint32, locations []int32) {
	fn(stacktraceID, locations)
}
