package block

import (
	"context"
	"fmt"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/bufferpool"
	"github.com/grafana/pyroscope/pkg/util/refctr"
)

type TenantService struct {
	meta *metastorev1.TenantService
	obj  *Object

	refs refctr.Counter
	buf  *bufferpool.Buffer
	err  error

	tsdb     *tsdbBuffer
	symbols  *symdb.Reader
	profiles *ParquetFile

	memSize int
}

func NewTenantService(meta *metastorev1.TenantService, obj *Object) *TenantService {
	return &TenantService{
		meta:    meta,
		obj:     obj,
		memSize: defaultTenantServiceSizeLoadInMemory,
	}
}

type TenantServiceOption func(*TenantService)

func WithTenantServiceMaxSizeLoadInMemory(size int) TenantServiceOption {
	return func(s *TenantService) {
		s.memSize = size
	}
}

// Open opens the service, initializing the sections specified.
//
// Open may be called multiple times concurrently, but the service
// is only initialized once. While it is possible to open the service
// repeatedly after close, the caller must pass the failure reason to
// the CloseWithError call, preventing further use, if applicable.
func (s *TenantService) Open(ctx context.Context, sections ...Section) (err error) {
	return s.refs.IncErr(func() error {
		return s.open(ctx, sections...)
	})
}

func (s *TenantService) open(ctx context.Context, sections ...Section) (err error) {
	if s.err != nil {
		// The tenant service has been already closed with an error.
		return s.err
	}
	if err = s.obj.Open(ctx); err != nil {
		return fmt.Errorf("failed to open object: %w", err)
	}
	defer func() {
		// Close the object here because the tenant service won't be
		// closed if it fails to open.
		if err != nil {
			_ = s.closeErr(err)
		}
	}()
	if s.obj.buf == nil && s.meta.Size < uint64(s.memSize) {
		s.buf = bufferpool.GetBuffer(int(s.meta.Size))
		off, size := int64(s.offset()), int64(s.meta.Size)
		if err = objstore.ReadRange(ctx, s.buf, s.obj.path, s.obj.storage, off, size); err != nil {
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

func (s *TenantService) Close() error { return s.CloseWithError(nil) }

// CloseWithError closes the tenant service and disposes all the resources
// associated with it.
//
// Any further attempts to open the service will return the provided error.
func (s *TenantService) CloseWithError(err error) (closeErr error) {
	s.refs.Dec(func() {
		closeErr = s.closeErr(err)
	})
	return closeErr
}

func (s *TenantService) closeErr(err error) error {
	s.err = err
	if s.buf != nil {
		bufferpool.Put(s.buf)
		s.buf = nil
	}
	var merr multierror.MultiError
	if s.tsdb != nil {
		merr.Add(s.tsdb.Close())
	}
	if s.symbols != nil {
		merr.Add(s.symbols.Close())
	}
	if s.profiles != nil {
		merr.Add(s.profiles.Close())
	}
	if s.obj != nil {
		merr.Add(s.obj.CloseWithError(err))
	}
	return merr.Err()
}

func (s *TenantService) Meta() *metastorev1.TenantService { return s.meta }

func (s *TenantService) Profiles() *ParquetFile { return s.profiles }

func (s *TenantService) ProfileRowReader() parquet.RowReader { return s.profiles.RowReader() }

func (s *TenantService) Symbols() symdb.SymbolsReader { return s.symbols }

func (s *TenantService) Index() phlaredb.IndexReader { return s.tsdb.index }

// Offset of the tenant service section within the object.
func (s *TenantService) offset() uint64 { return s.meta.TableOfContents[0] }

func (s *TenantService) sectionIndex(sc Section) int {
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

func (s *TenantService) sectionName(sc Section) string {
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

func (s *TenantService) sectionOffset(sc Section) int64 {
	return int64(s.meta.TableOfContents[s.sectionIndex(sc)])
}

func (s *TenantService) sectionSize(sc Section) int64 {
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

func (s *TenantService) inMemoryBuffer() []byte {
	if s.obj.buf != nil {
		// If the entire object is loaded into memory,
		// return the tenant service sub-slice.
		lo := s.offset()
		hi := lo + s.meta.Size
		buf := s.obj.buf.B
		return buf[lo:hi]
	}
	if s.buf != nil {
		// Otherwise, if the tenant service is loaded into memory
		// individually, return the buffer.
		return s.buf.B
	}
	// Otherwise, the tenant service is not loaded into memory.
	return nil
}

func (s *TenantService) inMemoryBucket(buf []byte) objstore.Bucket {
	bucket := memory.NewInMemBucket()
	bucket.Set(s.obj.path, buf)
	return objstore.NewBucket(bucket)
}
