package symbolizer

import (
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/lidia"
)

// LidiaTableCacheEntry represents a cached Lidia table with its binary layout information
type LidiaTableCacheEntry struct {
	Data []byte // Processed Lidia table data
}

// location represents a memory address to be symbolized
type location struct {
	address uint64
	lines   []lidia.SourceInfoFrame
}

// request represents a symbolization request for multiple addresses
type request struct {
	buildID    string
	binaryName string
	locations  []*location
}

// symbolizedLocation represents a location that has been symbolized
type symbolizedLocation struct {
	loc     *googlev1.Location
	symLoc  *location
	mapping *googlev1.Mapping
}

type funcKey struct {
	nameIdx, filenameIdx int64
}
