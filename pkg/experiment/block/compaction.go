package block

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

func WithSampleObserver(observer SampleObserver) CompactionOption {
	return func(p *compactionConfig) {
		p.sampleObserver = observer
	}
}

type compactionConfig struct {
	objectOptions  []ObjectOption
	source         objstore.BucketReader
	destination    objstore.Bucket
	tempdir        string
	sampleObserver SampleObserver
}

type SampleObserver interface {
	symdb.SymbolsObserver

	// Observe is called before the compactor appends the entry
	// to the output block. This method must not modify the entry.
	Observe(ProfileEntry)
}

func Compact(
	ctx context.Context,
	blocks []*metastorev1.BlockMeta,
	storage objstore.Bucket,
	options ...CompactionOption,
) (m []*metastorev1.BlockMeta, err error) {
	c := &compactionConfig{
		source:      storage,
		destination: storage,
		tempdir:     os.TempDir(),
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
		// per cada tenant
		md, compactionErr := p.Compact(ctx, c.destination, c.tempdir, c.sampleObserver)
		if compactionErr != nil {
			return nil, compactionErr
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
		for _, ds := range obj.meta.Datasets {
			if ds.Name == 0 {
				// Anonymous dataset is never compacted:
				// it is rebuilt based on the actual block contents.
				continue
			}
			tm, ok := m[obj.meta.StringTable[ds.Tenant]]
			if !ok {
				tm = newBlockCompaction(
					g.ULID().String(),
					obj.meta.StringTable[ds.Tenant],
					r.meta.Shard,
					level,
				)
				m[obj.meta.StringTable[ds.Tenant]] = tm
			}
			// Bind objects to datasets.
			sm := tm.addDataset(obj.meta, ds)
			sm.append(NewDataset(ds, obj))
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

func (b *CompactionPlan) Compact(
	ctx context.Context,
	dst objstore.Bucket,
	tempdir string,
	observer SampleObserver,
) (m *metastorev1.BlockMeta, err error) {
	w, err := NewBlockWriter(tempdir)
	if err != nil {
		return nil, fmt.Errorf("creating block writer: %w", err)
	}
	defer func() {
		_ = w.Close()
	}()

	// Datasets are compacted in a strict order.
	for i, s := range b.datasets {
		// per cada service_name
		b.datasetIndex.setIndex(uint32(i))
		s.registerSampleObserver(observer)
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
	if err = w.Upload(ctx, dst, b.path); err != nil {
		return nil, fmt.Errorf("uploading block: %w", err)
	}
	return b.meta, nil
}

func (b *CompactionPlan) writeDatasetIndex(w *Writer) error {
	if err := b.datasetIndex.Flush(); err != nil {
		return err
	}
	off := w.Offset()
	n, err := io.Copy(w, bytes.NewReader(b.datasetIndex.buf))
	if err != nil {
		return err
	}
	// We annotate the dataset with the
	// __tenant_dataset__ = "dataset_tsdb_index" label,
	// so the dataset index metadata can be queried.
	labels := metadata.NewLabelBuilder(b.strings).
		WithLabelSet(metadata.LabelNameTenantDataset, metadata.LabelValueDatasetTSDBIndex).
		Build()
	b.meta.Datasets = append(b.meta.Datasets, &metastorev1.Dataset{
		Format:          1,
		Tenant:          b.meta.Tenant,
		Name:            0, // Anonymous.
		MinTime:         b.meta.MinTime,
		MaxTime:         b.meta.MaxTime,
		TableOfContents: []uint64{off},
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

	datasets []*Dataset

	indexRewriter   *indexRewriter
	symbolsRewriter *symbolsRewriter
	profilesWriter  *profilesWriter

	samples  uint64
	series   uint64
	profiles uint64

	flushOnce sync.Once

	observer SampleObserver
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
	off := w.Offset()
	m.meta.TableOfContents = make([]uint64, 0, 3)
	m.meta.TableOfContents = append(m.meta.TableOfContents, w.Offset())

	if err = m.open(ctx, w); err != nil {
		return fmt.Errorf("failed to open sections for compaction: %w", err)
	}
	defer func() {
		_ = m.close()
	}()

	if err = m.merge(ctx); err != nil {
		return fmt.Errorf("failed to merge datasets: %w", err)
	}
	if err = m.flush(); err != nil {
		return fmt.Errorf("failed to flush compacted dataset: %w", err)
	}

	m.meta.TableOfContents = append(m.meta.TableOfContents, w.Offset())
	if _, err = io.Copy(w, bytes.NewReader(m.indexRewriter.buf)); err != nil {
		return fmt.Errorf("failed to read index: %w", err)
	}
	m.meta.TableOfContents = append(m.meta.TableOfContents, w.Offset())
	if _, err = io.Copy(w, bytes.NewReader(m.symbolsRewriter.buf.Bytes())); err != nil {
		return fmt.Errorf("failed to read symbols: %w", err)
	}

	m.meta.Size = w.Offset() - off
	m.meta.Labels = m.labels.Build()
	return nil
}

func (m *datasetCompaction) registerSampleObserver(observer SampleObserver) {
	m.observer = observer
}

func (m *datasetCompaction) open(ctx context.Context, w io.Writer) (err error) {
	var estimatedProfileTableSize int64
	for _, ds := range m.datasets {
		estimatedProfileTableSize += ds.sectionSize(SectionProfiles)
	}
	pageBufferSize := estimatePageBufferSize(estimatedProfileTableSize)
	m.profilesWriter = newProfileWriter(pageBufferSize, w)

	m.indexRewriter = newIndexRewriter()
	m.symbolsRewriter = newSymbolsRewriter(m.observer)

	g, ctx := errgroup.WithContext(ctx)
	for _, s := range m.datasets {
		s := s
		g.Go(util.RecoverPanic(func() error {
			if openErr := s.Open(ctx,
				SectionProfiles,
				SectionTSDB,
				SectionSymbols,
			); openErr != nil {
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
	/*println(r.Dataset.tenant, r.Dataset.name, r.Dataset.obj.path, r.Fingerprint)
	reader, _ := r.Dataset.Symbols().Partition(context.Background(), r.Row.StacktracePartitionID())
	symbols := reader.Symbols()
	println(symbols)*/
	if err = m.parent.datasetIndex.writeRow(r); err != nil {
		return err
	}
	if err = m.indexRewriter.rewriteRow(r); err != nil {
		return err
	}
	if err = m.symbolsRewriter.rewriteRow(r); err != nil {
		return err
	}
	/*reader, _ = r.Dataset.Symbols().Partition(context.Background(), r.Row.StacktracePartitionID())
	symbols = reader.Symbols()*/
	if m.observer != nil {
		m.observer.Observe(r)
	}
	return m.profilesWriter.writeRow(r)
}

func (m *datasetCompaction) flush() (err error) {
	m.flushOnce.Do(func() {
		merr := multierror.New()
		merr.Add(m.symbolsRewriter.Flush())
		merr.Add(m.indexRewriter.Flush())
		merr.Add(m.profilesWriter.Close())
		m.samples = m.symbolsRewriter.samples
		m.series = m.indexRewriter.NumSeries()
		m.profiles = m.profilesWriter.profiles
		err = merr.Err()
		if m.observer != nil {
			m.observer.FlushSymbols()
		}
	})
	return err
}

func (m *datasetCompaction) close() error {
	err := m.flush()
	m.symbolsRewriter = nil
	m.indexRewriter = nil
	m.profilesWriter = nil
	m.datasets = nil
	return err
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
	buf      *bytes.Buffer
	w        *symdb.SymDB
	rw       map[*Dataset]*symdb.Rewriter
	samples  uint64
	observer SampleObserver

	stacktraces []uint32
}

func newSymbolsRewriter(observer SampleObserver) *symbolsRewriter {
	// TODO(kolesnikovae):
	//  * Estimate size.
	//  * Use buffer pool.
	buf := bytes.NewBuffer(make([]byte, 0, 1<<20))
	return &symbolsRewriter{
		buf: buf,
		rw:  make(map[*Dataset]*symdb.Rewriter),
		w: symdb.NewSymDB(&symdb.Config{
			Version: symdb.FormatV3,
			Writer:  &nopWriteCloser{buf},
		}),
		observer: observer,
	}
}

type nopWriteCloser struct{ io.Writer }

func (*nopWriteCloser) Close() error { return nil }

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
		rw = symdb.NewRewriter(s.w, x.Symbols(), s.observer)
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

func (rw *datasetIndexWriter) setIndex(i uint32) { rw.idx = i }

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
