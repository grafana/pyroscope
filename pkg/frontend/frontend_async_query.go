package frontend

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// AsyncQuery is not supported on the V1 frontend; the experimental async
// query path lives in the V2 read-path.
func (f *Frontend) AsyncQuery(
	context.Context,
	*connect.Request[querierv1.AsyncQueryRequest],
) (*connect.Response[querierv1.AsyncQueryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("async queries are not supported on the V1 frontend"))
}
