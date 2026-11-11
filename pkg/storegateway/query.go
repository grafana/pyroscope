package storegateway

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"

	"connectrpc.com/connect"
	"github.com/prometheus/common/model"
	"golang.org/x/sync/errgroup"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
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

func (s *StoreGateway) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	var res *ingestv1.ProfileTypesResponse
	_, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		return bs.withBlocks(func(getBlocks phlaredb.BlockGetter) error {
			result, err := phlaredb.ProfileTypes(ctx, req, getBlocks)
			if err != nil {
				return err
			}
			res = result.Msg
			return nil
		})
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(res), nil
}

func (s *StoreGateway) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	var res *typesv1.LabelValuesResponse
	_, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		return bs.withBlocks(func(getBlocks phlaredb.BlockGetter) error {
			var err error
			res, err = phlaredb.LabelValues(ctx, req, getBlocks)
			return err
		})
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(res), nil
}

func (s *StoreGateway) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	var res *typesv1.LabelNamesResponse
	_, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		return bs.withBlocks(func(getBlocks phlaredb.BlockGetter) error {
			var err error
			res, err = phlaredb.LabelNames(ctx, req, getBlocks)
			return err
		})
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(res), nil
}

func (s *StoreGateway) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	var res *ingestv1.SeriesResponse
	_, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		return bs.withBlocks(func(getBlocks phlaredb.BlockGetter) error {
			var err error
			res, err = phlaredb.Series(ctx, req.Msg, getBlocks)
			return err
		})
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(res), nil
}

func (s *StoreGateway) MergeSpanProfile(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeSpanProfileRequest, ingestv1.MergeSpanProfileResponse]) error {
	found, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		return bs.MergeSpanProfile(ctx, stream)
	})
	if err != nil || found {
		return err
	}
	return terminateStream(stream)
}

func (s *StoreGateway) BlockMetadata(ctx context.Context, req *connect.Request[ingestv1.BlockMetadataRequest]) (*connect.Response[ingestv1.BlockMetadataResponse], error) {
	var res *ingestv1.BlockMetadataResponse
	found, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		var err error
		res, err = bs.BlockMetadata(ctx, req.Msg)
		return err
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if !found {
		res = &ingestv1.BlockMetadataResponse{}
	}

	return connect.NewResponse(res), nil
}

func (s *StoreGateway) GetBlockStats(ctx context.Context, req *connect.Request[ingestv1.GetBlockStatsRequest]) (*connect.Response[ingestv1.GetBlockStatsResponse], error) {
	res := &ingestv1.GetBlockStatsResponse{}
	_, err := s.forBucketStore(ctx, func(bs *BucketStore) error {
		bs.blocksMx.RLock()
		defer bs.blocksMx.RUnlock()

		for ulid, block := range bs.blocks {
			if slices.Contains(req.Msg.Ulids, ulid.String()) {
				res.BlockStats = append(res.BlockStats, block.meta.GetStats().ConvertToBlockStats())
			}
		}
		return nil
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(res), nil
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

// withBlocks runs f with a block getter whose blocks stay open until f returns.
func (s *BucketStore) withBlocks(f func(phlaredb.BlockGetter) error) error {
	var refs blockRefs
	defer refs.releaseAll()
	return f(func(ctx context.Context, minT, maxT model.Time, hints *ingestv1.Hints) (phlaredb.Queriers, error) {
		return s.openBlocksForReading(ctx, minT, maxT, hints, &refs)
	})
}

func (s *BucketStore) openBlocksForReading(ctx context.Context, minT, maxT model.Time, hints *ingestv1.Hints, refs *blockRefs) (phlaredb.Queriers, error) {
	skipBlock := phlaredb.HintsToBlockSkipper(hints)
	blks := s.blockSet.getFor(minT, maxT)
	querier := make(phlaredb.Queriers, 0, len(blks))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(128)
	for _, b := range blks {
		if skipBlock(b.BlockID()) {
			continue
		}
		querier = append(querier, b)
		g.Go(func() error {
			release, err := b.Retain(ctx)
			if err != nil {
				return err
			}
			refs.add(release)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return querier, nil
}

// blockRefs holds the block references taken by one request.
type blockRefs struct {
	mu      sync.Mutex
	release []func()
}

func (r *blockRefs) add(release func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.release = append(r.release, release)
}

func (r *blockRefs) releaseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, release := range r.release {
		release()
	}
	r.release = nil
}

func (store *BucketStore) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	return store.withBlocks(func(getBlocks phlaredb.BlockGetter) error {
		return phlaredb.MergeProfilesStacktraces(ctx, stream, getBlocks)
	})
}

func (store *BucketStore) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	return store.withBlocks(func(getBlocks phlaredb.BlockGetter) error {
		return phlaredb.MergeProfilesLabels(ctx, stream, getBlocks)
	})
}

func (store *BucketStore) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	return store.withBlocks(func(getBlocks phlaredb.BlockGetter) error {
		return phlaredb.MergeProfilesPprof(ctx, stream, getBlocks)
	})
}

func (store *BucketStore) MergeSpanProfile(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeSpanProfileRequest, ingestv1.MergeSpanProfileResponse]) error {
	return store.withBlocks(func(getBlocks phlaredb.BlockGetter) error {
		return phlaredb.MergeSpanProfile(ctx, stream, getBlocks)
	})
}

func (s *BucketStore) BlockMetadata(ctx context.Context, req *ingestv1.BlockMetadataRequest) (*ingestv1.BlockMetadataResponse, error) {
	set := s.blockSet.getFor(model.Time(req.Start), model.Time(req.End))
	result := &ingestv1.BlockMetadataResponse{
		Blocks: make([]*typesv1.BlockInfo, len(set)),
	}
	for idx, b := range set {
		var info typesv1.BlockInfo
		b.meta.WriteBlockInfo(&info)
		result.Blocks[idx] = &info
	}
	return result, nil
}
