package symdb

import (
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type SymbolsWriter interface {
	AppendStacktraces([]uint32, []*schemav1.Stacktrace)
	AppendLocations([]uint32, []*schemav1.InMemoryLocation)
	AppendMappings([]uint32, []*schemav1.InMemoryMapping)
	AppendFunctions([]uint32, []*schemav1.InMemoryFunction)
	AppendStrings([]uint32, []string)
}

type SymbolsReader interface {
	StacktraceResolver

	Locations() ([]*schemav1.InMemoryLocation, error)
	Mappings() ([]*schemav1.InMemoryMapping, error)
	Functions() ([]*schemav1.InMemoryFunction, error)
	Strings() ([]string, error)

	WriteStats(*Stats)
	Release()
}
