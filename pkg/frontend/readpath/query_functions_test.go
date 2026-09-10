package readpath

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
)

type functionsClient struct {
	querierv1connect.QuerierServiceClient
	query func(context.Context, *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error)
}

func (c functionsClient) SelectMergeStacktraces(ctx context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	return c.query(ctx, req)
}

type functionsOverrides struct{ cfg Config }

func (o functionsOverrides) ReadPathOverrides(string) Config { return o.cfg }

func TestRouter_FunctionsAcrossMigration(t *testing.T) {
	t.Parallel()
	split := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	client := func(name string, start, end int64) functionsClient {
		return functionsClient{query: func(ctx context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
			id, err := tenant.ExtractTenantIDFromContext(ctx)
			require.NoError(t, err)
			require.Equal(t, "tenant-a", id)
			require.Equal(t, querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, req.Msg.Format)
			require.Equal(t, int64(-1), req.Msg.MaxFunctions)
			require.Equal(t, start, req.Msg.Start)
			require.Equal(t, end, req.Msg.End)
			return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Functions: &querierv1.FunctionTable{
				Total: 19, Functions: []*querierv1.FunctionRow{
					{Name: name, Self: 10, Total: 10}, {Name: "global", Self: 9, Total: 9},
				},
			}}), nil
		}}
	}
	start, end := split.Add(-time.Hour).UnixMilli(), split.Add(time.Hour).UnixMilli()
	router := NewRouter(log.NewNopLogger(), functionsOverrides{Config{
		EnableQueryBackend: true, EnableQueryBackendFrom: QueryBackendFrom{Time: split},
	}}, nil, client("old", start, split.UnixMilli()-1), client("new", split.UnixMilli(), end))
	resp, err := router.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), "tenant-a"), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		Start: start, End: end, Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, MaxFunctions: 1,
	}))
	require.NoError(t, err)
	require.Equal(t, int64(38), resp.Msg.Functions.Total)
	require.Equal(t, []*querierv1.FunctionRow{{Name: "global", Self: 18, Total: 18}}, resp.Msg.Functions.Functions)
}
