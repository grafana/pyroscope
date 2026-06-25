package queryfrontend

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// TODO(kolesnikovae): Decide whether we want to implement those.

func (q *QueryFrontend) AnalyzeQuery(
	context.Context,
	*connect.Request[querierv1.AnalyzeQueryRequest],
) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	return connect.NewResponse(&querierv1.AnalyzeQueryResponse{}), nil
}

// AsyncQuery is served by the async decorator at the read-path layer; the
// underlying QueryFrontend never sees it.
func (q *QueryFrontend) AsyncQuery(
	context.Context,
	*connect.Request[querierv1.AsyncQueryRequest],
) (*connect.Response[querierv1.AsyncQueryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("async queries are not handled at this layer"))
}
