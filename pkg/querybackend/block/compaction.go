package block

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/grafana/dskit/multierror"
	"github.com/oklog/ulid"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util"
)

var (
	ErrNoBlocksToMerge    = fmt.Errorf("no blocks to merge")
	ErrShardMergeMismatch = fmt.Errorf("only blocks from the same shard can be merged")
)

func Compact(
	ctx context.Context,
	blocks []*metastorev1.BlockMeta,
	storage objstore.Bucket,
) (m []*metastorev1.BlockMeta, err error) {
	objects := ObjectsFromMetas(storage, blocks)
	plan, err := PlanCompaction(objects)
	if err != nil {
		return nil, err
	}

	if err = objects.Open(ctx); err != nil {
		return nil, err
	}
	defer func() {
		err = objects.Close()
	}()

	compacted := make([]*metastorev1.BlockMeta, len(plan))
	for i, p := range plan {
		compacted[i], err = p.Compact(ctx, storage)
		if err != nil {
			return compacted, err
		}
	}

	return compacted, nil
}

// ObjectsFromMetas binds block metas to corresponding objects in the storage.
func ObjectsFromMetas(storage objstore.Bucket, blocks []*metastorev1.BlockMeta) Objects {
	objects := make([]*Object, len(blocks))
	for i, m := range blocks {
		objects[i] = NewObject(storage, m)
	}
	return objects
}

func PlanCompaction(objects Objects) ([]*CompactionPlan, error) {
	if len(objects) == 0 {
		// Even if there's just a single object, we still need to rewrite it.
		return nil, ErrNoBlocksToMerge
	}

	r := objects[0]
	var c uint32
	for _, obj := range objects {
		if r.meta.Shard != obj.meta.Shard {
			return nil, ErrShardMergeMismatch
		}
		c = max(c, obj.meta.CompactionLevel)
	}
	c++

	m := make(map[string]*CompactionPlan)
	for _, obj := range objects {
		for _, s := range obj.meta.TenantServices {
			tm, ok := m[s.TenantId]
			if !ok {
				tm = newBlockCompaction(s.TenantId, r.meta.Shard, c)
				m[s.TenantId] = tm
			}
			sm := tm.addTenantService(s)
			// Bind objects to services.
			sm.append(NewTenantService(s, obj))
		}
	}

	ordered := make([]*CompactionPlan, 0, len(m))
	for _, tm := range m {
		ordered = append(ordered, tm)
		slices.SortFunc(tm.services, func(a, b *tenantServiceCompaction) int {
			return strings.Compare(a.meta.Name, b.meta.Name)
		})
	}
	slices.SortFunc(ordered, func(a, b *CompactionPlan) int {
		return strings.Compare(a.tenantID, b.tenantID)
	})

	return ordered, nil
}

type CompactionPlan struct {
	tenantID   string
	serviceMap map[string]*tenantServiceCompaction
	services   []*tenantServiceCompaction
	meta       *metastorev1.BlockMeta
}

func newBlockCompaction(tenantID string, shard uint32, compactionLevel uint32) *CompactionPlan {
	return &CompactionPlan{
		tenantID:   tenantID,
		serviceMap: make(map[string]*tenantServiceCompaction),
		meta: &metastorev1.BlockMeta{
			FormatVersion:   1,
			Id:              ulid.MustNew(uint64(time.Now().UnixMilli()), rand.Reader).String(),
			TenantId:        tenantID,
			Shard:           shard,
			CompactionLevel: compactionLevel,
			TenantServices:  nil,
			MinTime:         0,
			MaxTime:         0,
			Size:            0,
		},
	}
}

func (b *CompactionPlan) Estimate() {
	// TODO(kolesnikovae): Implement.
}

