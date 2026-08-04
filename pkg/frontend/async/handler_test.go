package async

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func TestHandlerPollCopiesPprofResponse(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	const (
		tenantID  = "tenant-a"
		requestID = "550e8400-e29b-41d4-a716-446655440000"
	)
	lease, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)
	want := &profilev1.Profile{Sample: []*profilev1.Sample{{Value: []int64{1}}}}
	require.NoError(t, store.complete(ctx, tenantID, requestID, &lease, &querierv1.SelectMergeStacktracesResponse{
		Pprof: &querierv1.PprofProfile{Profile: want},
	}))

	handler := &Handler{coordinator: &Coordinator{store: store}}
	resp, err := handler.poll(ctx, tenantID, requestID)

	require.NoError(t, err)
	require.Equal(t, querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS, resp.Msg.GetAsync().GetStatus())
	require.True(t, proto.Equal(want, resp.Msg.GetPprof().GetProfile()))
}

func TestHandlerPollTenantIsolation(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	const (
		tenantA   = "tenant-a"
		tenantB   = "tenant-b"
		requestID = "550e8400-e29b-41d4-a716-446655440001"
	)
	_, err := store.create(ctx, tenantA, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	handler := &Handler{coordinator: &Coordinator{store: store}}
	_, err = handler.poll(ctx, tenantB, requestID)

	require.Error(t, err)
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
