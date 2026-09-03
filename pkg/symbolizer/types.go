package symbolizer

import (
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/lidia"
)

// LidiaTableCacheEntry represents a cached Lidia table with its binary layout information
type LidiaTableCacheEntry struct {
	Data []byte // Processed Lidia table data
}

// symbolizedLocation represents a location that has been symbolized
type symbolizedLocation struct {
	loc     *googlev1.Location
	lines   []lidia.SourceInfoFrame
	mapping *googlev1.Mapping
}

type funcKey struct {
	nameIdx, filenameIdx int64
}
