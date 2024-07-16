package querybackend

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/multierror"
	"github.com/prometheus/common/model"
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
	"github.com/grafana/pyroscope/pkg/util/refctr"
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
		return nil, status.Errorf(codes.InvalidArgument, "request validation failed: %v", err)
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
	src       *querybackendv1.InvokeRequest
	matchers  []*labels.Matcher
	startTime int64 // Unix nano.
	endTime   int64 // Unix nano.
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
	// TODO: Validate the rest, just in case.
	r := request{
		src:       req,
		matchers:  matchers,
		startTime: model.Time(req.StartTime).UnixNano(),
		endTime:   model.Time(req.EndTime).UnixNano(),
	}
	return &r, nil
}

const (
	segmentDirPath    = "segments/"
	blockDirPath      = "blocks/"
	anonTenantDirName = "anon"
)

type section uint32

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

	refs refctr.Counter
	buf  *bytes.Buffer
	err  error
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
	return topLevel + strconv.Itoa(int(md.Shard)) + "/" + tenantDirName + "/" + md.Id + "/block.bin"
}

// open the object, loading the data into memory if it's small enough.
// open may be called multiple times concurrently, but the object is
// only initialized once. While it is possible to open the object
// repeatedly after close, the caller must pass the failure reason
// to the "close" call, preventing further use, if applicable.
func (obj *object) open(ctx context.Context) error {
	obj.err = obj.refs.Inc(func() error {
		return obj.doOpen(ctx)
	})
	return obj.err
}

func (obj *object) doOpen(ctx context.Context) error {
	if obj.err != nil {
		// In case if the object has been already closed with an error,
		// and then released, return the error immediately.
		return obj.err
	}
	// Estimate the size of the sections to process, and load the
	// data into memory, if it's small enough.
	if len(obj.meta.TenantServices) == 0 {
		panic("bug: invalid block meta: at least one section is expected")
	}
	obj.buf = new(bytes.Buffer) // TODO: Take from pool.
	if err := objstore.FetchRange(ctx, obj.buf, obj.path, obj.storage, 0, 0); err != nil {
		return fmt.Errorf("loading object into memory: %w", err)
	}
	return nil
}

// close the object, releasing all the acquired resources, once the last
// reference is released. If the provided error is not nil, the object will
// be marked as failed, preventing any further use.
func (obj *object) close(err error) {
	obj.refs.Dec(func() {
		obj.doClose(err)
	})
}

func (obj *object) doClose(err error) {
	if obj.err == nil {
		obj.err = err
	}
	obj.buf = nil // TODO: Release.
}

type tenantService struct {
	meta *metastorev1.TenantService
	obj  *object

	refs refctr.Counter
	buf  *bytes.Buffer
	err  error

	tsdb     *index.Reader
	symbols  *symdb.Reader
	profiles *parquetFile
}

func newTenantService(meta *metastorev1.TenantService, obj *object) *tenantService {
	return &tenantService{
		meta: meta,
		obj:  obj,
	}
}

func (s *tenantService) open(ctx context.Context, sections ...section) (err error) {
	s.err = s.refs.Inc(func() error {
		return s.doOpen(ctx, sections...)
	})
	return s.err
}

func (s *tenantService) doOpen(ctx context.Context, sections ...section) (err error) {
	if s.err != nil {
		// The tenant service has been already closed with an error.
		return s.err
	}
	if err = s.obj.open(ctx); err != nil {
		return fmt.Errorf("failed to open object: %w", err)
	}
	defer func() {
		// Close the object here because the tenant service won't be
		// closed if it fails to open.
		if err != nil {
			s.obj.close(err)
		}
	}()
	if s.obj.buf == nil && s.meta.Size < loadInMemorySizeThreshold {
		s.buf = new(bytes.Buffer) // TODO: Pool.
		off, size := int64(s.offset()), int64(s.meta.Size)
		if err = objstore.FetchRange(ctx, s.buf, s.obj.path, s.obj.storage, off, size); err != nil {
			return fmt.Errorf("loading sections into memory: %w", err)
		}
	}
	g, ctx := errgroup.WithContext(ctx)
	for _, sc := range sections {
		sc := sc
		g.Go(util.RecoverPanic(func() error {
			if err = sc.open(ctx, s); err != nil {
				return fmt.Errorf("openning section %v: %w", s.sectionName(sc), err)
			}
			return nil
		}))
	}
	return g.Wait()
}

func (s *tenantService) close(err error) {
	s.refs.Dec(func() {
		s.doClose(err)
	})
}

func (s *tenantService) doClose(err error) {
	if s.buf != nil {
		s.buf = nil // TODO: Release.
	}
	var m multierror.MultiError
	m.Add(s.err) // Preserve the existing error
	m.Add(err)   // Add the new error, if any.
	if s.tsdb != nil {
		m.Add(s.tsdb.Close())
	}
	if s.symbols != nil {
		m.Add(s.symbols.Close())
	}
	if s.profiles != nil {
		m.Add(s.profiles.Close())
	}
	s.err = m.Err()
}

// Offset of the tenant service section within the object.
func (s *tenantService) offset() uint64 { return s.meta.TableOfContents[0] }

func (s *tenantService) sectionIndex(sc section) int {
	var n []int
	switch s.obj.meta.FormatVersion {
	default:
		n = sectionIndices[1]
	}
	if int(sc) >= len(n) {
		panic(fmt.Sprintf("bug: invalid section index: %d (total: %d)", sc, len(n)))
	}
	return n[sc]
}

func (s *tenantService) sectionName(sc section) string {
	var n []string
	switch s.obj.meta.FormatVersion {
	default:
		n = sectionNames[1]
	}
	if int(sc) >= len(n) {
		panic(fmt.Sprintf("bug: invalid section index: %d (total: %d)", sc, len(n)))
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
		lo := s.offset()
		hi := lo + s.meta.Size
		buf := s.obj.buf.Bytes()
		return buf[lo:hi]
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
	bucket := memory.NewInMemBucket()
	bucket.Set(s.obj.path, buf)
	return objstore.NewBucket(bucket)
}