func (b *CompactionPlan) Compact(ctx context.Context, storage objstore.Bucket) (m *metastorev1.BlockMeta, err error) {
	dir := filepath.Join(os.TempDir(), "pyroscope-compactor", b.meta.Id)
	w := NewBlockWriter(ctx, storage, ObjectPath(b.meta), dir)
	defer func() {
		err = multierror.New(err, w.Close()).Err()
	}()
	// Services are compacted in strict order.
	for _, s := range b.services {
		s.estimate()
		// TODO(kolesnikovae): Wait until the required resources are available?
		if err = s.compact(ctx, w); err != nil {
			return nil, fmt.Errorf("compacting block: %w", err)
		}
		b.meta.TenantServices = append(b.meta.TenantServices, s.meta)
	}
	b.meta.Size = w.Offset()
	return b.meta, nil
}

func (b *CompactionPlan) addTenantService(s *metastorev1.TenantService) *tenantServiceCompaction {
	sm, ok := b.serviceMap[s.Name]
	if !ok {
		sm = newTenantServiceCompaction(s.TenantId, s.Name)
		b.serviceMap[s.Name] = sm
		b.services = append(b.services, sm)
	}
	if b.meta.MinTime == 0 || s.MinTime < b.meta.MinTime {
		b.meta.MinTime = s.MinTime
	}
	if s.MaxTime > b.meta.MaxTime {
		b.meta.MaxTime = s.MaxTime
	}
	return sm
}

type compactionEstimates struct {
	inMemorySizeInputSymbols  int64
	inMemorySizeInputIndex    int64
	inMemorySizeInputProfiles int64

	inMemorySizeOutputSymbols  int64
	inMemorySizeOutputIndex    int64
	inMemorySizeOutputProfiles int64

	outputSizeIndex    int64
	outputSizeSymbols  int64
	outputSizeProfiles int64
}

func (m *compactionEstimates) inMemorySizeTotal() int64 {
	return m.inMemorySizeInputSymbols +
		m.inMemorySizeInputIndex +
		m.inMemorySizeInputProfiles +
		m.inMemorySizeOutputSymbols +
		m.inMemorySizeOutputIndex +
		m.inMemorySizeOutputProfiles
}

type tenantServiceCompaction struct {
	meta   *metastorev1.TenantService
	ptypes map[string]struct{}
	path   string // Set at open.

	services []*TenantService

	indexRewriter   *indexRewriter
	symbolsRewriter *symbolsRewriter
	profilesWriter  *profilesWriter

	estimates compactionEstimates
	samples   uint64
	series    uint64
	profiles  uint64

	flushOnce sync.Once
}

func newTenantServiceCompaction(tenantID, name string) *tenantServiceCompaction {
	return &tenantServiceCompaction{
		ptypes: make(map[string]struct{}, 10),
		meta: &metastorev1.TenantService{
			TenantId: tenantID,
			Name:     name,
			// Updated at append.
			MinTime: 0,
			MaxTime: 0,
			// Updated at writeTo.
			TableOfContents: nil,
			Size:            0,
			ProfileTypes:    nil,
		},
	}
}

func (m *tenantServiceCompaction) append(s *TenantService) {
	m.services = append(m.services, s)
	if m.meta.MinTime == 0 || s.meta.MinTime < m.meta.MinTime {
		m.meta.MinTime = s.meta.MinTime
	}
	if s.meta.MaxTime > m.meta.MaxTime {
		m.meta.MaxTime = s.meta.MaxTime
	}
	for _, pt := range s.meta.ProfileTypes {
		m.ptypes[pt] = struct{}{}
	}
}

func (m *tenantServiceCompaction) compact(ctx context.Context, w *Writer) (err error) {
	if err = m.open(ctx, w.Dir()); err != nil {
		return fmt.Errorf("failed to open sections for compaction: %w", err)
	}
	defer func() {
		err = multierror.New(err, m.cleanup()).Err()
	}()
	if err = m.mergeAndClose(ctx); err != nil {
		return fmt.Errorf("failed to merge profiles: %w", err)
	}
	if err = m.writeTo(w); err != nil {
		return fmt.Errorf("failed to write sections: %w", err)
	}
	return nil
}

