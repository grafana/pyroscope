package phlaredb

import (
	"context"
	"io/fs"
	"math"
	"os"
	"path/filepath"

	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/segmentio/parquet-go"

	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/loser"
)

type BlockReader interface {
	Meta() block.Meta
	Profiles() []parquet.RowGroup
	Index() IndexReader
	Symbols() SymbolsResolver
}

// TODO(kolesnikovae): Refactor to symdb.

// ProfileSymbols represents symbolic information associated with a profile.
type ProfileSymbols struct {
	StacktracePartition uint64
	StacktraceIDs       []uint32

	Stacktraces []*schemav1.Stacktrace
	Locations   []*schemav1.InMemoryLocation
	Mappings    []*schemav1.InMemoryMapping
	Functions   []*schemav1.InMemoryFunction
	Strings     []string
}

type SymbolsResolver interface {
	Stacktraces(iter.Iterator[uint32]) iter.Iterator[*schemav1.Stacktrace]
	Locations(iter.Iterator[uint32]) iter.Iterator[*schemav1.InMemoryLocation]
	Mappings(iter.Iterator[uint32]) iter.Iterator[*schemav1.InMemoryMapping]
	Functions(iter.Iterator[uint32]) iter.Iterator[*schemav1.InMemoryFunction]
	Strings(iter.Iterator[uint32]) iter.Iterator[string]
}

type SymbolsAppender interface {
	AppendStacktrace(*schemav1.Stacktrace) uint32
	AppendLocation(*schemav1.InMemoryLocation) uint32
	AppendMapping(*schemav1.InMemoryMapping) uint32
	AppendFunction(*schemav1.InMemoryFunction) uint32
	AppendString(string) uint32
}

func Compact(ctx context.Context, src []BlockReader, dst string) (meta block.Meta, err error) {
	srcMetas := make([]block.Meta, len(src))
	ulids := make([]string, len(src))

	for i, b := range src {
		srcMetas[i] = b.Meta()
		ulids[i] = b.Meta().ULID.String()
	}
	meta = compactMetas(srcMetas)
	blockPath := filepath.Join(dst, meta.ULID.String())
	indexPath := filepath.Join(blockPath, block.IndexFilename)
	profilePath := filepath.Join(blockPath, (&schemav1.ProfilePersister{}).Name()+block.ParquetSuffix)

	sp, ctx := opentracing.StartSpanFromContext(ctx, "Compact")
	defer func() {
		// todo: context propagation is not working through objstore
		// This is because the BlockReader has no context.
		sp.SetTag("src", ulids)
		sp.SetTag("block_id", meta.ULID.String())
		if err != nil {
			sp.SetTag("error", err)
		}
		sp.Finish()
	}()

	if len(src) <= 1 {
		return block.Meta{}, errors.New("not enough blocks to compact")
	}
	if err := os.MkdirAll(blockPath, 0o777); err != nil {
		return block.Meta{}, err
	}

	indexw, err := prepareIndexWriter(ctx, indexPath, src)
	if err != nil {
		return block.Meta{}, err
	}

	profileFile, err := os.OpenFile(profilePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return block.Meta{}, err
	}
	profileWriter := newProfileWriter(profileFile)
	symw, err := newSymbolsWriter(dst)
	if err != nil {
		return block.Meta{}, err
	}

	rowsIt, err := newMergeRowProfileIterator(src)
	if err != nil {
		return block.Meta{}, err
	}
	seriesRewriter := newSeriesRewriter(rowsIt, indexw)
	symRewriter := newSymbolsRewriter(seriesRewriter, src, symw)
	reader := phlareparquet.NewIteratorRowReader(newRowsIterator(symRewriter))

	total, _, err := phlareparquet.CopyAsRowGroups(profileWriter, reader, defaultParquetConfig.MaxBufferRowCount)
	if err != nil {
		return block.Meta{}, err
	}

	// flush the index file.
	if err := indexw.Close(); err != nil {
		return block.Meta{}, err
	}

	if err := profileWriter.Close(); err != nil {
		return block.Meta{}, err
	}

	metaFiles, err := metaFilesFromDir(blockPath)
	if err != nil {
		return block.Meta{}, err
	}
	meta.Files = metaFiles
	meta.Stats.NumProfiles = total
	meta.Stats.NumSeries = seriesRewriter.NumSeries()
	meta.Stats.NumSamples = symbolsRewriter.NumSamples()
	if _, err := meta.WriteToFile(util.Logger, blockPath); err != nil {
		return block.Meta{}, err
	}
	return meta, nil
}

