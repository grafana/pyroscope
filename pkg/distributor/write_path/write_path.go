package writepath

import (
	"errors"
	"fmt"
)

// WritePath controls the write path.
type WritePath string

const (
	// IngesterPath specifies old write path the requests are sent to ingester.
	IngesterPath WritePath = "ingester"
	// SegmentWriterPath specifies the new write path: distributor sends
	// the request to segment writers before profile split, using the new
	// distribution algorithm and the segment-writer ring.
	SegmentWriterPath = "segment-writer"
	// CombinedPath specifies that each request should be sent to both write
	// paths. For each request we decide on how a failure is handled:
	//  * If the request is sent to ingester (regardless of anything),
	//    the response is returned to the client immediately after the old
	//    write path returns. Failure of the new write path should be logged
	//    and counted in metrics but NOT returned to the client.
	//  * If the request is sent to segment-writer exclusively: the response
	//    returns to the client only when the new write path returns.
	//    Failure of the new write is returned to the client.
	//    Failure of the old write path is NOT returned to the client.
	CombinedPath = "combined"
)

var ErrInvalidWritePath = errors.New("invalid write path")

var paths = []WritePath{
	IngesterPath,
	SegmentWriterPath,
	CombinedPath,
}

func (m WritePath) Validate() error {
	for _, name := range paths {
		if m == name {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrInvalidWritePath, m)
}

type Config interface {
	WritePath() WritePath
	IngesterWeight() float64
	SegmentWriterWeight() float64
}
