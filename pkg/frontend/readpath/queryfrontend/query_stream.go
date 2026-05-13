package queryfrontend

import (
	"time"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
)

// streamFrontend wraps QueryFrontend to implement querierv1connect.QuerierStreamServiceHandler.
// A separate type is required because QuerierService and QuerierStreamService share
// method names (SelectMergeStacktracesStream, SelectSeriesStream) but with different
// stream element types, which is not expressible via method overloading in Go.
type streamFrontend struct {
	*QueryFrontend
}

// StreamHandler returns a QuerierStreamServiceHandler view of the QueryFrontend.
func (q *QueryFrontend) StreamHandler() querierv1connect.QuerierStreamServiceHandler {
	return &streamFrontend{q}
}

// streamRecv is a single result from a backend stream Recv call.
type streamRecv struct {
	ev  *queryv1.InvokeStreamEvent
	err error
}

// recvLoop reads from stream.Recv in a goroutine and sends results to ch.
// It exits when err != nil or ctx.Done() fires. Callers must close ctx to
// guarantee the goroutine terminates.
func recvLoop(recv func() (*queryv1.InvokeStreamEvent, error), ch chan<- streamRecv, done <-chan struct{}) {
	for {
		ev, err := recv()
		select {
		case ch <- streamRecv{ev, err}:
		case <-done:
			return
		}
		if err != nil {
			return
		}
	}
}

// computeETA returns the estimated completion time as Unix milliseconds.
// Returns 0 when the estimate is not yet available (bytesDone == 0 or
// bytesTotal == 0).
func computeETA(start time.Time, bytesDone, bytesTotal uint64) int64 {
	if bytesTotal == 0 || bytesDone == 0 {
		return 0
	}
	elapsed := time.Since(start)
	totalDur := time.Duration(int64(elapsed) * int64(bytesTotal) / int64(bytesDone))
	return start.Add(totalDur).UnixMilli()
}