// metaFilesFromDir returns a list of block files description from a directory.
func metaFilesFromDir(dir string) ([]block.File, error) {
	var files []block.File
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		var f block.File
		switch filepath.Ext(info.Name()) {
		case block.ParquetSuffix:
			f, err = parquetMetaFile(path, info.Size())
			if err != nil {
				return err
			}
		case filepath.Ext(block.IndexFilename):
			f, err = tsdbMetaFile(path)
			if err != nil {
				return err
			}
		}
		f.RelPath, err = filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		f.SizeBytes = uint64(info.Size())
		files = append(files, f)
		return nil
	})
	return files, err
}

func tsdbMetaFile(filePath string) (block.File, error) {
	idxReader, err := index.NewFileReader(filePath)
	if err != nil {
		return block.File{}, err
	}

	return idxReader.FileInfo(), nil
}

func parquetMetaFile(filePath string, size int64) (block.File, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return block.File{}, err
	}
	defer f.Close()

	pqFile, err := parquet.OpenFile(f, size)
	if err != nil {
		return block.File{}, err
	}
	return block.File{
		Parquet: &block.ParquetFile{
			NumRowGroups: uint64(len(pqFile.RowGroups())),
			NumRows:      uint64(pqFile.NumRows()),
		},
	}, nil
}

// todo write tests
func compactMetas(src []block.Meta) block.Meta {
	meta := block.NewMeta()
	highestCompactionLevel := 0
	ulids := make([]ulid.ULID, len(src))
	parents := make([]tsdb.BlockDesc, len(src))
	minTime, maxTime := model.Latest, model.Earliest
	labels := make(map[string]string)
	for _, b := range src {
		if b.Compaction.Level > highestCompactionLevel {
			highestCompactionLevel = b.Compaction.Level
		}
		ulids = append(ulids, b.ULID)
		parents = append(parents, tsdb.BlockDesc{
			ULID:    b.ULID,
			MinTime: int64(b.MinTime),
			MaxTime: int64(b.MaxTime),
		})
		if b.MinTime < minTime {
			minTime = b.MinTime
		}
		if b.MaxTime > maxTime {
			maxTime = b.MaxTime
		}
		for k, v := range b.Labels {
			if k == block.HostnameLabel {
				continue
			}
			labels[k] = v
		}
	}
	if hostname, err := os.Hostname(); err == nil {
		labels[block.HostnameLabel] = hostname
	}
	meta.Source = block.CompactorSource
	meta.Compaction = tsdb.BlockMetaCompaction{
		Deletable: meta.Stats.NumSamples == 0,
		Level:     highestCompactionLevel + 1,
		Sources:   ulids,
		Parents:   parents,
	}
	meta.MaxTime = maxTime
	meta.MinTime = minTime
	meta.Labels = labels
	return *meta
}

type profileRow struct {
	timeNanos int64

	seriesRef uint32
	labels    phlaremodel.Labels
	fp        model.Fingerprint
	row       schemav1.ProfileRow

	blockReader BlockReader
}

type profileRowIterator struct {
	profiles    iter.Iterator[parquet.Row]
	blockReader BlockReader
	index       IndexReader
	allPostings index.Postings
	err         error

	currentRow profileRow
	chunks     []index.ChunkMeta
}

