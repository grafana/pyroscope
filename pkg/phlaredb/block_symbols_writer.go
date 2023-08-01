package phlaredb

import (
	"context"
	"fmt"
	"path/filepath"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

// TODO(kolesnikovae): Refactor to symdb.

type SymbolsWriter interface {
	SymbolsAppender(partition uint64) (SymbolsAppender, error)
}

type SymbolsAppender interface {
	AppendStacktraces([]uint32, []*schemav1.Stacktrace)
	AppendLocations([]uint32, []*schemav1.InMemoryLocation)
	AppendMappings([]uint32, []*schemav1.InMemoryMapping)
	AppendFunctions([]uint32, []*schemav1.InMemoryFunction)
	AppendStrings([]uint32, []string)
}

type symbolsWriter struct {
	partitions map[uint64]*symbolsAppender

	locations deduplicatingSlice[*schemav1.InMemoryLocation, locationsKey, *locationsHelper, *schemav1.LocationPersister]
	mappings  deduplicatingSlice[*schemav1.InMemoryMapping, mappingsKey, *mappingsHelper, *schemav1.MappingPersister]
	functions deduplicatingSlice[*schemav1.InMemoryFunction, functionsKey, *functionsHelper, *schemav1.FunctionPersister]
	strings   deduplicatingSlice[string, string, *stringsHelper, *schemav1.StringPersister]
	tables    []Table

	symdb *symdb.SymDB
}

func newSymbolsWriter(dst string, cfg *ParquetConfig) (*symbolsWriter, error) {
	w := symbolsWriter{
		partitions: make(map[uint64]*symbolsAppender),
	}
	dir := filepath.Join(dst, symdb.DefaultDirName)
	w.symdb = symdb.NewSymDB(symdb.DefaultConfig().WithDirectory(dir))
	w.tables = []Table{
		&w.locations,
		&w.mappings,
		&w.functions,
		&w.strings,
	}
	for _, t := range w.tables {
		if err := t.Init(dst, cfg, contextHeadMetrics(context.Background())); err != nil {
			return nil, err
		}
	}
	return &w, nil
}

func (w *symbolsWriter) SymbolsAppender(partition uint64) (SymbolsAppender, error) {
	p, ok := w.partitions[partition]
	if !ok {
		appender := w.symdb.SymbolsWriter(partition)
		x := &symbolsAppender{
			stacktraces: appender,
			writer:      w,
		}
		w.partitions[partition] = x
		p = x
	}
	return p, nil
}

func (w *symbolsWriter) Close() error {
	for _, t := range w.tables {
		_, _, err := t.Flush(context.Background())
		if err != nil {
			return fmt.Errorf("flushing table %s: %w", t.Name(), err)
		}
		if err = t.Close(); err != nil {
			return fmt.Errorf("closing table %s: %w", t.Name(), err)
		}
	}
	if err := w.symdb.Flush(); err != nil {
		return fmt.Errorf("flushing symbol database: %w", err)
	}
	return nil
}

type symbolsAppender struct {
	stacktraces symdb.SymbolsWriter
	writer      *symbolsWriter
}

func (s symbolsAppender) AppendStacktraces(dst []uint32, stacktraces []*schemav1.Stacktrace) {
	s.stacktraces.AppendStacktraces(dst, stacktraces)
}

func (s symbolsAppender) AppendLocations(dst []uint32, locations []*schemav1.InMemoryLocation) {
	s.writer.locations.append(dst, locations)
}

func (s symbolsAppender) AppendMappings(dst []uint32, mappings []*schemav1.InMemoryMapping) {
	s.writer.mappings.append(dst, mappings)
}

func (s symbolsAppender) AppendFunctions(dst []uint32, functions []*schemav1.InMemoryFunction) {
	s.writer.functions.append(dst, functions)
}

func (s symbolsAppender) AppendStrings(dst []uint32, strings []string) {
	s.writer.strings.append(dst, strings)
}
