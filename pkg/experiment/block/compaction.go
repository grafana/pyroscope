package block

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	memindex "github.com/grafana/pyroscope/pkg/experiment/ingester/memdb/index"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util"
)

var (
	ErrNoBlocksToMerge    = fmt.Errorf("no blocks to merge")
	ErrShardMergeMismatch = fmt.Errorf("only blocks from the same shard can be merged")
)

type CompactionOption func(*compactionConfig)

func WithCompactionObjectOptions(options ...ObjectOption) CompactionOption {
	return func(p *compactionConfig) {
		p.objectOptions = append(p.objectOptions, options...)
	}
}

func WithCompactionTempDir(tempdir string) CompactionOption {
	return func(p *compactionConfig) {
		p.tempdir = tempdir
	}
}

func WithCompactionDestination(storage objstore.Bucket) CompactionOption {
	return func(p *compactionConfig) {
		p.destination = storage
	}
}

type compactionConfig struct {
	objectOptions []ObjectOption
	tempdir       string
	source        objstore.BucketReader
	destination   objstore.Bucket
}

func Compact(
	ctx context.Context,
	blocks []*metastorev1.BlockMeta,
	storage objstore.Bucket,
	options ...CompactionOption,
) (m []*metastorev1.BlockMeta, err error) {
	c := &compactionConfig{
		tempdir:     os.TempDir(),
		source:      storage,
		destination: storage,
	}
	for _, option := range options {
		option(c)
	}

	objects := ObjectsFromMetas(storage, blocks, c.objectOptions...)
	plan, err := PlanCompaction(objects)
	if err != nil {
		return nil, err
	}

	if err = objects.Open(ctx); err != nil {
		return nil, fmt.Errorf("objects.Open: %w", err)
	}
	defer func() {
		_ = objects.Close()
	}()

	compacted := make([]*metastorev1.BlockMeta, 0, len(plan))
	for _, p := range plan {
		md, compactionErr := p.Compact(ctx, c.destination, c.tempdir)
		if compactionErr != nil {
			return nil, err
		}
		compacted = append(compacted, md)
	}

	return compacted, nil
}

func PlanCompaction(objects Objects) ([]*CompactionPlan, error) {
	if len(objects) == 0 {
		// Even if there's just a single object, we still need to rewrite it.
		return nil, ErrNoBlocksToMerge
	}

	r := objects[0]
	var level uint32
	for _, obj := range objects {
		if r.meta.Shard != obj.meta.Shard {
			return nil, ErrShardMergeMismatch
		}
		level = max(level, obj.meta.CompactionLevel)
	}
	level++

	g := NewULIDGenerator(objects)
	m := make(map[string]*CompactionPlan)
	for _, obj := range objects {
		for _, s := range obj.meta.Datasets {
			tm, ok := m[obj.meta.StringTable[s.Tenant]]
			if !ok {
				tm = newBlockCompaction(
					g.ULID().String(),
					obj.meta.StringTable[s.Tenant],
					r.meta.Shard,
					level,
				)
				m[obj.meta.StringTable[s.Tenant]] = tm
			}
			// Bind objects to datasets.
			sm := tm.addDataset(obj.meta, s)
			sm.append(NewDataset(s, obj))
		}
	}

	ordered := make([]*CompactionPlan, 0, len(m))
	for _, tm := range m {
		ordered = append(ordered, tm)
		slices.SortFunc(tm.datasets, func(a, b *datasetCompaction) int {
			return strings.Compare(a.name, b.name)
		})
	}
	slices.SortFunc(ordered, func(a, b *CompactionPlan) int {
		return strings.Compare(a.tenant, b.tenant)
	})

	return ordered, nil
}

type CompactionPlan struct {
	tenant       string
	path         string
	datasetMap   map[int32]*datasetCompaction
	datasets     []*datasetCompaction
	meta         *metastorev1.BlockMeta
	strings      *metadata.StringTable
	datasetIndex *datasetIndexWriter
}