func newProfileRowIterator(reader parquet.RowReader, s BlockReader) (*profileRowIterator, error) {
	k, v := index.AllPostingsKey()
	allPostings, err := s.Index().Postings(k, nil, v)
	if err != nil {
		return nil, err
	}
	return &profileRowIterator{
		profiles:    phlareparquet.NewBufferedRowReaderIterator(reader, 1024),
		blockReader: s,
		index:       s.Index(),
		allPostings: allPostings,
		currentRow: profileRow{
			seriesRef: math.MaxUint32,
		},
		chunks: make([]index.ChunkMeta, 1),
	}, nil
}

func (p *profileRowIterator) At() profileRow {
	return p.currentRow
}

func (p *profileRowIterator) Next() bool {
	if !p.profiles.Next() {
		return false
	}
	p.currentRow.blockReader = p.blockReader
	p.currentRow.row = schemav1.ProfileRow(p.profiles.At())
	seriesIndex := p.currentRow.row.SeriesIndex()
	p.currentRow.timeNanos = p.currentRow.row.TimeNanos()
	// do we have a new series?
	if seriesIndex == p.currentRow.seriesRef {
		return true
	}
	p.currentRow.seriesRef = seriesIndex
	if !p.allPostings.Next() {
		if err := p.allPostings.Err(); err != nil {
			p.err = err
			return false
		}
		p.err = errors.New("unexpected end of postings")
		return false
	}

	fp, err := p.index.Series(p.allPostings.At(), &p.currentRow.labels, &p.chunks)
	if err != nil {
		p.err = err
		return false
	}
	p.currentRow.fp = model.Fingerprint(fp)
	return true
}

func (p *profileRowIterator) Err() error {
	if p.err != nil {
		return p.err
	}
	return p.profiles.Err()
}

func (p *profileRowIterator) Close() error {
	return p.profiles.Close()
}

func newMergeRowProfileIterator(src []BlockReader) (iter.Iterator[profileRow], error) {
	its := make([]iter.Iterator[profileRow], len(src))
	for i, s := range src {
		// todo: may be we could merge rowgroups in parallel but that requires locking.
		reader := parquet.MultiRowGroup(s.Profiles()...).Rows()
		it, err := newProfileRowIterator(reader, s)
		if err != nil {
			return nil, err
		}
		its[i] = it
	}
	return &dedupeProfileRowIterator{
		Iterator: iter.NewTreeIterator(loser.New(
			its,
			profileRow{
				timeNanos: math.MaxInt64,
			},
			func(it iter.Iterator[profileRow]) profileRow { return it.At() },
			func(r1, r2 profileRow) bool {
				// first handle max profileRow if it's either r1 or r2
				if r1.timeNanos == math.MaxInt64 {
					return false
				}
				if r2.timeNanos == math.MaxInt64 {
					return true
				}
				// then handle normal profileRows
				if cmp := phlaremodel.CompareLabelPairs(r1.labels, r2.labels); cmp != 0 {
					return cmp < 0
				}
				return r1.timeNanos < r2.timeNanos
			},
			func(it iter.Iterator[profileRow]) { _ = it.Close() },
		)),
	}, nil
}

type seriesRewriter struct {
	iter.Iterator[profileRow]

	indexw *index.Writer

	seriesRef        storage.SeriesRef
	labels           phlaremodel.Labels
	previousFp       model.Fingerprint
	currentChunkMeta index.ChunkMeta
	err              error

	numSeries uint64
}

func newSeriesRewriter(it iter.Iterator[profileRow], indexw *index.Writer) *seriesRewriter {
	return &seriesRewriter{
		Iterator: it,
		indexw:   indexw,
	}
}

func (s *seriesRewriter) NumSeries() uint64 {
	return s.numSeries
}

