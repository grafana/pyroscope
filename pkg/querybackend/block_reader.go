package querybackend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/go-kit/log"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util"
)

// Block reader reads objects from the object storage. Each block is currently
// represented by a single object.
//
// An object consists of a set of "tenant services" – regions within the block
// that include data of a specific tenant service. Each such tenant service
// consists of 3 sections: profile table, TSDB, and symbol database.
//
// A single Invoke request typically spans multiple blocks (objects).
// Querying an object involves processing multiple tenant services in parallel.
// Multiple parallel queries can be executed on the same tenant service.
//
// Thus, queries share the same "execution context": the object and a tenant
// service:
//
// object-a    service-a   query-a
//                         query-b
//             service-b   query-a
//                         query-b
// object-b    service-a   query-a
//                         query-b
//             service-b   query-a
//                         query-b
//

type BlockReader struct {
	log     log.Logger
	storage objstore.Bucket

	// TODO Next:
	//  - In-memory threshold option.
	//  - Series API
	//  - In-memory bucket.
	//  - parquet reader at.
	//  - Metrics API
	//  - symdb reader.
	//  - Tree API
	//  - Buffer pool.

	// TODO:
	//  - Store the object size in metadata.
	//  - Separate storages for segments and compacted blocks.
	//  - Distributed cache client.
	//  - Local cache? Useful for all-in-one deployments.
	//  - Use a worker pool instead of the errgroup.
	//  - Reusable query context.
	//  - Query pipelining: currently, queries share the same context,
	//    and reuse resources, but the data is processed independently.
	//    Instead, they should share the processing pipeline, if possible.
}

func NewBlockReader(logger log.Logger, storage objstore.Bucket) *BlockReader {
	return &BlockReader{
		log:     logger,
		storage: storage,
	}
}

func (b *BlockReader) Invoke(
	ctx context.Context,
	req *querybackendv1.InvokeRequest,
) (*querybackendv1.InvokeResponse, error) {
	vr, err := validateRequest(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "request validation failed")
	}
	g, ctx := errgroup.WithContext(ctx)
	m := newMerger()
	for _, block := range req.QueryPlan.Blocks {
		obj := newObject(b.storage, block)
		for _, meta := range block.TenantServices {
			c := newQueryContext(ctx, b.log, meta, vr, obj)
			for _, query := range req.Query {
				q := query
				g.Go(util.RecoverPanic(func() error {
					r, err := executeQuery(c, q)
					if err != nil {
						return err
					}
					return m.mergeReport(r)
				}))
			}
		}
	}
	if err = g.Wait(); err != nil {
		return nil, err
	}
	return m.response()
}

type request struct {
	src      *querybackendv1.InvokeRequest
	matchers []*labels.Matcher
}

func validateRequest(req *querybackendv1.InvokeRequest) (*request, error) {
	if len(req.Query) == 0 {
		return nil, fmt.Errorf("no queries provided")
	}
	if req.QueryPlan == nil || len(req.QueryPlan.Blocks) == 0 {
		return nil, fmt.Errorf("no blocks planned")
	}
	matchers, err := parser.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("label selection is invalid: %w", err)
	}
	r := request{
		src:      req,
		matchers: matchers,
	}
	// TODO: Validate the rest.
	return &r, nil
}

const (
	segmentDirPath    = "segments/"
	blockDirPath      = "blocks/"
	anonTenantDirName = "anon"
)

type section int

const (
	// Table of contents sections.
	_ section = iota
	sectionProfiles
	sectionTSDB
	sectionSymbols
)

var (
	// Version-specific.
	sectionNames   = [...][]string{1: {"invalid", "profiles", "tsdb", "symbols"}}
	sectionIndices = [...][]int{1: {-1, 0, 1, 2}}
)

func (sc section) open(ctx context.Context, s *tenantService) (err error) {
	switch sc {
	case sectionTSDB:
		return openTSDB(ctx, s)
	case sectionSymbols:
		return openSymbols(ctx, s)
	case sectionProfiles:
		return openProfileTable(ctx, s)
	default:
		panic(fmt.Sprintf("bug: unknown section: %d", sc))
	}
}

const loadInMemorySizeThreshold = 1 << 20

// Object represents a block or a segment in the object storage.
type object struct {
	storage objstore.Bucket
	path    string
	meta    *metastorev1.BlockMeta

	openOnce sync.Once
	buf      *bytes.Buffer
	err      error
}

