package phlaredb

import schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"

// TODO(kolesnikovae): Refactor to symdb.

type SymbolsWriter interface {
	SymbolsAppender(partition uint64) (SymbolsAppender, error)
}

type SymbolsAppender interface {
	AppendStacktrace([]int32) uint32
	AppendLocation(*schemav1.InMemoryLocation) uint32
	AppendMapping(*schemav1.InMemoryMapping) uint32
	AppendFunction(*schemav1.InMemoryFunction) uint32
	AppendString(string) uint32

	Flush() error
}