func (s *seriesRewriter) Next() bool {
	if !s.Iterator.Next() {
		if s.previousFp != 0 {
			if err := s.indexw.AddSeries(s.seriesRef, s.labels, s.previousFp, s.currentChunkMeta); err != nil {
				s.err = err
				return false
			}
			s.numSeries++
		}
		return false
	}
	currentProfile := s.Iterator.At()
	if s.previousFp != currentProfile.fp {
		// store the previous series.
		if s.previousFp != 0 {
			if err := s.indexw.AddSeries(s.seriesRef, s.labels, s.previousFp, s.currentChunkMeta); err != nil {
				s.err = err
				return false
			}
			s.numSeries++
		}
		s.seriesRef++
		s.labels = currentProfile.labels.Clone()
		s.previousFp = currentProfile.fp
		s.currentChunkMeta.MinTime = currentProfile.timeNanos
	}
	s.currentChunkMeta.MaxTime = currentProfile.timeNanos
	currentProfile.row.SetSeriesIndex(uint32(s.seriesRef))
	return true
}

type rowsIterator struct {
	iter.Iterator[profileRow]
}

func newRowsIterator(it iter.Iterator[profileRow]) *rowsIterator {
	return &rowsIterator{
		Iterator: it,
	}
}

func (r *rowsIterator) At() parquet.Row {
	return parquet.Row(r.Iterator.At().row)
}

type dedupeProfileRowIterator struct {
	iter.Iterator[profileRow]

	prevFP        model.Fingerprint
	prevTimeNanos int64
}

func (it *dedupeProfileRowIterator) Next() bool {
	for {
		if !it.Iterator.Next() {
			return false
		}
		currentProfile := it.Iterator.At()
		if it.prevFP == currentProfile.fp && it.prevTimeNanos == currentProfile.timeNanos {
			// skip duplicate profile
			continue
		}
		it.prevFP = currentProfile.fp
		it.prevTimeNanos = currentProfile.timeNanos
		return true
	}
}

func prepareIndexWriter(ctx context.Context, path string, readers []BlockReader) (*index.Writer, error) {
	var symbols index.StringIter
	indexw, err := index.NewWriter(ctx, path)
	if err != nil {
		return nil, err
	}
	for i, r := range readers {
		if i == 0 {
			symbols = r.Index().Symbols()
		}
		symbols = tsdb.NewMergedStringIter(symbols, r.Index().Symbols())
	}

	for symbols.Next() {
		if err := indexw.AddSymbol(symbols.At()); err != nil {
			return nil, errors.Wrap(err, "add symbol")
		}
	}
	if symbols.Err() != nil {
		return nil, errors.Wrap(symbols.Err(), "next symbol")
	}

	return indexw, nil
}

type symbolsRewriter struct {
	profiles         iter.Iterator[profileRow]
	stacktraces, dst []uint32
	err              error

	rewriters map[BlockReader]*stacktraceRewriter

	numSamples uint64
}

func newSymbolsRewriter(it iter.Iterator[profileRow], blocks []BlockReader, a SymbolsAppender) *symbolsRewriter {
	sr := symbolsRewriter{
		profiles:  it,
		rewriters: make(map[BlockReader]*stacktraceRewriter, len(blocks)),
	}
	for _, b := range blocks {
		sr.rewriters[b] = newStacktraceRewriter()
	}
	return &sr
}

func (s *symbolsRewriter) NumSamples() uint64 { return s.numSamples }

func (s *symbolsRewriter) At() profileRow { return s.profiles.At() }

func (s *symbolsRewriter) Close() error { return s.profiles.Close() }

func (s *symbolsRewriter) Err() error {
	if s.err != nil {
		return s.err
	}
	return s.profiles.Err()
}

func (s *symbolsRewriter) Next() bool {
	if !s.profiles.Next() {
		return false
	}
	var err error
	profile := s.profiles.At()
	profile.row.ForStacktraceIDsValues(func(values []parquet.Value) {
		s.loadStacktracesID(values)
		r := s.rewriters[profile.blockReader]
		if err = r.rewriteStacktraces(profile.row.StacktracePartitionID(), s.stacktraces); err != nil {
			return
		}
		s.numSamples += uint64(len(values))
		for i, v := range values {
			values[i] = parquet.Int64Value(int64(s.dst[i])).Level(v.RepetitionLevel(), v.DefinitionLevel(), v.Column())
		}
	})
	if err != nil {
		s.err = err
		return false
	}
	return true
}

