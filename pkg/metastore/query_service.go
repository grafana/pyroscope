package metastore

import (
	"context"
	"errors"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/metastore/index"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode"
)

type IndexQuerier interface {
	QueryMetadata(*bbolt.Tx, context.Context, index.MetadataQuery) ([]*metastorev1.BlockMeta, error)
	QueryMetadataLabels(*bbolt.Tx, context.Context, index.MetadataQuery) ([]*typesv1.Labels, error)
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryService.QueryMetadata")
	defer func() {
		if err != nil {
			ext.LogError(span, err)
		}
		span.Finish()
	}()
	span.SetTag("tenant_id", req.GetTenantId())
	span.SetTag("start_time", req.GetStartTime())
	span.SetTag("end_time", req.GetEndTime())
	span.SetTag("labels", len(req.GetLabels()))
	if q := req.GetQuery(); q != "" {
		span.LogFields(otlog.String("query", q))
	}

	read := func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		resp, err = svc.queryMetadata(ctx, tx, req)
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	return resp, err
}

func (svc *QueryService) queryMetadata(
	ctx context.Context,
	tx *bbolt.Tx,
	req *metastorev1.QueryMetadataRequest,
) (resp *metastorev1.QueryMetadataResponse, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryService.indexQueryMetadata")
	defer func() {
		if err != nil {
			ext.LogError(span, err)
		}
		span.Finish()
	}()

	metas, err := svc.index.QueryMetadata(tx, ctx, index.MetadataQuery{
		Tenant:    req.TenantId,
		StartTime: time.UnixMilli(req.StartTime),
		EndTime:   time.UnixMilli(req.EndTime),
		Expr:      req.Query,
		Labels:    req.Labels,
	})
	if err == nil {
		span.SetTag("result_count", len(metas))
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryService.QueryMetadataLabels")
	defer func() {
		if err != nil {
			ext.LogError(span, err)
		}
		span.Finish()
	}()

	span.SetTag("tenant_id", req.GetTenantId())
	span.SetTag("start_time", req.GetStartTime())
	span.SetTag("end_time", req.GetEndTime())
	span.SetTag("labels", len(req.GetLabels()))
	if q := req.GetQuery(); q != "" {
		span.LogFields(otlog.String("query", q))
	}

	read := func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		resp, err = svc.queryMetadataLabels(ctx, tx, req)
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	return resp, err
}

func (svc *QueryService) queryMetadataLabels(
	ctx context.Context,
	tx *bbolt.Tx,
	req *metastorev1.QueryMetadataLabelsRequest,
) (resp *metastorev1.QueryMetadataLabelsResponse, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryService.indexQueryMetadataLabels")
	defer func() {
		if err != nil {
			ext.LogError(span, err)
		}
		span.Finish()
	}()

	labels, err := svc.index.QueryMetadataLabels(tx, ctx, index.MetadataQuery{
		Tenant:    req.TenantId,
		StartTime: time.UnixMilli(req.StartTime),
		EndTime:   time.UnixMilli(req.EndTime),
		Expr:      req.Query,
		Labels:    req.Labels,
	})
	if err == nil {
		span.SetTag("result_count", len(labels))
		return &metastorev1.QueryMetadataLabelsResponse{Labels: labels}, nil
	}
	var invalid *index.InvalidQueryError
	if errors.As(err, &invalid) {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	level.Error(svc.logger).Log("msg", "failed to query metadata labels", "err", err)
	return nil, status.Error(codes.Internal, err.Error())
}