func newBlockCompaction(
	id string,
	tenant string,
	shard uint32,
	compactionLevel uint32,
) *CompactionPlan {
	p := &CompactionPlan{
		tenant:       tenant,
		datasetMap:   make(map[int32]*datasetCompaction),
		strings:      metadata.NewStringTable(),
		datasetIndex: newDatasetIndexWriter(),
	}
	p.path = BuildObjectPath(tenant, shard, compactionLevel, id)
	p.meta = &metastorev1.BlockMeta{
		FormatVersion:   1,
		Id:              id,
		Tenant:          p.strings.Put(tenant),
		Shard:           shard,
		CompactionLevel: compactionLevel,
	}
	return p
}

func (b *CompactionPlan) Compact(ctx context.Context, dst objstore.Bucket, tmpdir string) (m *metastorev1.BlockMeta, err error) {
	w, err := NewBlockWriter(dst, b.path, tmpdir)
	if err != nil {
		return nil, fmt.Errorf("block writer: %w", err)
	}
	defer func() {
		err = multierror.New(err, w.Close()).Err()
	}()
	// Datasets are compacted in a strict order.
	for i, s := range b.datasets {
		b.datasetIndex.resetDatasetIndex(uint32(i))
		if err = s.compact(ctx, w); err != nil {
			return nil, fmt.Errorf("compacting block: %w", err)
		}
		b.meta.Datasets = append(b.meta.Datasets, s.meta)
	}
	if err = b.writeDatasetIndex(w); err != nil {
		return nil, fmt.Errorf("writing tenant index: %w", err)
	}
	b.meta.StringTable = b.strings.Strings
	b.meta.MetadataOffset = w.Offset()
	if err = metadata.Encode(w, b.meta); err != nil {
		return nil, fmt.Errorf("writing metadata: %w", err)
	}
	b.meta.Size = w.Offset()
	if err = w.Upload(ctx); err != nil {
		return nil, fmt.Errorf("flushing block writer: %w", err)
	}
	return b.meta, nil
}

func (b *CompactionPlan) writeDatasetIndex(w *Writer) error {
	if err := b.datasetIndex.Flush(); err != nil {
		return err
	}
	off := w.Offset()
	n, err := w.ReadFrom(bytes.NewReader(b.datasetIndex.buf))
	if err != nil {
		return err
	}
	labels := metadata.NewLabelBuilder(b.strings).BuildPairs(
		metadata.LabelNameTenantDataset,
		metadata.LabelValueDatasetTSDBIndex,
	)
	b.meta.Datasets = append(b.meta.Datasets, &metastorev1.Dataset{
		Tenant:  b.meta.Tenant,
		Name:    0, // Anonymous.
		MinTime: b.meta.MinTime,
		MaxTime: b.meta.MaxTime,
		// FIXME: We mimic the default layout: empty profiles, index, and empty symbols.
		//  Instead, it should be handled at the query time: substitute the dataset layout.
		TableOfContents: []uint64{off, off, w.Offset()},
		Size:            uint64(n),
		Labels:          labels,
	})
	return nil
}

func (b *CompactionPlan) addDataset(md *metastorev1.BlockMeta, s *metastorev1.Dataset) *datasetCompaction {
	name := b.strings.Put(md.StringTable[s.Name])
	tenant := b.strings.Put(md.StringTable[s.Tenant])
	sm, ok := b.datasetMap[name]
	if !ok {
		sm = b.newDatasetCompaction(tenant, name)
		b.datasetMap[name] = sm
		b.datasets = append(b.datasets, sm)
	}
	if b.meta.MinTime == 0 || s.MinTime < b.meta.MinTime {
		b.meta.MinTime = s.MinTime
	}
	if s.MaxTime > b.meta.MaxTime {
		b.meta.MaxTime = s.MaxTime
	}
	return sm
}

