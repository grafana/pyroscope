package block

import (
	"context"
	"fmt"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/v2/pkg/util"
	"github.com/grafana/pyroscope/v2/pkg/util/bufferpool"
	"github.com/grafana/pyroscope/v2/pkg/util/refctr"
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

// DatasetWeight holds the section-level size breakdown of a dataset.
// For Format1 (tenant-wide TSDB index) datasets, IndexLookupCount is 1
// and only TSDBBytes is set; the real profile and symbol sizes are
// unknown until the query backend resolves the datasets at runtime.
type DatasetWeight struct {
	ProfilesBytes    uint64
	TSDBBytes        uint64
	SymbolsBytes     uint64
	IndexLookupCount int
}

// WeightOf computes the section size breakdown for a dataset from its
// table of contents.
func WeightOf(ds *metastorev1.Dataset) DatasetWeight {
	toc := ds.TableOfContents
	switch {
	case len(toc) >= 3: // Format0: profiles, tsdb, symbols
		return DatasetWeight{
			ProfilesBytes: toc[1] - toc[0],
			TSDBBytes:     toc[2] - toc[1],
			SymbolsBytes:  (toc[0] + ds.Size) - toc[2],
		}
	case len(toc) == 1: // Format1: tenant-wide dataset index
		return DatasetWeight{
			TSDBBytes:        ds.Size,
			IndexLookupCount: 1,
		}
	}
	return DatasetWeight{}
}

// Add accumulates another DatasetWeight into this one.
func (w *DatasetWeight) Add(other DatasetWeight) {
	w.ProfilesBytes += other.ProfilesBytes
	w.TSDBBytes += other.TSDBBytes
	w.SymbolsBytes += other.SymbolsBytes
	w.IndexLookupCount += other.IndexLookupCount
}

// Total returns the sum of all section bytes.
func (w DatasetWeight) Total() uint64 {
	return w.ProfilesBytes + w.TSDBBytes + w.SymbolsBytes
}

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

type sharedRegion struct {
	refs refctr.Counter
	buf  *bufferpool.Buffer
	off  int64
	size int64
}

func (r *sharedRegion) fetch(ctx context.Context, path string, storage objstore.BucketReader) error {
	return r.refs.IncErr(func() error {
		buf := bufferpool.GetBuffer(int(r.size))
		if err := objstore.ReadRange(ctx, buf, path, storage, r.off, r.size); err != nil {
			bufferpool.Put(buf)
			return err
		}
		r.buf = buf
		return nil
	})
}

func (r *sharedRegion) release() {
	r.refs.Dec(func() {
		if r.buf != nil {
			bufferpool.Put(r.buf)
			r.buf = nil
		}
	})
}

type Dataset struct {
	tenant string
	name   string

	meta *metastorev1.Dataset
	obj  *Object

	refs   refctr.Counter
	buf    *bufferpool.Buffer
	bufOff int64
	region *sharedRegion
	err    error

	tsdb     *tsdbBuffer
	symbols  *symdb.Reader
	profiles *ParquetFile

	memSize int
}

func NewDataset(meta *metastorev1.Dataset, obj *Object, opts ...DatasetOption) *Dataset {
	d := &Dataset{
		tenant:  obj.meta.StringTable[meta.Tenant],
		name:    obj.meta.StringTable[meta.Name],
		meta:    meta,
		obj:     obj,
		memSize: DefaultTenantDatasetSizeLoadInMemory,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

const (
	smallDatasetCoalesceMaxGap   = 256 << 10
	smallDatasetCoalesceMaxRange = 8 << 20
)

// NewCoalescedDatasets binds dataset metas to the object and groups nearby
// small (< memSize) datasets into shared coalesced range reads covering only
// the requested sections so that opening them issues one GetRange per run
// rather than one GetRange per dataset.
func NewCoalescedDatasets(metas []*metastorev1.Dataset, obj *Object, neededSections []Section, opts ...DatasetOption) []*Dataset {
	datasets := make([]*Dataset, len(metas))
	for i, m := range metas {
		datasets[i] = NewDataset(m, obj, opts...)
	}
	if len(datasets) < 2 {
		return datasets
	}

	type cand struct {
		idx int
		lo  int64
		hi  int64
	}
	cands := make([]cand, 0, len(datasets))
	for i, d := range datasets {
		off, sz := d.neededSpan(neededSections)
		if sz <= 0 || sz >= int64(d.memSize) {
			continue
		}
		cands = append(cands, cand{idx: i, lo: off, hi: off + sz})
	}
	if len(cands) < 2 {
		return datasets
	}

	// Datasets in metadata are typically already ordered by offset, but sort
	// defensively in case a caller reordered them.
	for i := 1; i < len(cands); i++ {
		if cands[i].lo < cands[i-1].lo {
			// Fallback insertion sort on small slice without extra import if needed,
			// or slices.SortFunc in parquet_coalescer.go.
			sortCands := func(s []cand) {
				for a := 1; a < len(s); a++ {
					for b := a; b > 0 && s[b].lo < s[b-1].lo; b-- {
						s[b], s[b-1] = s[b-1], s[b]
					}
				}
			}
			sortCands(cands)
			break
		}
	}

	runStart := 0
	flushRun := func(start, end int) {
		if end-start < 2 {
			return
		}
		lo := cands[start].lo
		hi := cands[start].hi
		for k := start + 1; k < end; k++ {
			if cands[k].hi > hi {
				hi = cands[k].hi
			}
		}
		reg := &sharedRegion{off: lo, size: hi - lo}
		for k := start; k < end; k++ {
			datasets[cands[k].idx].region = reg
		}
	}

	curLo := cands[0].lo
	curHi := cands[0].hi
	for i := 1; i < len(cands); i++ {
		nextHi := max(curHi, cands[i].hi)
		if cands[i].lo >= curLo && cands[i].lo-curHi <= smallDatasetCoalesceMaxGap && nextHi-curLo <= smallDatasetCoalesceMaxRange {
			curHi = nextHi
			continue
		}
		flushRun(runStart, i)
		runStart = i
		curLo = cands[i].lo
		curHi = cands[i].hi
	}
	flushRun(runStart, len(cands))

	return datasets
}

// neededSpan returns the minimal contiguous [offset, size) covering all
// requested sections for this dataset.
func (s *Dataset) neededSpan(needed []Section) (int64, int64) {
	if len(s.meta.TableOfContents) == 0 || s.meta.Size == 0 {
		return 0, 0
	}
	start := int64(s.offset())
	if len(needed) == 0 || DatasetFormat(s.meta.Format) != DatasetFormat0 {
		return start, int64(s.meta.Size)
	}
	minStart := start + int64(s.meta.Size)
	var maxEnd int64
	for _, sc := range needed {
		off := s.sectionOffset(sc)
		end := off + s.sectionSize(sc)
		if off < minStart {
			minStart = off
		}
		if end > maxEnd {
			maxEnd = end
		}
	}
	if maxEnd <= minStart {
		return start, int64(s.meta.Size)
	}
	return minStart, maxEnd - minStart
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
	var releaseRegion bool
	defer func() {
		// Close the object here because the dataset won't be
		// closed if it fails to open.
		if err != nil {
			if !releaseRegion {
				// Prevent closeErr from decrementing a region we haven't acquired.
				s.region = nil
			}
			_ = s.closeErr(err)
		}
	}()
	if s.obj.buf == nil {
		if s.region != nil {
			if err = s.region.fetch(ctx, s.obj.path, s.obj.storage); err != nil {
				return fmt.Errorf("loading coalesced dataset region into memory: %w", err)
			}
			releaseRegion = true
		} else if off, size := s.neededSpan(sections); size > 0 && size < int64(s.memSize) {
			s.buf = bufferpool.GetBuffer(int(size))
			s.bufOff = off
			if err = objstore.ReadRange(ctx, s.buf, s.obj.path, s.obj.storage, off, size); err != nil {
				return fmt.Errorf("loading sections into memory: %w", err)
			}
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
	if s.region != nil {
		s.region.release()
		s.region = nil
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

// inMemorySection returns an in-memory buffer and the relative offset of sc
// within that buffer if sc is completely covered by an already-loaded buffer.
func (s *Dataset) inMemorySection(sc Section) ([]byte, int64) {
	secOff := s.sectionOffset(sc)
	secEnd := secOff + s.sectionSize(sc)
	if s.obj.buf != nil {
		lo := int64(s.offset())
		hi := lo + int64(s.meta.Size)
		if secOff >= lo && secEnd <= hi {
			return s.obj.buf.B[lo:hi], secOff - lo
		}
	}
	if s.region != nil && s.region.buf != nil {
		lo := s.region.off
		hi := lo + int64(len(s.region.buf.B))
		if secOff >= lo && secEnd <= hi {
			return s.region.buf.B, secOff - lo
		}
	}
	if s.buf != nil {
		lo := s.bufOff
		hi := lo + int64(len(s.buf.B))
		if secOff >= lo && secEnd <= hi {
			return s.buf.B, secOff - lo
		}
	}
	return nil, 0
}

func (s *Dataset) inMemoryBucket(buf []byte) objstore.Bucket {
	bucket := memory.NewInMemBucket()
	bucket.Set(s.obj.path, buf)
	return objstore.NewBucket(bucket)
}