// TODO(kolesnikovae):
//   - Add statistics to the block meta.
//   - Measure. Ideally, we should track statistics.
func (m *tenantServiceCompaction) estimate() {
	columns := len(schemav1.ProfilesSchema.Columns())
	// Services are to be opened concurrently.
	for _, s := range m.services {
		s1 := s.sectionSize(SectionSymbols)
		// It's likely that both symbols and tsdb sections will
		// be heavily deduplicated, so the actual output size will
		// be smaller than we estimate – to be deduced later.
		m.estimates.outputSizeSymbols += s1
		// Both the symbols and the tsdb are loaded into memory entirely.
		// It's multiplied here according to experiments.
		// https://gist.github.com/kolesnikovae/6f7bdc0b8a14174a8e63485300144b4a
		m.estimates.inMemorySizeInputSymbols += s1 * 3 // Pessimistic estimate.

		s2 := s.sectionSize(SectionTSDB)
		m.estimates.outputSizeIndex += s2
		// TSDB index is loaded into memory entirely, but is not decoded.
		m.estimates.inMemorySizeInputIndex += int64(nextPowerOfTwo(uint32(s2)))

		s3 := s.sectionSize(SectionProfiles)
		m.estimates.outputSizeProfiles += s3
		// All columns are to be opened.
		// Assuming async read mode – 2 buffers per column:
		m.estimates.inMemorySizeInputProfiles += int64(2 * columns * estimateReadBufferSize(s3))
	}
	const symbolsDuplicationRatio = 0.5 // Two blocks are likely to have a half of symbols in common.
	m.estimates.outputSizeSymbols = int64(float64(m.estimates.outputSizeSymbols) * symbolsDuplicationRatio)
	// Duplication of series and profiles is ignored.

	// Output block memory footprint.
	m.estimates.inMemorySizeOutputIndex = m.estimates.outputSizeIndex * 8       // A guess. We keep all labels in memory.
	m.estimates.inMemorySizeOutputSymbols += m.estimates.outputSizeProfiles * 4 // Mind the lookup table of rewriter.
	// This is the most difficult part to estimate.
	// Parquet keeps ALL RG pages in memory. We have a limit of 10K rows per RG,
	// therefore it's very likely, that the whole table will be loaded into memory,
	// plus overhead of memory fragmentation. It's likely impossible to have a
	// reasonable estimate here.
	const rowSizeGuess = 2 << 10
	// Worst case should be appx ~32MB. If a doubled estimated output size is less than that, use it.
	columnBuffers := int64(nextPowerOfTwo(maxRowsPerRowGroup * rowSizeGuess))
	if s := 2 * m.estimates.outputSizeProfiles; s < columnBuffers {
		columnBuffers = s
	}
	pageBuffers := int64(columns * estimatePageBufferSize(m.estimates.outputSizeProfiles))
	m.estimates.inMemorySizeOutputProfiles += columnBuffers + pageBuffers
}

func (m *tenantServiceCompaction) open(ctx context.Context, path string) (err error) {
	m.path = path
	defer func() {
		if err != nil {
			err = multierror.New(err, m.cleanup()).Err()
		}
	}()

	if err = os.MkdirAll(m.path, 0o777); err != nil {
		return err
	}

	m.profilesWriter, err = newProfileWriter(m.path, m.estimates.outputSizeProfiles)
	if err != nil {
		return err
	}

	m.indexRewriter = newIndexRewriter(m.path)
	m.symbolsRewriter = newSymbolsRewriter(m.path)

	g, ctx := errgroup.WithContext(ctx)
	for _, s := range m.services {
		s := s
		g.Go(util.RecoverPanic(func() error {
			if err = s.Open(ctx, allSections...); err != nil {
				return fmt.Errorf("opening tenant service (block %s): %w", s.obj.path, err)
			}
			return nil
		}))
	}
	if err = g.Wait(); err != nil {
		merr := multierror.New(err)
		for _, s := range m.services {
			merr.Add(s.Close())
		}
		return merr.Err()
	}

	return nil
}