func newObject(storage objstore.Bucket, meta *metastorev1.BlockMeta) *object {
	return &object{
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

func (obj *object) open(ctx context.Context) (err error) {
	obj.openOnce.Do(func() {
		// Estimate the size of the sections to process, and load the
		// data into memory, if it's small enough.
		// NOTE(kolesnikovae): This could be done at the planning
		// step and stored in the query execution plan.
		if len(obj.meta.TenantServices) == 0 {
			panic("bug: invalid block meta: at least one section is expected")
		}
		// The order of services matches the physical placement.
		// Therefore, we can find the range that spans all of them.
		off := int64(obj.meta.TenantServices[0].TableOfContents[0])
		lastEntry := obj.meta.TenantServices[len(obj.meta.TenantServices)-1]
		length := int64(lastEntry.TableOfContents[0]+lastEntry.Size) - off
		if length > loadInMemorySizeThreshold {
			// The object won't be loaded into memory. However, each
			// of the sections is to be evaluated separately, and might
			// be loaded individually.
			return
		}
		obj.buf = new(bytes.Buffer)
		if err = objstore.FetchRange(ctx, obj.buf, obj.path, obj.storage, off, length); err != nil {
			obj.err = fmt.Errorf("loading object into memory: %w", err)
		}
	})
	return obj.err
}

type tenantService struct {
	meta *metastorev1.TenantService
	obj  *object

	openOnce sync.Once
	buf      *bytes.Buffer
	err      error

	tsdb     *index.Reader
	symbols  *symdb.Reader
	profiles *parquet.File
}

func newTenantService(meta *metastorev1.TenantService, obj *object) *tenantService {
	return &tenantService{
		meta: meta,
		obj:  obj,
	}
}

func (s *tenantService) open(ctx context.Context, sections ...section) (err error) {
	s.openOnce.Do(func() {
		if err = s.obj.open(ctx); err != nil {
			s.err = fmt.Errorf("failed to open object: %w", err)
			return
		}
		if s.obj.buf == nil && s.meta.Size < loadInMemorySizeThreshold {
			s.buf = new(bytes.Buffer)
			off, size := int64(s.offset()), int64(s.meta.Size)
			if err = objstore.FetchRange(ctx, s.buf, s.obj.path, s.obj.storage, off, size); err != nil {
				s.err = fmt.Errorf("loading sections into memory: %w", err)
				return
			}
		}
		g, ctx := errgroup.WithContext(ctx)
		for _, sc := range sections {
			sc := sc
			g.Go(func() error {
				if err = sc.open(ctx, s); err != nil {
					return fmt.Errorf("openning section %v: %w", s.sectionName(sc), err)
				}
				return nil
			})
		}
		s.err = g.Wait()
	})
	return s.err
}

// Offset of the tenant service section within the object.
func (s *tenantService) offset() uint64 { return s.meta.TableOfContents[0] }

func (s *tenantService) sectionIndex(sc section) int {
	var n []int
	switch s.obj.meta.FormatVersion {
	default:
		n = sectionIndices[1]
	}
	if sc <= 0 || int(sc) >= len(n) {
		return -1
	}
	return n[sc]
}

func (s *tenantService) sectionName(sc section) string {
	var n []string
	switch s.obj.meta.FormatVersion {
	default:
		n = sectionNames[1]
	}
	if sc <= 0 || int(sc) >= len(n) {
		return "invalid"
	}
	return n[sc]
}

func (s *tenantService) sectionOffset(sc section) int64 {
	return int64(s.meta.TableOfContents[s.sectionIndex(sc)])
}

func (s *tenantService) sectionSize(sc section) int64 {
	idx := s.sectionIndex(sc)
	off := s.meta.TableOfContents[idx]
	var next uint64
	if idx == len(s.meta.TableOfContents)-1 {
		next = s.offset() + s.meta.Size
	} else {
		next = s.meta.TableOfContents[idx+1]
	}
	return int64(next - off)
}

func (s *tenantService) inMemoryBuffer() []byte {
	if s.obj.buf != nil {
		// If the entire object is loaded into memory,
		// return the tenant service sub-slice.
		return s.obj.buf.Bytes()[s.offset() : s.offset()+s.meta.Size]
	}
	if s.buf != nil {
		// Otherwise, if the tenant service is loaded into memory
		// individually, return the buffer.
		return s.buf.Bytes()
	}
	// Otherwise, the tenant service is not loaded into memory.
	return nil
}

func (s *tenantService) inMemoryBucket(buf []byte) objstore.Bucket {
	bucket := memory.NewBucketClient()
	_ = bucket.Upload(context.Background(), s.obj.path, bytes.NewReader(buf))
	return objstore.NewBucket(bucket)
}

type readerWithOffset struct {
	offset int64
	objstore.Bucket
}

func newReaderWithOffset(bucket objstore.Bucket, offset int64) *readerWithOffset {
	return &readerWithOffset{
		Bucket: bucket,
		offset: offset,
	}
}

func (r readerWithOffset) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	return r.Bucket.GetRange(ctx, name, r.offset+off, length)
}
