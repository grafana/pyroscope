package main

import (
	"context"
	"fmt"
	"math"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/objstore/providers/gcs"
	"github.com/grafana/pyroscope/pkg/phlaredb"
)

type blocksQueryParams struct {
	BucketName      string
	BlockIds        []string
	TenantID        string
	ObjectStoreType string
	Query           string
}

type blocksQueryProfileParams struct {
	*blocksQueryParams
	Output             string
	ProfileType        string
	StacktraceSelector []string
}

type blocksQuerySeriesParams struct {
	*blocksQueryParams
	LabelNames []string
}

func addBlocksQueryParams(queryCmd commander) *blocksQueryParams {
	params := new(blocksQueryParams)
	queryCmd.Flag("bucket-name", "The name of the object storage bucket.").StringVar(&params.BucketName)
	queryCmd.Flag("object-store-type", "The type of the object storage (e.g., gcs).").Default("gcs").StringVar(&params.ObjectStoreType)
	queryCmd.Flag("block", "Block ids to query on (accepts multiples)").StringsVar(&params.BlockIds)
	queryCmd.Flag("tenant-id", "Tenant id of the queried block for remote bucket").StringVar(&params.TenantID)
	queryCmd.Flag("query", "Label selector to query.").Default("{}").StringVar(&params.Query)
	return params
}

func addBlocksQueryProfileParams(queryCmd commander) *blocksQueryProfileParams {
	params := new(blocksQueryProfileParams)
	params.blocksQueryParams = addBlocksQueryParams(queryCmd)
	queryCmd.Flag("output", "How to output the result, examples: console, raw, pprof=./my.pprof").Default("console").StringVar(&params.Output)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("stacktrace-selector", "Only query locations with those symbols. Provide multiple times starting with the root").StringsVar(&params.StacktraceSelector)
	return params
}

func addBlocksQuerySeriesParams(queryCmd commander) *blocksQuerySeriesParams {
	params := new(blocksQuerySeriesParams)
	params.blocksQueryParams = addBlocksQueryParams(queryCmd)
	queryCmd.Flag("label-names", "Filter returned labels to the supplied label names. Without any filter all labels are returned.").StringsVar(&params.LabelNames)
	return params
}

func blocksQueryProfile(ctx context.Context, params *blocksQueryProfileParams) error {
	level.Info(logger).Log("msg", "blocks query profile", "blockIds", fmt.Sprintf("%v", params.BlockIds), "path",
		cfg.blocks.path, "bucketName", params.BucketName, "tenantId", params.TenantID, "query", params.Query, "type", params.ProfileType)

	if len(params.BlockIds) > 1 {
		return errors.New("query profile is limited to a single block")
	}

	profileType, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return err
	}

	var stackTraceSelectors *typesv1.StackTraceSelector = nil
	if len(params.StacktraceSelector) > 0 {
		locations := make([]*typesv1.Location, 0, len(params.StacktraceSelector))
		for _, cs := range params.StacktraceSelector {
			locations = append(locations, &typesv1.Location{
				Name: cs,
			})
		}
		stackTraceSelectors = &typesv1.StackTraceSelector{
			CallSite: locations,
		}
		level.Info(logger).Log("msg", "selecting with stackstrace selector", "call-site", fmt.Sprintf("%#+v", params.StacktraceSelector))
	}

	bucket, err := getBucket(ctx, params.blocksQueryParams)
	if err != nil {
		return err
	}

	meta, err := phlaredb.NewBlockQuerier(ctx, bucket).BlockMeta(ctx, params.BlockIds[0])
	if err != nil {
		return err
	}

	resp, err := phlaredb.NewSingleBlockQuerierFromMeta(ctx, bucket, meta).SelectMergePprof(
		ctx,
		&ingestv1.SelectProfilesRequest{
			LabelSelector: params.Query,
			Type:          profileType,
			Start:         meta.MinTime.Time().UnixMilli(),
			End:           meta.MaxTime.Time().UnixMilli(),
		},
		0,
		stackTraceSelectors,
	)
	if err != nil {
		return errors.Wrap(err, "failed to query")
	}

	return outputMergeProfile(ctx, params.Output, resp)
}

func blocksQuerySeries(ctx context.Context, params *blocksQuerySeriesParams) error {
	level.Info(logger).Log("msg", "blocks query series", "labelNames", fmt.Sprintf("%v", params.LabelNames),
		"blockIds", fmt.Sprintf("%v", params.BlockIds), "path", cfg.blocks.path, "bucketName", params.BucketName, "tenantId", params.TenantID)

	bucket, err := getBucket(ctx, params.blocksQueryParams)
	if err != nil {
		return err
	}

	blockQuerier := phlaredb.NewBlockQuerier(ctx, bucket)

	var from, to int64
	from, to = math.MaxInt64, math.MinInt64
	var targetBlockQueriers phlaredb.Queriers
	for _, blockId := range params.blocksQueryParams.BlockIds {
		meta, err := blockQuerier.BlockMeta(ctx, blockId)
		if err != nil {
			return err
		}
		from = min(from, meta.MinTime.Time().UnixMilli())
		to = max(to, meta.MaxTime.Time().UnixMilli())
		targetBlockQueriers = append(targetBlockQueriers, phlaredb.NewSingleBlockQuerierFromMeta(ctx, bucket, meta))
	}

	response, err := targetBlockQueriers.Series(ctx, connect.NewRequest(
		&ingestv1.SeriesRequest{
			Start:      from,
			End:        to,
			Matchers:   []string{params.Query},
			LabelNames: params.LabelNames,
		},
	))
	if err != nil {
		return err
	}

	return outputSeries(response.Msg.LabelsSet)
}

func getBucket(ctx context.Context, params *blocksQueryParams) (objstore.Bucket, error) {
	if params.BucketName != "" {
		return getRemoteBucket(ctx, params)
	} else {
		return filesystem.NewBucket(cfg.blocks.path)
	}
}

func getRemoteBucket(ctx context.Context, params *blocksQueryParams) (objstore.Bucket, error) {
	if params.TenantID == "" {
		return nil, errors.New("specify tenant id for remote bucket")
	}
	return objstoreclient.NewBucket(ctx, objstoreclient.Config{
		StorageBackendConfig: objstoreclient.StorageBackendConfig{
			Backend: params.ObjectStoreType,
			GCS: gcs.Config{
				BucketName: params.BucketName,
			},
		},
		StoragePrefix: fmt.Sprintf("%s/phlaredb", params.TenantID),
	}, params.BucketName)
}
