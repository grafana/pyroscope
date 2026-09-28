package async

import (
	"context"
	"io"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

type emptyObjectEOFBucket struct {
	objstore.Bucket
}

func (b *emptyObjectEOFBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	attrs, err := b.Attributes(ctx, name)
	if err != nil {
		return nil, err
	}
	if attrs.Size == 0 {
		return nil, io.EOF
	}
	return b.Bucket.Get(ctx, name)
}

type getErrBucket struct {
	objstore.Bucket
	target string
	err    error
}

func (b *getErrBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	if name == b.target {
		return nil, b.err
	}
	return b.Bucket.Get(ctx, name)
}

func TestHandlerPollReturnsEmptyResult(t *testing.T) {
	ctx := t.Context()
	bucket := &emptyObjectEOFBucket{Bucket: objstore.NewInMemBucket()}
	store := NewStore(log.NewNopLogger(), bucket, nil)
	const (
		tenantID  = "tenant-a"
		requestID = "550e8400-e29b-41d4-a716-446655440002"
	)
	req := &querierv1.SelectMergeStacktracesRequest{}
	require.NoError(t, store.create(ctx, tenantID, requestID, req))
	require.NoError(t, store.complete(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesResponse{}))

	handler := &Handler{logger: log.NewNopLogger(), coordinator: &Coordinator{store: store}}
	resp, err := handler.poll(ctx, tenantID, requestID)

	require.NoError(t, err)
	require.Equal(t, querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS, resp.Msg.GetAsync().GetStatus())
}

func TestHandlerPollEOFFromNonEmptyResult(t *testing.T) {
	ctx := t.Context()
	bucket := &getErrBucket{Bucket: objstore.NewInMemBucket(), err: io.EOF}
	store := NewStore(log.NewNopLogger(), bucket, nil)
	const (
		tenantID  = "tenant-a"
		requestID = "550e8400-e29b-41d4-a716-446655440003"
	)
	req := &querierv1.SelectMergeStacktracesRequest{}
	require.NoError(t, store.create(ctx, tenantID, requestID, req))
	require.NoError(t, store.complete(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesResponse{Dot: "digraph {}"}))
	bucket.target = store.buildPath(tenantID, requestID, resultFilename)

	handler := &Handler{logger: log.NewNopLogger(), coordinator: &Coordinator{store: store}}
	_, err := handler.poll(ctx, tenantID, requestID)

	require.Error(t, err)
	require.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestHandlerPollCopiesPprofResponse(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	const (
		tenantID  = "tenant-a"
		requestID = "550e8400-e29b-41d4-a716-446655440000"
	)
	require.NoError(t, store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{}))
	want := &profilev1.Profile{Sample: []*profilev1.Sample{{Value: []int64{1}}}}
	require.NoError(t, store.complete(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesResponse{
		Pprof: &querierv1.PprofProfile{Profile: want},
	}))

	handler := &Handler{logger: log.NewNopLogger(), coordinator: &Coordinator{store: store}}
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
	require.NoError(t, store.create(ctx, tenantA, requestID, &querierv1.SelectMergeStacktracesRequest{}))

	handler := &Handler{logger: log.NewNopLogger(), coordinator: &Coordinator{store: store}}
	_, err := handler.poll(ctx, tenantB, requestID)

	require.Error(t, err)
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
