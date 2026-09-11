package readpath

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockquerierv1connect"
)

func TestRouter_DiffFunctions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                                                      string
		leftStart, rightStart                                     int64
		disabled, auto, federated, secondTenantV1, missingPayload bool
		resolveErr                                                error
		wantError                                                 bool
	}{
		{name: "v2", leftStart: 21000, rightStart: 25000},
		{name: "split boundary", leftStart: 20000, rightStart: 20000},
		{name: "v1", leftStart: 10000, rightStart: 10000, wantError: true},
		{name: "left spans split", leftStart: 19000, rightStart: 25000, wantError: true},
		{name: "right spans split", leftStart: 25000, rightStart: 19000, wantError: true},
		{name: "disabled", leftStart: 25000, rightStart: 25000, disabled: true, wantError: true},
		{name: "automatic split", leftStart: 25000, rightStart: 25000, auto: true},
		{name: "resolution failure", leftStart: 25000, rightStart: 25000, auto: true, resolveErr: context.DeadlineExceeded, wantError: true},
		{name: "no v2 data", leftStart: 25000, rightStart: 25000, auto: true, resolveErr: ErrNoV2Data, wantError: true},
		{name: "federated v2", leftStart: 25000, rightStart: 25000, federated: true},
		{name: "second tenant requires v1", leftStart: 25000, rightStart: 25000, federated: true, secondTenantV1: true, wantError: true},
		{name: "older backend ignores format", leftStart: 25000, rightStart: 25000, missingPayload: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			overrides := new(mockOverrides)
			cfg := Config{EnableQueryBackend: !tc.disabled, EnableQueryBackendFrom: QueryBackendFrom{Time: time.Unix(20, 0)}}
			if tc.auto {
				cfg.EnableQueryBackendFrom = QueryBackendFrom{Auto: true}
			}
			overrides.On("ReadPathOverrides", "tenant-a").Return(cfg).Once()
			orgID := "tenant-a"
			if tc.federated {
				orgID += "|tenant-b"
				second := cfg
				if tc.secondTenantV1 {
					second.EnableQueryBackendFrom.Time = time.Unix(26, 0)
				}
				overrides.On("ReadPathOverrides", "tenant-b").Return(second).Once()
			}
			resolver := new(mockSplitTimeResolver)
			if tc.auto {
				resolver.On("OldestProfileTime", mock.Anything, "tenant-a").Return(time.Unix(20, 0), tc.resolveErr).Once()
			}
			oldFrontend := mockquerierv1connect.NewMockQuerierServiceClient(t)
			newFrontend := mockquerierv1connect.NewMockQuerierServiceClient(t)
			req := connect.NewRequest(&querierv1.DiffRequest{
				Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, MaxNodes: new(int64(1)),
				Left:  &querierv1.SelectMergeStacktracesRequest{Start: tc.leftStart, End: 30000, MaxNodes: new(int64(2))},
				Right: &querierv1.SelectMergeStacktracesRequest{Start: tc.rightStart, End: 30000, MaxNodes: new(int64(3))},
			})
			want := connect.NewResponse(&querierv1.DiffResponse{Functions: &querierv1.FunctionTableDiff{LeftTotal: 10, RightTotal: 20}})
			if tc.missingPayload {
				want = connect.NewResponse(&querierv1.DiffResponse{Flamegraph: &querierv1.FlameGraphDiff{}})
			}
			ctx := tenant.InjectTenantID(context.Background(), orgID)
			if !tc.wantError || tc.missingPayload {
				newFrontend.On("Diff", ctx, req).Return(want, nil).Once()
			}
			router := NewRouter(log.NewNopLogger(), overrides, resolver, oldFrontend, newFrontend)
			resp, err := router.Diff(ctx, req)
			if tc.wantError {
				require.Nil(t, resp)
				require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
			} else {
				require.NoError(t, err)
				require.Same(t, want, resp)
			}
			overrides.AssertExpectations(t)
			resolver.AssertExpectations(t)
		})
	}
}

func TestRouter_DiffFunctions_InvalidRequest(t *testing.T) {
	t.Parallel()
	router := NewRouter(log.NewNopLogger(), nil, nil, nil, nil)
	req := connect.NewRequest(&querierv1.DiffRequest{Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS})
	for _, ctx := range []context.Context{context.Background(), tenant.InjectTenantID(context.Background(), "tenant-a")} {
		resp, err := router.Diff(ctx, req)
		require.Nil(t, resp)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	}
}
