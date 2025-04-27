package metastore

import (
	"context"
	"errors"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
)

type IndexQuerier interface {
	QueryMetadata(*bbolt.Tx, index.MetadataQuery) ([]*metastorev1.BlockMeta, error)
	QueryMetadataLabels(*bbolt.Tx, index.MetadataQuery) ([]*typesv1.Labels, error)
}

type QueryService struct {
	metastorev1.MetadataQueryServiceServer

	logger log.Logger
	state  State
	index  IndexQuerier
}

func NewQueryService(
	logger log.Logger,
	state State,
	index IndexQuerier,
) *QueryService {
	return &QueryService{
		logger: logger,
		state:  state,
		index:  index,
	}
}

func (svc *QueryService) QueryMetadata(
	ctx context.Context,
	req *metastorev1.QueryMetadataRequest,
) (resp *metastorev1.QueryMetadataResponse, err error) {
	read := func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		resp, err = svc.queryMetadata(ctx, tx, req)
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	return resp, err
}

func (svc *QueryService) queryMetadata(
	_ context.Context,
	tx *bbolt.Tx,
	req *metastorev1.QueryMetadataRequest,
) (*metastorev1.QueryMetadataResponse, error) {
	metas, err := svc.index.QueryMetadata(tx, index.MetadataQuery{
		Tenant:    req.TenantId,
		StartTime: time.UnixMilli(req.StartTime),
		EndTime:   time.UnixMilli(req.EndTime),
		Expr:      req.Query,
		Labels:    req.Labels,
	})
	if err == nil {
		return &metastorev1.QueryMetadataResponse{Blocks: metas}, nil
	}
	var invalid *index.InvalidQueryError
	if errors.As(err, &invalid) {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	level.Error(svc.logger).Log("msg", "failed to query metadata", "err", err)
	return nil, status.Error(codes.Internal, err.Error())
}

func (svc *QueryService) QueryMetadataLabels(
	ctx context.Context,
	req *metastorev1.QueryMetadataLabelsRequest,
) (resp *metastorev1.QueryMetadataLabelsResponse, err error) {
	read := func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		resp, err = svc.queryMetadataLabels(ctx, tx, req)
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	return resp, err
}

func (svc *QueryService) queryMetadataLabels(
	_ context.Context,
	tx *bbolt.Tx,
	req *metastorev1.QueryMetadataLabelsRequest,
) (*metastorev1.QueryMetadataLabelsResponse, error) {
	labels, err := svc.index.QueryMetadataLabels(tx, index.MetadataQuery{
		Tenant:    req.TenantId,
		StartTime: time.UnixMilli(req.StartTime),
		EndTime:   time.UnixMilli(req.EndTime),
		Expr:      req.Query,
		Labels:    req.Labels,
	})
	if err == nil {
		return &metastorev1.QueryMetadataLabelsResponse{Labels: labels}, nil
	}
	var invalid *index.InvalidQueryError
	if errors.As(err, &invalid) {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	level.Error(svc.logger).Log("msg", "failed to query metadata labels", "err", err)
	return nil, status.Error(codes.Internal, err.Error())
}
