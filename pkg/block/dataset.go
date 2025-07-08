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

type DatasetFormat uint32

const (
	DatasetFormat0 DatasetFormat = iota
	DatasetFormat1
)

type Section uint32

const (
	SectionProfiles Section = iota
	SectionTSDB
	SectionSymbols
	SectionDatasetIndex
)

type sectionDesc struct {
	// The section entry index in the table of contents.
	index int
	// The name is only used in log and error messages.
	name string
}

var (
	// Format: section => desc
	sections = [...][]sectionDesc{
		DatasetFormat0: {
			SectionProfiles: sectionDesc{index: 0, name: "profiles"},
			SectionTSDB:     sectionDesc{index: 1, name: "tsdb"},
			SectionSymbols:  sectionDesc{index: 2, name: "symbols"},
		},
		DatasetFormat1: {
			// The dataset index can be used instead of the tsdb section of the
			// dataset in cases where SeriesIndex is not used. Therefore, it has
			// an alias record: if a query accesses the tsdb index of the dataset,
			// it will access the tenant-wide dataset index.
			SectionDatasetIndex: sectionDesc{index: 0, name: "dataset_tsdb_index"},
			SectionTSDB:         sectionDesc{index: 0, name: "dataset_tsdb_index"},
		},
	}
)

func (sc Section) open(ctx context.Context, s *Dataset) (err error) {
	switch sc {
	case SectionTSDB:
		return openTSDB(ctx, s)
	case SectionSymbols:
		return openSymbols(ctx, s)
	case SectionProfiles:
		return openProfileTable(ctx, s)
	case SectionDatasetIndex:
		return openDatasetIndex(ctx, s)
	default:
		panic(fmt.Sprintf("bug: unknown section: %d", sc))
	}
}

type Dataset struct {
	tenant string
	name   string

	meta *metastorev1.Dataset
	obj  *Object

	refs refctr.Counter
	buf  *bufferpool.Buffer
	err  error

	tsdb     *tsdbBuffer
	symbols  *symdb.Reader
	profiles *ParquetFile

	memSize int
}

func NewDataset(meta *metastorev1.Dataset, obj *Object) *Dataset {
	return &Dataset{
		tenant:  obj.meta.StringTable[meta.Tenant],
		name:    obj.meta.StringTable[meta.Name],
		meta:    meta,
		obj:     obj,
		memSize: defaultTenantDatasetSizeLoadInMemory,
	}
}

type DatasetOption func(*Dataset)

func WithDatasetMaxSizeLoadInMemory(size int) DatasetOption {
	return func(s *Dataset) {
		s.memSize = size
	}
}

// Open opens the dataset, initializing the sections specified.
//
// Open may be called multiple times concurrently, but the dataset
// is only initialized once. While it is possible to open the dataset
// repeatedly after close, the caller must pass the failure reason to
// the CloseWithError call, preventing further use, if applicable.
func (s *Dataset) Open(ctx context.Context, sections ...Section) error {
	return s.refs.IncErr(func() error {
		if err := s.open(ctx, sections...); err != nil {
			return fmt.Errorf("%w (%s)", err, s.obj.meta.Id)
		}
		return nil
	})
}

func (s *Dataset) open(ctx context.Context, sections ...Section) (err error) {
	if s.err != nil {
		// The dataset has already been closed with an error.
		return s.err
	}
	if err = s.obj.Open(ctx); err != nil {
		return fmt.Errorf("failed to open object: %w", err)
	}
	defer func() {
		// Close the object here because the dataset won't be
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
			if openErr := sc.open(ctx, s); openErr != nil {
				return fmt.Errorf("opening section %v: %w", s.section(sc).name, openErr)
			}
			return nil
		}))
	}
	return g.Wait()
}

func (s *Dataset) Close() error { return s.CloseWithError(nil) }

// CloseWithError closes the dataset and disposes all the resources
// associated with it.
//
// Any further attempts to open the dataset will return the provided error.
func (s *Dataset) CloseWithError(err error) (closeErr error) {
	s.refs.Dec(func() {
		closeErr = s.closeErr(err)
	})
	return closeErr
}

func (s *Dataset) closeErr(err error) error {
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

func (s *Dataset) TenantID() string { return s.tenant }

func (s *Dataset) Name() string { return s.name }

func (s *Dataset) Metadata() *metastorev1.Dataset { return s.meta }

func (s *Dataset) Profiles() *ParquetFile { return s.profiles }

func (s *Dataset) ProfileRowReader() parquet.RowReader { return s.profiles.RowReader() }

func (s *Dataset) Symbols() symdb.SymbolsReader { return s.symbols }

func (s *Dataset) Index() phlaredb.IndexReader { return s.tsdb.index }

// Offset of the dataset section within the object.
func (s *Dataset) offset() uint64 { return s.meta.TableOfContents[0] }

func (s *Dataset) section(sc Section) sectionDesc {
	if int(s.meta.Format) >= len(sections) {
		panic(fmt.Sprintf("bug: unknown dataset format: %d", s.meta.Format))
	}
	f := sections[s.meta.Format]
	if int(sc) >= len(f) {
		panic(fmt.Sprintf("bug: invalid section index: %d", int(sc)))
	}
	return f[sc]
}

func (s *Dataset) sectionOffset(sc Section) int64 {
	return int64(s.meta.TableOfContents[s.section(sc).index])
}

func (s *Dataset) sectionSize(sc Section) int64 {
	idx := s.section(sc).index
	off := s.meta.TableOfContents[idx]
	var next uint64
	if idx == len(s.meta.TableOfContents)-1 {
		next = s.offset() + s.meta.Size
	} else {
		next = s.meta.TableOfContents[idx+1]
	}
	return int64(next - off)
}

func (s *Dataset) inMemoryBuffer() []byte {
	if s.obj.buf != nil {
		// If the entire object is loaded into memory,
		// return the dataset sub-slice.
		lo := s.offset()
		hi := lo + s.meta.Size
		buf := s.obj.buf.B
		return buf[lo:hi]
	}
	if s.buf != nil {
		return s.buf.B
	}
	return nil
}

func (s *Dataset) inMemoryBucket(buf []byte) objstore.Bucket {
	bucket := memory.NewInMemBucket()
	bucket.Set(s.obj.path, buf)
	return objstore.NewBucket(bucket)
}
