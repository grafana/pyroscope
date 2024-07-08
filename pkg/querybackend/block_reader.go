package querybackend

import (
	"context"
	"io"
	"strconv"

	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util"
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

// TODO:
//  - All the requested report types should be obtained in a single pass:
//    in the prototype we won't do this. Instead, query frontend will call
//    the query backend multiple times: one report type - one query.
//  - Use a worker pool instead of the errgroup.

func (b *BlockReader) Invoke(ctx context.Context, req *querybackendv1.InvokeRequest) (*querybackendv1.InvokeResponse, error) {
	m := newMerger()
	g, ctx := errgroup.WithContext(ctx)
	for _, block := range req.Blocks {
		o := newObject(b.storage, block)
		for _, svc := range block.TenantServices {
			g.Go(util.RecoverPanic(func() error {
				return m.merge(o.queryTenantService(ctx, svc))
			}))
		}
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return m.response()
}

// TODO: Better naming?

type object struct {
	storage objstore.Bucket
	path    string
	meta    *metastorev1.BlockMeta
}

func newObject(storage objstore.Bucket, meta *metastorev1.BlockMeta) *object {
	return &object{
		storage: storage,
		path:    objectPath(meta),
		meta:    meta,
	}
}

func objectPath(md *metastorev1.BlockMeta) string {
	return objectDir(md) + strconv.Itoa(int(md.Shard)) + "/anon/" + md.Id + "/data.bin"
}

const (
	segmentDirPath = "segments/"
	blockDirPath   = "blocks/"
)

func objectDir(md *metastorev1.BlockMeta) string {
	if md.CompactionLevel == 0 {
		return segmentDirPath
	}
	return blockDirPath
}

func (b *object) sectionByIndex(
	ctx context.Context,
	svc *metastorev1.TenantService,
	idx int,
) (io.ReadCloser, error) {
	off := svc.TableOfContents[idx]
	var next uint64
	if idx == len(svc.TableOfContents)-1 {
		next = svc.Size + svc.TableOfContents[0]
	} else {
		next = svc.TableOfContents[idx+1]
	}
	return b.storage.GetRange(ctx, b.path, int64(off), int64(next-off))
}

const (
	profileTableSectionIdx = 0
	tsdbSectionIdx         = 1
	symdbSectionIdx        = 2
)

func (b *object) queryTenantService(
	ctx context.Context,
	svc *metastorev1.TenantService,
) (*querybackendv1.InvokeResponse, error) {
	// TODO
	return nil, nil
}