func (s *symbolsRewriter) loadStacktracesID(values []parquet.Value) {
	if cap(s.stacktraces) < len(values) {
		s.stacktraces = make([]uint32, len(values)*2)
		s.dst = make([]uint32, len(values)*2)
	}
	s.stacktraces = s.stacktraces[:len(values)]
	s.dst = s.dst[:len(values)]
	for i := range values {
		s.stacktraces[i] = values[i].Uint32()
	}
}

type stacktraceRewriter struct {
	partition   uint64
	stacktraces map[uint64]*lookupTable[*schemav1.Stacktrace]

	locations *lookupTable[*schemav1.InMemoryLocation]
	mappings  *lookupTable[*schemav1.InMemoryMapping]
	functions *lookupTable[*schemav1.InMemoryFunction]
	strings   *lookupTable[string]
}

func newStacktraceRewriter() *stacktraceRewriter {
	// TODO(kolesnikovae):
	return new(stacktraceRewriter)
}

const (
	marker     = 1 << 31
	markedMask = math.MaxUint32 >> 1
)

type lookupTable[T any] struct {
	// Index is source ID, and the value is the destination ID.
	// If destination ID is not known, the element is index to 'unresolved' (marked).
	resolved []uint32
	// Source IDs.
	unresolved []uint32
	values     []T
}

func newLookupTable[T any](size int) *lookupTable[T] {
	var t lookupTable[T]
	t.init(size)
	return &t
}

func (t *lookupTable[T]) init(size int) {
	if cap(t.resolved) < size {
		t.resolved = make([]uint32, size)
		return
	}
	t.resolved = t.resolved[:size]
	for i := range t.resolved {
		t.resolved[i] = 0
	}
}

func (t *lookupTable[T]) reset() { t.unresolved = t.unresolved[:0] }

func (t *lookupTable[T]) tryLookup(x uint32) uint32 {
	if v := t.resolved[x]; v != 0 {
		return v - 1
	}
	v := uint32(len(t.unresolved)) | marker
	t.unresolved = append(t.unresolved, x)
	return v
}

func (t *lookupTable[T]) storeResolved(i, v uint32) { t.resolved[i] = v + 1 }

func (t *lookupTable[T]) lookupUnresolved(x uint32) uint32 {
	if x&marker == 0 {
		// Already resolved.
		return x
	}
	return t.unresolved[x&markedMask]
}

func (t *lookupTable[T]) iter() *lookupTableIterator[T] {
	t.values = make([]T, len(t.resolved))
	return &lookupTableIterator[T]{
		values: t.values,
	}
}

// TODO(kolesnikovae):
type lookupTableIterator[T any] struct {
	cur    uint32
	values []T
}

func (t *lookupTableIterator[T]) set(v T) { t.values[t.cur] = v }

func (r *stacktraceRewriter) symbolsResolver() SymbolsResolver {
	// TODO(kolesnikovae):
	return nil
}

func (r *stacktraceRewriter) symbolsAppender() SymbolsAppender {
	// TODO(kolesnikovae):
	return nil
}

func (r *stacktraceRewriter) reset(partition uint64) {
	r.partition = partition
	r.stacktraces[partition].reset()
	r.locations.reset()
	r.mappings.reset()
	r.functions.reset()
	r.strings.reset()
}

func (r *stacktraceRewriter) hasUnresolved() bool {
	return len(r.stacktraces[r.partition].unresolved)+
		len(r.locations.unresolved)+
		len(r.mappings.unresolved)+
		len(r.functions.unresolved)+
		len(r.strings.unresolved) > 0
}

func (r *stacktraceRewriter) rewriteStacktraces(partition uint64, stacktraces []uint32) error {
	r.reset(partition)
	r.populateUnresolved(stacktraces)
	if r.hasUnresolved() {
		r.append(stacktraces)
	}
	return nil
}

