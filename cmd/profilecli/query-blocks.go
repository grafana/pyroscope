package main

import (
	"context"
	"fmt"
	"math"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/objstore/providers/gcs"
	"github.com/grafana/pyroscope/pkg/phlaredb"
)

type queryBlocksParams struct {
	LocalPath       string
	BucketName      string
	BlockIds        []string
	TenantID        string
	ObjectStoreType string
	Query           string
}

type queryBlocksSeriesParams struct {
	*queryBlocksParams
	LabelNames []string
}

func addQueryBlocksParams(queryCmd commander) *queryBlocksParams {
	params := new(queryBlocksParams)
	queryCmd.Flag("local-path", "Path to blocks directory.").Default("./data/anonymous/local").StringVar(&params.LocalPath)
	queryCmd.Flag("bucket-name", "The name of the object storage bucket.").StringVar(&params.BucketName)
	queryCmd.Flag("object-store-type", "The type of the object storage (e.g., gcs).").Default("gcs").StringVar(&params.ObjectStoreType)
	queryCmd.Flag("block-ids", "List of blocks ids to query on").StringsVar(&params.BlockIds)
	queryCmd.Flag("tenant-id", "Tenant id of the queried block for remote bucket").StringVar(&params.TenantID)
	queryCmd.Flag("query", "Label selector to query.").Default("{}").StringVar(&params.Query)
	return params
}

func addQueryBlocksSeriesParams(queryCmd commander) *queryBlocksSeriesParams {
	params := new(queryBlocksSeriesParams)
	params.queryBlocksParams = addQueryBlocksParams(queryCmd)
	queryCmd.Flag("label-names", "Filter returned labels to the supplied label names. Without any filter all labels are returned.").StringsVar(&params.LabelNames)
	return params
}

func queryBlocksSeries(ctx context.Context, params *queryBlocksSeriesParams) error {
	level.Info(logger).Log("msg", "query-block series", "labelNames", fmt.Sprintf("%v", params.LabelNames),
		"blockIds", fmt.Sprintf("%v", params.BlockIds), "localPath", params.LocalPath, "bucketName", params.BucketName, "tenantId", params.TenantID)

	bucket, err := getBucket(ctx, params)
	if err != nil {
		return err
	}

	blockQuerier := phlaredb.NewBlockQuerier(ctx, bucket)

	var from, to int64
	from, to = math.MaxInt64, math.MinInt64
	var targetBlockQueriers phlaredb.Queriers
	for _, blockId := range params.queryBlocksParams.BlockIds {
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

func getBucket(ctx context.Context, params *queryBlocksSeriesParams) (objstore.Bucket, error) {
	if params.BucketName != "" {
		return getRemoteBucket(ctx, params)
	} else {
		return filesystem.NewBucket(params.LocalPath)
	}
}

func getRemoteBucket(ctx context.Context, params *queryBlocksSeriesParams) (objstore.Bucket, error) {
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