type datasetCompaction struct {
	// Dataset name.
	name   string
	parent *CompactionPlan
	meta   *metastorev1.Dataset
	labels *metadata.LabelBuilder
	path   string // Set at open.

	datasets []*Dataset

	indexRewriter   *indexRewriter
	symbolsRewriter *symbolsRewriter
	profilesWriter  *profilesWriter

	samples  uint64
	series   uint64
	profiles uint64

	flushOnce sync.Once
}

func (b *CompactionPlan) newDatasetCompaction(tenant, name int32) *datasetCompaction {
	return &datasetCompaction{
		parent: b,
		name:   b.strings.Strings[name],
		labels: metadata.NewLabelBuilder(b.strings),
		meta: &metastorev1.Dataset{
			Tenant: tenant,
			Name:   name,
			// Updated at append.
			MinTime: 0,
			MaxTime: 0,
			// Updated at writeTo.
			TableOfContents: nil,
			Size:            0,
			Labels:          nil,
		},
	}
}

func (m *datasetCompaction) append(s *Dataset) {
	m.datasets = append(m.datasets, s)
	if m.meta.MinTime == 0 || s.meta.MinTime < m.meta.MinTime {
		m.meta.MinTime = s.meta.MinTime
	}
	if s.meta.MaxTime > m.meta.MaxTime {
		m.meta.MaxTime = s.meta.MaxTime
	}
	m.labels.Put(s.meta.Labels, s.obj.meta.StringTable)
}

func (m *datasetCompaction) compact(ctx context.Context, w *Writer) (err error) {
	if err = m.open(ctx, w.Dir()); err != nil {
		return fmt.Errorf("failed to open sections for compaction: %w", err)
	}
	defer func() {
		err = multierror.New(err, m.cleanup()).Err()
	}()
	if err = m.mergeAndClose(ctx); err != nil {
		return fmt.Errorf("failed to merge datasets: %w", err)
	}
	if err = m.writeTo(w); err != nil {
		return fmt.Errorf("failed to write sections: %w", err)
	}
	return nil
}

func (m *datasetCompaction) open(ctx context.Context, path string) (err error) {
	m.path = path
	defer func() {
		if err != nil {
			err = multierror.New(err, m.cleanup()).Err()
		}
	}()

	if err = os.MkdirAll(m.path, 0o777); err != nil {
		return err
	}

	var estimatedProfileTableSize int64
	for _, ds := range m.datasets {
		estimatedProfileTableSize += ds.sectionSize(SectionProfiles)
	}
	pageBufferSize := estimatePageBufferSize(estimatedProfileTableSize)
	m.profilesWriter, err = newProfileWriter(m.path, pageBufferSize)
	if err != nil {
		return err
	}

	m.indexRewriter = newIndexRewriter()
	m.symbolsRewriter = newSymbolsRewriter()

	g, ctx := errgroup.WithContext(ctx)
	for _, s := range m.datasets {
		s := s
		g.Go(util.RecoverPanic(func() error {
			if openErr := s.Open(ctx, allSections...); openErr != nil {
				return fmt.Errorf("opening tenant dataset (block %s): %w", s.obj.path, openErr)
			}
			return nil
		}))
	}
	if err = g.Wait(); err != nil {
		merr := multierror.New(err)
		for _, s := range m.datasets {
			merr.Add(s.Close())
		}
		return merr.Err()
	}

	return nil
}

func (m *datasetCompaction) mergeAndClose(ctx context.Context) (err error) {
	defer func() {
		err = multierror.New(err, m.close()).Err()
	}()
	return m.merge(ctx)
}

