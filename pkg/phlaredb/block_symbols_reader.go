package phlaredb

import (
	"context"

	"github.com/grafana/pyroscope/pkg/iter"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

// TODO(kolesnikovae): Refactor to symdb.

type SymbolsReader interface {
	SymbolsResolver(partition uint64) (SymbolsResolver, error)
}

type SymbolsResolver interface {
	ResolveStacktraces(ctx context.Context, dst symdb.StacktraceInserter, stacktraces []uint32) error

	Locations(iter.Iterator[uint32]) iter.Iterator[*schemav1.InMemoryLocation]
	Mappings(iter.Iterator[uint32]) iter.Iterator[*schemav1.InMemoryMapping]
	Functions(iter.Iterator[uint32]) iter.Iterator[*schemav1.InMemoryFunction]
	Strings(iter.Iterator[uint32]) iter.Iterator[string]

	WriteStats(*symdb.Stats)
}

type inMemorySymbolsReader struct {
	partitions map[uint64]*inMemorySymbolsResolver

	// TODO(kolesnikovae): Split into partitions.
	strings     inMemoryparquetReader[string, *schemav1.StringPersister]
	functions   inMemoryparquetReader[*schemav1.InMemoryFunction, *schemav1.FunctionPersister]
	locations   inMemoryparquetReader[*schemav1.InMemoryLocation, *schemav1.LocationPersister]
	mappings    inMemoryparquetReader[*schemav1.InMemoryMapping, *schemav1.MappingPersister]
	stacktraces StacktraceDB
}

func (r *inMemorySymbolsReader) SymbolsResolver(partition uint64) (SymbolsResolver, error) {
	p, ok := r.partitions[partition]
	if !ok {
		p = &inMemorySymbolsResolver{
			partition: partition,
			reader:    r,
		}
		r.partitions[partition] = p
	}
	return p, nil
}

type inMemorySymbolsResolver struct {
	partition uint64
	reader    *inMemorySymbolsReader
}

func (s inMemorySymbolsResolver) ResolveStacktraces(ctx context.Context, dst symdb.StacktraceInserter, stacktraces []uint32) error {
	return s.reader.stacktraces.Resolve(ctx, s.partition, dst, stacktraces)
}

func (s inMemorySymbolsResolver) Locations(i iter.Iterator[uint32]) iter.Iterator[*schemav1.InMemoryLocation] {
	return iter.NewSliceIndexIterator(s.reader.locations.cache, i)
}

func (s inMemorySymbolsResolver) Mappings(i iter.Iterator[uint32]) iter.Iterator[*schemav1.InMemoryMapping] {
	return iter.NewSliceIndexIterator(s.reader.mappings.cache, i)
}

func (s inMemorySymbolsResolver) Functions(i iter.Iterator[uint32]) iter.Iterator[*schemav1.InMemoryFunction] {
	return iter.NewSliceIndexIterator(s.reader.functions.cache, i)
}

func (s inMemorySymbolsResolver) Strings(i iter.Iterator[uint32]) iter.Iterator[string] {
	return iter.NewSliceIndexIterator(s.reader.strings.cache, i)
}

func (s inMemorySymbolsResolver) WriteStats(stats *symdb.Stats) {
	s.reader.stacktraces.WriteStats(s.partition, stats)
	stats.LocationsTotal = int(s.reader.locations.file.NumRows())
	stats.MappingsTotal = int(s.reader.mappings.file.NumRows())
	stats.FunctionsTotal = int(s.reader.functions.file.NumRows())
	stats.StringsTotal = int(s.reader.strings.file.NumRows())
}
