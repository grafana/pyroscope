package symbolizer

import (
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/lidia"

	pprof "github.com/google/pprof/profile"
)

type locToSymbolize struct {
	idx int
	loc *googlev1.Location
}

// LidiaTableCacheEntry represents a cached Lidia table with its binary layout information
type LidiaTableCacheEntry struct {
	Data []byte // Processed Lidia table data
}

// location represents a memory address to be symbolized
type location struct {
	address uint64
	lines   []lidia.SourceInfoFrame
	mapping *pprof.Mapping
}

// request represents a symbolization request for multiple addresses
type request struct {
	buildID    string
	binaryName string
	locations  []*location
}