func (m *datasetCompaction) merge(ctx context.Context) (err error) {
	rows, err := NewMergeRowProfileIterator(m.datasets)
	if err != nil {
		return err
	}
	defer func() {
		err = multierror.New(err, rows.Close()).Err()
	}()
	var i int
	for rows.Next() {
		if i++; i%1000 == 0 {
			if err = ctx.Err(); err != nil {
				return err
			}
		}
		if err = m.writeRow(rows.At()); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (m *datasetCompaction) writeRow(r ProfileEntry) (err error) {
	if err = m.parent.datasetIndex.writeRow(r); err != nil {
		return err
	}
	if err = m.indexRewriter.rewriteRow(r); err != nil {
		return err
	}
	if err = m.symbolsRewriter.rewriteRow(r); err != nil {
		return err
	}
	return m.profilesWriter.writeRow(r)
}

func (m *datasetCompaction) close() (err error) {
	m.flushOnce.Do(func() {
		merr := multierror.New()
		merr.Add(m.symbolsRewriter.Flush())
		merr.Add(m.indexRewriter.Flush())
		merr.Add(m.profilesWriter.Close())
		m.samples = m.symbolsRewriter.samples
		m.series = m.indexRewriter.NumSeries()
		m.profiles = m.profilesWriter.profiles
		err = merr.Err()
	})
	return err
}

func (m *datasetCompaction) writeTo(w *Writer) error {
	off := w.Offset()
	m.meta.TableOfContents = make([]uint64, 0, 3)
	m.meta.TableOfContents = append(m.meta.TableOfContents, w.Offset())
	if err := w.ReadFromFile(FileNameProfilesParquet); err != nil {
		return err
	}
	m.meta.TableOfContents = append(m.meta.TableOfContents, w.Offset())
	if _, err := w.ReadFrom(bytes.NewReader(m.indexRewriter.buf)); err != nil {
		return err
	}
	m.meta.TableOfContents = append(m.meta.TableOfContents, w.Offset())
	if _, err := w.ReadFrom(bytes.NewReader(m.symbolsRewriter.buf.Bytes())); err != nil {
		return err
	}
	m.meta.Size = w.Offset() - off
	m.meta.Labels = m.labels.Build()
	return nil
}

func (m *datasetCompaction) cleanup() error {
	return os.RemoveAll(m.path)
}

func newIndexRewriter() *indexRewriter {
	return &indexRewriter{
		symbols: make(map[string]struct{}),
	}
}

type indexRewriter struct {
	series     []seriesLabels
	symbols    map[string]struct{}
	chunks     []index.ChunkMeta // one chunk per series
	previousFp model.Fingerprint
	buf        []byte
}

type seriesLabels struct {
	labels      phlaremodel.Labels
	fingerprint model.Fingerprint
}

func (rw *indexRewriter) rewriteRow(e ProfileEntry) error {
	if rw.previousFp != e.Fingerprint || len(rw.series) == 0 {
		series := e.Labels.Clone()
		for _, l := range series {
			rw.symbols[l.Name] = struct{}{}
			rw.symbols[l.Value] = struct{}{}
		}
		rw.series = append(rw.series, seriesLabels{
			labels:      series,
			fingerprint: e.Fingerprint,
		})
		rw.chunks = append(rw.chunks, index.ChunkMeta{
			MinTime:     e.Timestamp,
			MaxTime:     e.Timestamp,
			SeriesIndex: uint32(len(rw.series) - 1),
		})
		rw.previousFp = e.Fingerprint
	}
	rw.chunks[len(rw.chunks)-1].MaxTime = e.Timestamp
	e.Row.SetSeriesIndex(rw.chunks[len(rw.chunks)-1].SeriesIndex)
	return nil
}

func (rw *indexRewriter) NumSeries() uint64 { return uint64(len(rw.series)) }

func (rw *indexRewriter) Flush() error {
	// TODO(kolesnikovae):
	//  * Estimate size.
	//  * Use buffer pool.
	w, err := memindex.NewWriter(context.Background(), 256<<10)
	if err != nil {
		return err
	}

	// Sort symbols
	symbols := make([]string, 0, len(rw.symbols))
	for s := range rw.symbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// Add symbols
	for _, symbol := range symbols {
		if err = w.AddSymbol(symbol); err != nil {
			return err
		}
	}

	// Add Series
	for i, series := range rw.series {
		if err = w.AddSeries(storage.SeriesRef(i), series.labels, series.fingerprint, rw.chunks[i]); err != nil {
			return err
		}
	}

	err = w.Close()
	rw.buf = w.ReleaseIndex()
	return err
}

type symbolsRewriter struct {
	buf     *bytes.Buffer
	w       *symdb.SymDB
	rw      map[*Dataset]*symdb.Rewriter
	samples uint64

	stacktraces []uint32
}

func newSymbolsRewriter() *symbolsRewriter {
	// TODO(kolesnikovae):
	//  * Estimate size.
	//  * Use buffer pool.
	buf := bytes.NewBuffer(make([]byte, 0, 1<<20))
	return &symbolsRewriter{
		buf: buf,
		rw:  make(map[*Dataset]*symdb.Rewriter),
		w: symdb.NewSymDB(&symdb.Config{
			Version:       symdb.FormatV3,
			Writer:        buf,
			NoStatsUpdate: true,
		}),
	}
}

func (s *symbolsRewriter) rewriteRow(e ProfileEntry) (err error) {
	rw := s.rewriterFor(e.Dataset)
	e.Row.ForStacktraceIDsValues(func(values []parquet.Value) {
		s.loadStacktraceIDs(values)
		if err = rw.Rewrite(e.Row.StacktracePartitionID(), s.stacktraces); err != nil {
			return
		}
		s.samples += uint64(len(values))
		for i, v := range values {
			values[i] = parquet.Int64Value(int64(s.stacktraces[i])).Level(v.RepetitionLevel(), v.DefinitionLevel(), v.Column())
		}
	})
	return err
}

func (s *symbolsRewriter) rewriterFor(x *Dataset) *symdb.Rewriter {
	rw, ok := s.rw[x]
	if !ok {
		rw = symdb.NewRewriter(s.w, x.Symbols())
		s.rw[x] = rw
	}
	return rw
}

func (s *symbolsRewriter) loadStacktraceIDs(values []parquet.Value) {
	s.stacktraces = slices.Grow(s.stacktraces[0:], len(values))[:len(values)]
	for i := range values {
		s.stacktraces[i] = values[i].Uint32()
	}
}

func (s *symbolsRewriter) Flush() error { return s.w.Flush() }

// datasetIndexWriter is identical with indexRewriter,
// except it writes dataset ID instead of series ID.
type datasetIndexWriter struct {
	series   []seriesLabels
	chunks   []index.ChunkMeta
	previous model.Fingerprint
	symbols  map[string]struct{}
	idx      uint32
	buf      []byte
}

func newDatasetIndexWriter() *datasetIndexWriter {
	return &datasetIndexWriter{
		symbols: make(map[string]struct{}),
	}
}

func (rw *datasetIndexWriter) resetDatasetIndex(i uint32) { rw.idx = i }

func (rw *datasetIndexWriter) writeRow(e ProfileEntry) error {
	if rw.previous != e.Fingerprint || len(rw.series) == 0 {
		series := e.Labels.Clone()
		for _, l := range series {
			rw.symbols[l.Name] = struct{}{}
			rw.symbols[l.Value] = struct{}{}
		}
		rw.series = append(rw.series, seriesLabels{
			labels:      series,
			fingerprint: e.Fingerprint,
		})
		rw.chunks = append(rw.chunks, index.ChunkMeta{
			SeriesIndex: rw.idx,
		})
		rw.previous = e.Fingerprint
	}
	return nil
}

func (rw *datasetIndexWriter) Flush() error {
	// TODO(kolesnikovae):
	//  * Estimate size.
	//  * Use buffer pool.
	w, err := memindex.NewWriter(context.Background(), 1<<20)
	if err != nil {
		return err
	}

	// Sort symbols
	symbols := make([]string, 0, len(rw.symbols))
	for s := range rw.symbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// Add symbols
	for _, symbol := range symbols {
		if err = w.AddSymbol(symbol); err != nil {
			return err
		}
	}

	// Add Series
	for i, series := range rw.series {
		if err = w.AddSeries(storage.SeriesRef(i), series.labels, series.fingerprint, rw.chunks[i]); err != nil {
			return err
		}
	}

	err = w.Close()
	rw.buf = w.ReleaseIndex()
	return err
}
