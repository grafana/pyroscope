package readpath

import (
	"context"
	"errors"
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

func TestRouter_FunctionsStorageVersions(t *testing.T) {
	t.Parallel()
	split := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	for _, tc := range []struct {
		name           string
		start, end     time.Time
		legacyFallback bool
		wantError      bool
	}{
		{name: "v1", start: split.Add(-time.Hour), end: split.Add(-time.Minute), wantError: true},
		{name: "v2", start: split.Add(time.Minute), end: split.Add(time.Hour)},
		{name: "mixed", start: split.Add(-time.Hour), end: split.Add(time.Hour), wantError: true},
		{name: "v1 ignores unknown format", start: split.Add(-time.Hour), end: split.Add(-time.Minute), legacyFallback: true, wantError: true},
		{name: "mixed with v1 ignoring unknown format", start: split.Add(-time.Hour), end: split.Add(time.Hour), legacyFallback: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := &querierv1.FunctionTable{Total: 19, Functions: []*querierv1.FunctionRow{{Name: "function", Self: 10, Total: 10}}}
			oldClient := functionsClient{query: func(_ context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
				require.Less(t, req.Msg.End, split.UnixMilli())
				if tc.legacyFallback {
					return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Flamegraph: &querierv1.FlameGraph{Total: 19}}), nil
				}
				return nil, connect.NewError(connect.CodeUnimplemented, errors.New("functions format is only supported with the v2 query backend"))
			}}
			newClient := functionsClient{query: func(ctx context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
				id, err := tenant.ExtractTenantIDFromContext(ctx)
				require.NoError(t, err)
				require.Equal(t, "tenant-a", id)
				require.Equal(t, querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, req.Msg.Format)
				require.Equal(t, int64(1), req.Msg.MaxFunctions)
				require.GreaterOrEqual(t, req.Msg.Start, split.UnixMilli())
				return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Functions: want}), nil
			}}
			router := NewRouter(log.NewNopLogger(), functionsOverrides{Config{
				EnableQueryBackend: true, EnableQueryBackendFrom: QueryBackendFrom{Time: split},
			}}, nil, oldClient, newClient)
			resp, err := router.SelectMergeStacktraces(tenant.InjectTenantID(context.Background(), "tenant-a"), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
				Start: tc.start.UnixMilli(), End: tc.end.UnixMilli(), Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, MaxFunctions: 1,
			}))
			if tc.wantError {
				require.Nil(t, resp)
				require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, want, resp.Msg.Functions)
		})
	}
}
