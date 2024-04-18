package frontend

import (
	"context"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func (f *Frontend) AnalyzeQuery(ctx context.Context,
	c *connect.Request[querierv1.AnalyzeQueryRequest]) (
	*connect.Response[querierv1.AnalyzeQueryResponse], error,
) {
	return nil, nil
}
