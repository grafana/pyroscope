package querybackend

import (
	"context"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
)

type BlockReader struct {
	storage objstore.Bucket
	// TODO:
	//  - Separate storages for segments and compacted blocks.
	//  - Distributed cache client.
	//  - Local cache? Useful for all-in-one deployments.
}

func NewBlockReader(storage objstore.Bucket) *BlockReader {
	return &BlockReader{storage: storage}
}

func (b BlockReader) Invoke(ctx context.Context, req *querybackendv1.InvokeRequest) (*querybackendv1.InvokeResponse, error) {
	// TODO:
	//  - Block querier for the new block layout.
	//  - Note that the request can only have a single block meta, but multiple
	//    tenant service sections.
	//  - All the requested report types should be obtained in a single pass:
	//    in the prototype we won't do this. Instead, query frontend will call
	//    the query backend multiple times: one report type - one query.
	//
	// We currently have something like this:
	// phlaredb.NewSingleBlockQuerierFromMeta(ctx, b.storage, req.Blocks[0])
	return new(querybackendv1.InvokeResponse), nil
}
