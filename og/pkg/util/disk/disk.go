package disk

import (
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type UsageStats struct {
	Total     bytesize.ByteSize
	Available bytesize.ByteSize
}