func (r *stacktraceRewriter) populateUnresolved(stacktraces []uint32) {
	// Filter out all stack traces that have been already resolved.
	src := r.stacktraces[r.partition]
	for i, v := range stacktraces {
		stacktraces[i] = src.tryLookup(v)
	}
	if len(src.unresolved) == 0 {
		return
	}

	// Resolve locations for new stack traces.
	var stacktrace *schemav1.Stacktrace
	unresolvedStacktraces := src.iter()
	p := r.symbolsResolver()
	for i := p.Stacktraces(unresolvedStacktraces); i.Next(); stacktrace = i.At() {
		for i, loc := range stacktrace.LocationIDs {
			stacktrace.LocationIDs[i] = uint64(r.locations.tryLookup(uint32(loc)))
		}
		unresolvedStacktraces.set(stacktrace)
	}

	// Resolve functions and mappings for new locations.
	var location *schemav1.InMemoryLocation
	unresolvedLocs := r.locations.iter()
	for i := p.Locations(unresolvedLocs); i.Next(); location = i.At() {
		location.MappingId = r.mappings.tryLookup(location.MappingId)
		for j, line := range location.Line {
			location.Line[j].FunctionId = r.functions.tryLookup(line.FunctionId)
		}
		unresolvedLocs.set(location)
	}

	// Resolve strings.
	var mapping *schemav1.InMemoryMapping
	unresolvedMappings := r.mappings.iter()
	for i := p.Mappings(unresolvedMappings); i.Next(); mapping = i.At() {
		mapping.BuildId = r.strings.tryLookup(mapping.BuildId)
		mapping.Filename = r.strings.tryLookup(mapping.Filename)
		unresolvedMappings.set(mapping)
	}
	var function *schemav1.InMemoryFunction
	unresolvedFunctions := r.functions.iter()
	for i := p.Functions(unresolvedFunctions); i.Next(); function = i.At() {
		function.Name = r.strings.tryLookup(function.Name)
		function.Filename = r.strings.tryLookup(function.Filename)
		function.SystemName = r.strings.tryLookup(function.SystemName)
		unresolvedFunctions.set(function)
	}
	var str string
	unresolvedStrings := r.strings.iter()
	for i := p.Strings(unresolvedStrings); i.Next(); str = i.At() {
		unresolvedStrings.set(str)
	}
}

func (r *stacktraceRewriter) append(stacktraces []uint32) {
	a := r.symbolsAppender()
	for _, str := range r.strings.values {
		r.functions.storeResolved(0, a.AppendString(str))
	}

	for _, function := range r.functions.values {
		function.Name = r.strings.lookupUnresolved(function.Name)
		function.Filename = r.strings.lookupUnresolved(function.Filename)
		function.SystemName = r.strings.lookupUnresolved(function.SystemName)
		r.functions.storeResolved(0, a.AppendFunction(function))
	}

	for _, mapping := range r.mappings.values {
		mapping.BuildId = r.strings.lookupUnresolved(mapping.BuildId)
		mapping.Filename = r.strings.lookupUnresolved(mapping.Filename)
		r.mappings.storeResolved(0, a.AppendMapping(mapping))
	}

	for _, location := range r.locations.values {
		location.MappingId = r.mappings.lookupUnresolved(location.MappingId)
		for j, line := range location.Line {
			location.Line[j].FunctionId = r.functions.lookupUnresolved(line.FunctionId)
		}
		r.locations.storeResolved(0, a.AppendLocation(location))
	}

	src := r.stacktraces[r.partition]
	for _, stacktrace := range src.values {
		for j, v := range stacktrace.LocationIDs {
			stacktrace.LocationIDs[j] = uint64(r.locations.lookupUnresolved(uint32(v)))
		}
		src.storeResolved(0, a.AppendStacktrace(stacktrace))
	}
	for i, v := range stacktraces {
		stacktraces[i] = src.lookupUnresolved(v)
	}
}

type symbolsWriter struct {
	// TODO(kolesnikovae):
	SymbolsAppender
}

func newSymbolsWriter(dst string) (*symbolsWriter, error) { return &symbolsWriter{}, nil }
