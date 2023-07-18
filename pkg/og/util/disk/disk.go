package disk

import (
	"github.com/grafana/pyroscope/pkg/og/util/bytesize"
)

type UsageStats struct {
	Total     bytesize.ByteSize
	Available bytesize.ByteSize
}
