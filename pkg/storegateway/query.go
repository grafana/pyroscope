package storegateway

import (
	"context"
	"io"

	"github.com/bufbuild/connect-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	"github.com/grafana/phlare/pkg/phlaredb"
	"github.com/grafana/phlare/pkg/tenant"
)

func (s *StoreGateway) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	found, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		return bs.MergeProfilesStacktraces(ctx, stream)
	})
	if err != nil || found {
		return err
	}
	return terminateStream(stream)
}

func (s *StoreGateway) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	found, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		return bs.MergeProfilesLabels(ctx, stream)
	})
	if err != nil || found {
		return err
	}
	return terminateStream(stream)
}

func (s *StoreGateway) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	found, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		return bs.MergeProfilesPprof(ctx, stream)
	})
	if err != nil || found {
		return err
	}
	return terminateStream(stream)
}

func terminateStream[Req, Resp any](stream *connect.BidiStream[Req, Resp]) (err error) {
	if _, err = stream.Receive(); err != nil {
		if errors.Is(err, io.EOF) {
			return connect.NewError(connect.CodeCanceled, errors.New("client closed stream"))
		}
		return err
	}
	if err = stream.Send(new(Resp)); err != nil {
		return err
	}
	return stream.Send(new(Resp))
}

// forBucketStore executes the given function for the bucketstore with the given tenant ID in the context.
func (s *StoreGateway) forBucketStore(ctx context.Context, f func(*BucketStore) error) (bool, error) {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return true, connect.NewError(connect.CodeInvalidArgument, err)
	}
	store := s.stores.getStore(tenantID)
	if store != nil {
		return true, f(store)
	}
	return false, nil
}

func (s *BucketStore) openBlocksForReading(ctx context.Context, minT, maxT model.Time) (phlaredb.Queriers, error) {
	blks := s.blockSet.getFor(minT, maxT)
	querier := make(phlaredb.Queriers, 0, len(blks))
	for _, b := range blks {
		querier = append(querier, b)
	}
	if err := querier.Open(ctx); err != nil {
		return nil, err
	}
	return querier, nil
}

func (store *BucketStore) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	return phlaredb.MergeProfilesStacktraces(ctx, stream, store.openBlocksForReading)
}

func (store *BucketStore) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	return phlaredb.MergeProfilesLabels(ctx, stream, store.openBlocksForReading)
}

func (store *BucketStore) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	return phlaredb.MergeProfilesPprof(ctx, stream, store.openBlocksForReading)
}
