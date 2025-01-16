package symbolizer

import (
	"context"

	pprof "github.com/google/pprof/profile"
)

// SymbolLocation represents a resolved source code location with function information
type SymbolLocation struct {
	Function *pprof.Function
	Line     int64
}

// Location represents a memory address to be symbolized
type Location struct {
	ID      string
	Address uint64
	Lines   []SymbolLocation
	Mapping *pprof.Mapping
}

// Request represents a symbolization request for multiple addresses
type Request struct {
	BuildID  string
	Mappings []RequestMapping
}

type RequestMapping struct {
	Locations []*Location
}

// Mapping describes how a binary section is mapped in memory
type Mapping struct {
	Start  uint64
	End    uint64
	Limit  uint64
	Offset uint64
}

// SymbolResolver converts memory addresses to source code locations
type SymbolResolver interface {
	ResolveAddress(ctx context.Context, addr uint64) ([]SymbolLocation, error)
	//Close() error
}
