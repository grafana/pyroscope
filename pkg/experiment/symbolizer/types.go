package symbolizer

import (
	"github.com/grafana/pyroscope/lidia"

	pprof "github.com/google/pprof/profile"
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

type locToSymbolize struct {
	idx int
	loc *googlev1.Location
}

// LidiaTableCacheEntry represents a cached Lidia table with its binary layout information
type LidiaTableCacheEntry struct {
	Data []byte        // Processed Lidia table data
	EI   *BinaryLayout // Binary layout information for address mapping
}

// Location represents a memory address to be symbolized
type Location struct {
	ID      string
	Address uint64
	Lines   []lidia.SourceInfoFrame
	Mapping *pprof.Mapping
}

// Request represents a symbolization request for multiple addresses
type Request struct {
	BuildID    string
	BinaryName string
	Locations  []*Location
}

// Mapping describes how a binary section is mapped in memory
type Mapping struct {
	Start  uint64
	End    uint64
	Limit  uint64
	Offset uint64
}
