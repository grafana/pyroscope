package block

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/runutil"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

var (
	ErrNoBlocksToMerge         = fmt.Errorf("no blocks to merge")
	ErrNoTenantServicesToMerge = fmt.Errorf("no tenant services to merge")
	ErrShardMergeMismatch      = fmt.Errorf("only blocks from the same shard can be merged")
	ErrLevelMergeMismatch      = fmt.Errorf("only blocks os the same compaction level can be merged")
)

func EstimateMergeMemoryFootprint(objects []*Object) (int64, error) {
	groups, err := tenantServiceMergeIterator(objects)
	if err != nil {
		return 0, err
	}
	var m int64
	for groups.Next() {
		g := groups.At()
		g.estimate()
		m = max(m, g.estimates.inMemorySizeTotal())
	}
	return m, nil
}

func tenantServiceMergeIterator(objects []*Object) (iter.Iterator[*tenantServiceMerge], error) {
	if len(objects) == 0 {
		// Even if there's just a single object, we still need to rewrite it.
		return nil, ErrNoBlocksToMerge
	}
	r := objects[0]
	for _, obj := range objects {
		if r.meta.Shard != obj.meta.Shard {
			return nil, ErrShardMergeMismatch
		}
		// This is not strictly necessary, but it's a good sanity check.
		if r.meta.CompactionLevel != obj.meta.CompactionLevel {
			return nil, ErrLevelMergeMismatch
		}
	}

	services := make(map[tenantServiceKey]*tenantServiceMerge)
	for _, obj := range objects {
		for _, s := range obj.meta.TenantServices {
			k := tenantServiceKey{tenantID: s.TenantId, name: s.Name}
			m, ok := services[k]
			if !ok {
				m = newTenantServiceMerge(k)
			}
			m.append(NewTenantService(s, obj))
		}
	}
	if len(services) == 0 {
		return nil, ErrNoTenantServicesToMerge
	}

	for _, group := range services {
		slices.SortFunc(group.services, func(a, b *TenantService) int {
			return strings.Compare(a.obj.path, b.obj.path)
		})
	}

	groups := make([]*tenantServiceMerge, 0, len(services))
	for _, g := range services {
		groups = append(groups, g)
	}
	slices.SortFunc(groups, func(a, b *tenantServiceMerge) int {
		return a.tenantServiceKey.compare(b.tenantServiceKey)
	})

	return iter.NewSliceIterator(groups), nil
}

type tenantServiceKey struct {
	tenantID string
	name     string
}

func (k tenantServiceKey) compare(x tenantServiceKey) int {
	if k.tenantID != x.tenantID {
		return strings.Compare(k.tenantID, x.tenantID)
	}
	return strings.Compare(k.name, x.name)
}

type tenantServiceMerge struct {
	tenantServiceKey
	log  log.Logger
	path string

	meta     *metastorev1.TenantService
	services []*TenantService
	ptypes   map[string]struct{}

	indexRewriter   *indexRewriter
	symbolsRewriter *symbolsRewriter
	profilesWriter  *profilesWriter

	estimates mergeEstimates
	samples   uint64
	series    uint64
	profiles  uint64
}

type mergeEstimates struct {
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

func (m *mergeEstimates) inMemorySizeTotal() int64 {
	return m.inMemorySizeInputSymbols +
		m.inMemorySizeInputIndex +
		m.inMemorySizeInputProfiles +
		m.inMemorySizeOutputSymbols +
		m.inMemorySizeOutputIndex +
		m.inMemorySizeOutputProfiles
}

func newTenantServiceMerge(k tenantServiceKey) *tenantServiceMerge {
	return &tenantServiceMerge{
		tenantServiceKey: k,
		meta: &metastorev1.TenantService{
			TenantId: k.tenantID,
			Name:     k.name,
		},
	}
}

func (m *tenantServiceMerge) append(s *TenantService) {
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

func (m *tenantServiceMerge) build() *metastorev1.TenantService {
	m.meta.ProfileTypes = make([]string, 0, len(m.ptypes))
	for pt := range m.ptypes {
		m.meta.ProfileTypes = append(m.meta.ProfileTypes, pt)
	}
	sort.Strings(m.meta.ProfileTypes)
	// TODO: Collect files, fill TOC, Size
	return m.meta
}

// TODO(kolesnikovae):
//   - Add statistics to the block meta.
//   - Measure. Ideally, we should track statistics.
func (m *tenantServiceMerge) estimate() {
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

func (m *tenantServiceMerge) open() (rows iter.Iterator[ProfileEntry], err error) {
	if err = os.MkdirAll(m.path, 0o777); err != nil {
		return nil, err
	}
	m.profilesWriter, err = newProfileWriter(m.path, m.estimates.outputSizeProfiles)
	if err != nil {
		return nil, err
	}
	m.indexRewriter = newIndexRewriter(m.path)
	m.symbolsRewriter = newSymbolsRewriter(m.path)
	return NewMergeRowProfileIterator(m.services)
}

func (m *tenantServiceMerge) merge(ctx context.Context, path string) (err error) {
	m.path = path
	m.estimate()
	rows, err := m.open()
	if err != nil {
		return err
	}
	defer runutil.CloseWithLogOnErr(m.log, rows, "closing rows iterator")
	var i int
	for rows.Next() {
		if i++; i%10000 == 0 {
			if err = ctx.Err(); err != nil {
				return err
			}
		}
		if err = m.writeRow(rows.At()); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return m.Close()
}

func (m *tenantServiceMerge) writeRow(r ProfileEntry) (err error) {
	if err = m.indexRewriter.rewriteRow(r); err != nil {
		return err
	}
	if err = m.symbolsRewriter.rewriteRow(r); err != nil {
		return err
	}
	return m.profilesWriter.writeRow(r)
}

func (m *tenantServiceMerge) Close() (err error) {
	if err = m.symbolsRewriter.Flush(); err != nil {
		return err
	}
	m.samples = m.symbolsRewriter.samples
	if err = m.indexRewriter.Flush(); err != nil {
		return err
	}
	m.series = m.indexRewriter.NumSeries()
	if err = m.profilesWriter.Close(); err != nil {
		return err
	}
	m.profiles = m.profilesWriter.profiles
	return nil
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
		index.SegmentsIndexWriterBufSize)
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
		if err := w.AddSymbol(symbol); err != nil {
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
			// FIXME: the original order is not preserved, which will affect encoding.
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
	s.stacktraces = slices.Grow(s.stacktraces[0:], len(values))
	for i := range values {
		s.stacktraces[i] = values[i].Uint32()
	}
}

func (s *symbolsRewriter) Flush() error { return s.w.Flush() }
