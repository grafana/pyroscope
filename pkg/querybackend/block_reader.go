package querybackend

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util"
)

type BlockReader struct {
	log     log.Logger
	storage objstore.Bucket

	// TODO:
	//  - Separate storages for segments and compacted blocks.
	//  - Distributed cache client.
	//  - Local cache? Useful for all-in-one deployments.
}

func NewBlockReader(logger log.Logger, storage objstore.Bucket) *BlockReader {
	return &BlockReader{
		log:     logger,
		storage: storage,
	}
}

// TODO:
//  - All the requested report types should be obtained in a single pass/context:
//    in the prototype we won't do this. Instead, query frontend will call
//    the query backend multiple times: one report type - one query.

func (b *BlockReader) Invoke(ctx context.Context, req *querybackendv1.InvokeRequest) (*querybackendv1.InvokeResponse, error) {
	//  TODO: Use a worker pool instead of the errgroup.
	g, ctx := errgroup.WithContext(ctx)
	m := newMerger()
	for _, block := range req.Blocks {
		obj := newObject(b.storage, block)
		for _, svc := range block.TenantServices {
			svc := svc
			for _, query := range req.Query {
				g.Go(util.RecoverPanic(func() error {
					q := &queryContext{
						ctx: ctx,
						log: b.log,
						req: req,
						svc: svc,
						obj: obj,
					}
					r, err := q.execute(query)
					if err != nil {
						return err
					}
					return m.mergeReport(r)
				}))
			}
		}
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return m.response()
}

const (
	segmentDirPath    = "segments/"
	blockDirPath      = "blocks/"
	anonTenantDirName = "anon"
)

const (
	// Table of contents sections.
	// Version-specific.

	profileTableSectionIdx = iota
	tsdbSectionIdx
	symdbSectionIdx
)

// Object represents a block or a segment in the object storage.
// TODO: Better naming?
// TODO: Prefetch small objects into memory.
type object struct {
	storage objstore.Bucket
	path    string
	meta    *metastorev1.BlockMeta
}

func newObject(storage objstore.Bucket, meta *metastorev1.BlockMeta) object {
	return object{
		storage: storage,
		path:    objectPath(meta),
		meta:    meta,
	}
}

func objectPath(md *metastorev1.BlockMeta) string {
	topLevel := blockDirPath
	tenantDirName := md.TenantId
	if md.CompactionLevel == 0 {
		topLevel = segmentDirPath
		tenantDirName = anonTenantDirName
	}
	return topLevel + strconv.Itoa(int(md.Shard)) + "/" + tenantDirName + "/" + md.Id + "/data.bin"
}

func (object) sectionOffsetByIndex(svc *metastorev1.TenantService, idx int) int64 {
	return int64(svc.TableOfContents[idx])
}

func (b object) sectionSizeByIndex(svc *metastorev1.TenantService, idx int) int64 {
	off := b.sectionOffsetByIndex(svc, idx)
	var next uint64
	if idx == len(svc.TableOfContents)-1 {
		next = svc.Size + svc.TableOfContents[0]
	} else {
		next = svc.TableOfContents[idx+1]
	}
	return int64(next) - off
}

func (b object) sectionReaderByIndex(
	ctx context.Context,
	svc *metastorev1.TenantService,
	idx int,
) (io.ReadCloser, error) {
	return b.storage.GetRange(ctx, b.path,
		b.sectionOffsetByIndex(svc, idx),
		b.sectionSizeByIndex(svc, idx))
}

func (b object) openTsdb(ctx context.Context, svc *metastorev1.TenantService) (*index.Reader, error) {
	r, err := b.sectionReaderByIndex(ctx, svc, tsdbSectionIdx)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, b.sectionSizeByIndex(svc, tsdbSectionIdx))
	if _, err = io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return index.NewReader(index.RealByteSlice(buf))
}

func (b object) openSymdb(ctx context.Context, svc *metastorev1.TenantService) (*symdb.Reader, error) {
	r, err := b.sectionReaderByIndex(ctx, svc, symdbSectionIdx)
	if err != nil {
		return nil, err
	}
	return symdb.OpenReader(r) // TODO: Implement.
}

func (b object) openProfileTable(ctx context.Context, svc *metastorev1.TenantService) (*parquet.File, error) {
	offset := b.sectionOffsetByIndex(svc, profileTableSectionIdx)
	size := b.sectionSizeByIndex(svc, profileTableSectionIdx)
	rat := &objstore.ReaderAt{
		Context:        ctx,
		GetRangeReader: b.storage,
		Name:           b.path,
		Offset:         offset,
	}
	return parquet.OpenFile(rat, size) // TODO: options, etc.
}

type queryContext struct {
	ctx context.Context
	log log.Logger
	req *querybackendv1.InvokeRequest
	svc *metastorev1.TenantService
	obj object
}

func (q *queryContext) execute(query *querybackendv1.Query) (*querybackendv1.Report, error) {
	// TODO: Replace with a map type => handler?
	_ = level.Info(q.log).Log("msg", "executing query", "query", query.QueryType)
	// TODO: Implement query methods.
	switch x := query.QueryType.(type) {
	case *querybackendv1.Query_LabelNames:
		return nil, nil
	case *querybackendv1.Query_LabelValues:
		return nil, nil
	case *querybackendv1.Query_SeriesLabels:
		return nil, nil
	case *querybackendv1.Query_Metrics:
		return nil, nil
	case *querybackendv1.Query_Tree:
		return q.queryTree(x.Tree)
	default:
		return nil, fmt.Errorf("unknown query type %T", x)
	}
}
