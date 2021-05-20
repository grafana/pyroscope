package disk

import (
	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

func FreeSpace(_ string) (bytesize.ByteSize, error) {
	return 10 * bytesize.GB, nil
}