func (m *tenantServiceCompaction) mergeAndClose(ctx context.Context) (err error) {
	defer func() {
		err = multierror.New(err, m.close()).Err()
	}()
	return m.merge(ctx)
}

func (m *tenantServiceCompaction) merge(ctx context.Context) (err error) {
	rows, err := NewMergeRowProfileIterator(m.services)
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

func (m *tenantServiceCompaction) writeRow(r ProfileEntry) (err error) {
	if err = m.indexRewriter.rewriteRow(r); err != nil {
		return err
	}
	if err = m.symbolsRewriter.rewriteRow(r); err != nil {
		return err
	}
	return m.profilesWriter.writeRow(r)
}

func (m *tenantServiceCompaction) close() (err error) {
	m.flushOnce.Do(func() {
		merr := multierror.New()
		merr.Add(m.symbolsRewriter.Flush())
		merr.Add(m.indexRewriter.Flush())
		merr.Add(m.profilesWriter.Close())
		m.samples = m.symbolsRewriter.samples
		m.series = m.indexRewriter.NumSeries()
		m.profiles = m.profilesWriter.profiles
		m.symbolsRewriter = nil
		m.indexRewriter = nil
		m.profilesWriter = nil
		// Note that m.services are closed by merge
		// iterator as they reach the end of the profile
		// table. We do it here again just in case.
		// TODO(kolesnikovae): Double check error handling.
		m.services = nil
		err = merr.Err()
	})
	return err
}

func (m *tenantServiceCompaction) writeTo(w *Writer) (err error) {
	off := w.Offset()
	m.meta.TableOfContents, err = w.ReadFromFiles(
		FileNameProfilesParquet,
		block.IndexFilename,
		symdb.DefaultFileName,
	)
	if err != nil {
		return err
	}
	m.meta.Size = w.Offset() - off
	m.meta.ProfileTypes = make([]string, 0, len(m.ptypes))
	for pt := range m.ptypes {
		m.meta.ProfileTypes = append(m.meta.ProfileTypes, pt)
	}
	sort.Strings(m.meta.ProfileTypes)
	return nil
}

func (m *tenantServiceCompaction) cleanup() error {
	return os.RemoveAll(m.path)
}

func newIndexRewriter(path string) *indexRewriter {
	return &indexRewriter{
		symbols: make(map[string]struct{}),
		path:    path,
	}
}

type indexRewriter struct {
	series     []seriesLabels
	symbols    map[string]struct{}
	chunks     []index.ChunkMeta // one chunk per series
	previousFp model.Fingerprint

	path string
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
	w, err := index.NewWriter(context.Background(),
		filepath.Join(rw.path, block.IndexFilename),
		// There is no particular reason to use a buffer (bufio.Writer)
		// larger than the default one when writing on disk
		4<<10)
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

	return w.Close()
}

type symbolsRewriter struct {
	w       *symdb.SymDB
	rw      map[*TenantService]*symdb.Rewriter
	samples uint64

	stacktraces []uint32
}

func newSymbolsRewriter(path string) *symbolsRewriter {
	return &symbolsRewriter{
		rw: make(map[*TenantService]*symdb.Rewriter),
		w: symdb.NewSymDB(symdb.DefaultConfig().
			WithVersion(symdb.FormatV3).
			WithDirectory(path)),
	}
}

func (s *symbolsRewriter) rewriteRow(e ProfileEntry) (err error) {
	rw := s.rewriterFor(e.TenantService)
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

func (s *symbolsRewriter) rewriterFor(x *TenantService) *symdb.Rewriter {
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
