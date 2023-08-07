package phlaredb

import (
	"context"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	"github.com/parquet-go/parquet-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"

	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/loser"
)

type BlockReader interface {
	Meta() block.Meta
	Profiles() []parquet.RowGroup
	Index() IndexReader
	Symbols() SymbolsReader
}

func Compact(ctx context.Context, src []BlockReader, dst string) (meta block.Meta, err error) {
	srcMetas := make([]block.Meta, len(src))
	ulids := make([]string, len(src))

	for i, b := range src {
		srcMetas[i] = b.Meta()
		ulids[i] = b.Meta().ULID.String()
	}
	meta = compactMetas(srcMetas...)
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
	symw, err := newSymbolsWriter(blockPath, defaultParquetConfig)
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

	if err = symRewriter.Close(); err != nil {
		return block.Meta{}, err
	}
	if err = symw.Close(); err != nil {
		return block.Meta{}, err
	}

	// flush the index file.
	if err = indexw.Close(); err != nil {
		return block.Meta{}, err
	}

	if err = profileWriter.Close(); err != nil {
		return block.Meta{}, err
	}

	metaFiles, err := metaFilesFromDir(blockPath)
	if err != nil {
		return block.Meta{}, err
	}
	meta.Files = metaFiles
	meta.Stats.NumProfiles = total
	meta.Stats.NumSeries = seriesRewriter.NumSeries()
	meta.Stats.NumSamples = symRewriter.NumSamples()
	meta.Compaction.Deletable = meta.Stats.NumSamples == 0
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

func compactMetas(src ...block.Meta) block.Meta {
	meta := block.NewMeta()
	highestCompactionLevel := 0
	sources := map[ulid.ULID]struct{}{}
	parents := make([]tsdb.BlockDesc, 0, len(src))
	minTime, maxTime := model.Latest, model.Earliest
	labels := make(map[string]string)
	for _, b := range src {
		if b.Compaction.Level > highestCompactionLevel {
			highestCompactionLevel = b.Compaction.Level
		}
		for _, s := range b.Compaction.Sources {
			sources[s] = struct{}{}
		}
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
		Deletable: false,
		Level:     highestCompactionLevel + 1,
		Parents:   parents,
	}
	for s := range sources {
		meta.Compaction.Sources = append(meta.Compaction.Sources, s)
	}
	sort.Slice(meta.Compaction.Sources, func(i, j int) bool {
		return meta.Compaction.Sources[i].Compare(meta.Compaction.Sources[j]) < 0
	})
	meta.MaxTime = maxTime
	meta.MinTime = minTime
	meta.Labels = labels
	return *meta
}

type profileRow struct {
	timeNanos int64

	labels phlaremodel.Labels
	fp     model.Fingerprint
	row    schemav1.ProfileRow

	blockReader BlockReader
}

type profileRowIterator struct {
	profiles    iter.Iterator[parquet.Row]
	blockReader BlockReader
	index       IndexReader
	allPostings index.Postings
	err         error

	currentRow       profileRow
	currentSeriesIdx uint32
	chunks           []index.ChunkMeta
}

func newProfileRowIterator(s BlockReader) (*profileRowIterator, error) {
	k, v := index.AllPostingsKey()
	allPostings, err := s.Index().Postings(k, nil, v)
	if err != nil {
		return nil, err
	}
	// todo close once https://github.com/grafana/pyroscope/issues/2172 is done.
	reader := parquet.MultiRowGroup(s.Profiles()...).Rows()
	return &profileRowIterator{
		profiles:         phlareparquet.NewBufferedRowReaderIterator(reader, 1024),
		blockReader:      s,
		index:            s.Index(),
		allPostings:      allPostings,
		currentSeriesIdx: math.MaxUint32,
		chunks:           make([]index.ChunkMeta, 1),
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
	if seriesIndex == p.currentSeriesIdx {
		return true
	}
	p.currentSeriesIdx = seriesIndex
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
		it, err := newProfileRowIterator(s)
		if err != nil {
			return nil, err
		}
		its[i] = it
	}
	if len(its) == 1 {
		return its[0], nil
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
	done      bool
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
		if s.done {
			return false
		}
		s.done = true
		if s.previousFp != 0 {
			s.currentChunkMeta.SeriesIndex = uint32(s.seriesRef) - 1
			if err := s.indexw.AddSeries(s.seriesRef-1, s.labels, s.previousFp, s.currentChunkMeta); err != nil {
				s.err = err
				return false
			}
			s.numSeries++
		}
		return false
	}
	currentProfile := s.Iterator.At()
	if s.previousFp != currentProfile.fp {
		if s.previousFp != 0 {
			s.currentChunkMeta.SeriesIndex = uint32(s.seriesRef) - 1
			if err := s.indexw.AddSeries(s.seriesRef-1, s.labels, s.previousFp, s.currentChunkMeta); err != nil {
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
	currentProfile.row.SetSeriesIndex(uint32(s.seriesRef - 1))
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
	profiles    iter.Iterator[profileRow]
	rewriters   map[BlockReader]*stacktraceRewriter
	stacktraces []uint32
	err         error

	numSamples uint64
}

func newSymbolsRewriter(it iter.Iterator[profileRow], blocks []BlockReader, w SymbolsWriter) *symbolsRewriter {
	sr := symbolsRewriter{
		profiles:  it,
		rewriters: make(map[BlockReader]*stacktraceRewriter, len(blocks)),
	}
	for _, r := range blocks {
		sr.rewriters[r] = newStacktraceRewriter(r.Symbols(), w)
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
			// FIXME: the original order is not preserved, which will affect encoding.
			values[i] = parquet.Int64Value(int64(s.stacktraces[i])).Level(v.RepetitionLevel(), v.DefinitionLevel(), v.Column())
		}
	})
	if err != nil {
		s.err = err
		return false
	}
	return true
}

func (s *symbolsRewriter) loadStacktracesID(values []parquet.Value) {
	s.stacktraces = grow(s.stacktraces, len(values))
	for i := range values {
		s.stacktraces[i] = values[i].Uint32()
	}
}

type stacktraceRewriter struct {
	reader SymbolsReader
	writer SymbolsWriter

	partitions map[uint64]*symPartitionRewriter
	inserter   *stacktraceInserter

	// Objects below have global addressing.
	// TODO(kolesnikovae): Move to partition.
	locations *lookupTable[*schemav1.InMemoryLocation]
	mappings  *lookupTable[*schemav1.InMemoryMapping]
	functions *lookupTable[*schemav1.InMemoryFunction]
	strings   *lookupTable[string]
}

type symPartitionRewriter struct {
	name  uint64
	stats symdb.Stats
	// Stacktrace identifiers are only valid within the partition.
	stacktraces *lookupTable[[]int32]
	resolver    SymbolsResolver
	appender    SymbolsAppender

	r *stacktraceRewriter

	// FIXME(kolesnikovae): schemav1.Stacktrace should be just a uint32 slice:
	//   type Stacktrace []uint32
	current []*schemav1.Stacktrace
}

func newStacktraceRewriter(r SymbolsReader, w SymbolsWriter) *stacktraceRewriter {
	return &stacktraceRewriter{
		reader: r,
		writer: w,
	}
}

func (r *stacktraceRewriter) init(partition uint64) (p *symPartitionRewriter, err error) {
	if r.partitions == nil {
		r.partitions = make(map[uint64]*symPartitionRewriter)
	}
	if p, err = r.getOrCreatePartition(partition); err != nil {
		return nil, err
	}

	if r.locations == nil {
		r.locations = newLookupTable[*schemav1.InMemoryLocation](p.stats.LocationsTotal)
		r.mappings = newLookupTable[*schemav1.InMemoryMapping](p.stats.MappingsTotal)
		r.functions = newLookupTable[*schemav1.InMemoryFunction](p.stats.FunctionsTotal)
		r.strings = newLookupTable[string](p.stats.StringsTotal)
	} else {
		r.locations.reset()
		r.mappings.reset()
		r.functions.reset()
		r.strings.reset()
	}

	r.inserter = &stacktraceInserter{
		stacktraces: p.stacktraces,
		locations:   r.locations,
	}

	return p, nil
}

func (r *stacktraceRewriter) getOrCreatePartition(partition uint64) (_ *symPartitionRewriter, err error) {
	p, ok := r.partitions[partition]
	if ok {
		p.reset()
		return p, nil
	}
	n := &symPartitionRewriter{r: r, name: partition}
	if n.resolver, err = r.reader.SymbolsResolver(partition); err != nil {
		return nil, err
	}
	if n.appender, err = r.writer.SymbolsAppender(partition); err != nil {
		return nil, err
	}
	n.resolver.WriteStats(&n.stats)
	n.stacktraces = newLookupTable[[]int32](n.stats.MaxStacktraceID)
	r.partitions[partition] = n
	return n, nil
}

func (r *stacktraceRewriter) rewriteStacktraces(partition uint64, stacktraces []uint32) error {
	p, err := r.init(partition)
	if err != nil {
		return err
	}
	if err = p.populateUnresolved(stacktraces); err != nil {
		return err
	}
	if p.hasUnresolved() {
		return p.appendRewrite(stacktraces)
	}
	return nil
}

func (p *symPartitionRewriter) reset() {
	p.stacktraces.reset()
	p.current = p.current[:0]
}

func (p *symPartitionRewriter) hasUnresolved() bool {
	return len(p.stacktraces.unresolved)+
		len(p.r.locations.unresolved)+
		len(p.r.mappings.unresolved)+
		len(p.r.functions.unresolved)+
		len(p.r.strings.unresolved) > 0
}

func (p *symPartitionRewriter) populateUnresolved(stacktraceIDs []uint32) error {
	// Filter out all stack traces that have been already
	// resolved and populate locations lookup table.
	if err := p.resolveStacktraces(stacktraceIDs); err != nil {
		return err
	}
	if len(p.r.locations.unresolved) == 0 {
		return nil
	}

	// Resolve functions and mappings for new locations.
	unresolvedLocs := p.r.locations.iter()
	locations := p.resolver.Locations(unresolvedLocs)
	for locations.Next() {
		location := locations.At()
		location.MappingId = p.r.mappings.tryLookup(location.MappingId)
		for j, line := range location.Line {
			location.Line[j].FunctionId = p.r.functions.tryLookup(line.FunctionId)
		}
		unresolvedLocs.setValue(location)
	}
	if err := locations.Err(); err != nil {
		return err
	}

	// Resolve strings.
	unresolvedMappings := p.r.mappings.iter()
	mappings := p.resolver.Mappings(unresolvedMappings)
	for mappings.Next() {
		mapping := mappings.At()
		mapping.BuildId = p.r.strings.tryLookup(mapping.BuildId)
		mapping.Filename = p.r.strings.tryLookup(mapping.Filename)
		unresolvedMappings.setValue(mapping)
	}
	if err := mappings.Err(); err != nil {
		return err
	}

	unresolvedFunctions := p.r.functions.iter()
	functions := p.resolver.Functions(unresolvedFunctions)
	for functions.Next() {
		function := functions.At()
		function.Name = p.r.strings.tryLookup(function.Name)
		function.Filename = p.r.strings.tryLookup(function.Filename)
		function.SystemName = p.r.strings.tryLookup(function.SystemName)
		unresolvedFunctions.setValue(function)
	}
	if err := functions.Err(); err != nil {
		return err
	}

	unresolvedStrings := p.r.strings.iter()
	strings := p.resolver.Strings(unresolvedStrings)
	for strings.Next() {
		unresolvedStrings.setValue(strings.At())
	}
	return strings.Err()
}

func (p *symPartitionRewriter) appendRewrite(stacktraces []uint32) error {
	p.appender.AppendStrings(p.r.strings.buf, p.r.strings.values)
	p.r.strings.updateResolved()

	for _, v := range p.r.functions.values {
		v.Name = p.r.strings.lookupResolved(v.Name)
		v.Filename = p.r.strings.lookupResolved(v.Filename)
		v.SystemName = p.r.strings.lookupResolved(v.SystemName)
	}
	p.appender.AppendFunctions(p.r.functions.buf, p.r.functions.values)
	p.r.functions.updateResolved()

	for _, v := range p.r.mappings.values {
		v.BuildId = p.r.strings.lookupResolved(v.BuildId)
		v.Filename = p.r.strings.lookupResolved(v.Filename)
	}
	p.appender.AppendMappings(p.r.mappings.buf, p.r.mappings.values)
	p.r.mappings.updateResolved()

	for _, v := range p.r.locations.values {
		v.MappingId = p.r.mappings.lookupResolved(v.MappingId)
		for j, line := range v.Line {
			v.Line[j].FunctionId = p.r.functions.lookupResolved(line.FunctionId)
		}
	}
	p.appender.AppendLocations(p.r.locations.buf, p.r.locations.values)
	p.r.locations.updateResolved()

	for _, v := range p.stacktraces.values {
		for j, location := range v {
			v[j] = int32(p.r.locations.lookupResolved(uint32(location)))
		}
	}
	p.appender.AppendStacktraces(p.stacktraces.buf, p.stacktracesFromResolvedValues())
	p.stacktraces.updateResolved()

	for i, v := range stacktraces {
		stacktraces[i] = p.stacktraces.lookupResolved(v)
	}

	return nil
}

func (p *symPartitionRewriter) resolveStacktraces(stacktraceIDs []uint32) error {
	for i, v := range stacktraceIDs {
		stacktraceIDs[i] = p.stacktraces.tryLookup(v)
	}
	if len(p.stacktraces.unresolved) == 0 {
		return nil
	}
	p.stacktraces.initSorted()
	return p.resolver.ResolveStacktraceLocations(context.TODO(), p.r.inserter, p.stacktraces.buf)
}

func (p *symPartitionRewriter) stacktracesFromResolvedValues() []*schemav1.Stacktrace {
	p.current = grow(p.current, len(p.stacktraces.values))
	for i, v := range p.stacktraces.values {
		s := p.current[i]
		if s == nil {
			s = &schemav1.Stacktrace{LocationIDs: make([]uint64, len(v))}
			p.current[i] = s
		}
		s.LocationIDs = grow(s.LocationIDs, len(v))
		for j, m := range v {
			s.LocationIDs[j] = uint64(m)
		}
	}
	return p.current
}

type stacktraceInserter struct {
	stacktraces *lookupTable[[]int32]
	locations   *lookupTable[*schemav1.InMemoryLocation]
}

func (i *stacktraceInserter) InsertStacktrace(stacktrace uint32, locations []int32) {
	// Resolve locations for new stack traces.
	for j, loc := range locations {
		locations[j] = int32(i.locations.tryLookup(uint32(loc)))
	}
	// stacktrace points to resolved which should
	// be a marked pointer to unresolved value.
	idx := i.stacktraces.resolved[stacktrace] & markerMask
	v := &i.stacktraces.values[idx]
	n := grow(*v, len(locations))
	copy(n, locations)
	// Preserve allocated capacity.
	i.stacktraces.values[idx] = n
}

const (
	marker     = 1 << 31
	markerMask = math.MaxUint32 >> 1
)

type lookupTable[T any] struct {
	// Index is source ID, and the value is the destination ID.
	// If destination ID is not known, the element is index to 'unresolved' (marked).
	resolved   []uint32
	unresolved []uint32 // Points to resolved. Index matches values.
	values     []T      // Values are populated for unresolved items.
	buf        []uint32 // Sorted unresolved values.
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

func (t *lookupTable[T]) reset() {
	t.unresolved = t.unresolved[:0]
	t.values = t.values[:0]
	t.buf = t.buf[:0]
}

// tryLookup looks up the value at x in resolved.
// If x is has not been resolved yet, the x is memorized
// for future resolve, and returned values is the marked
// index to unresolved.
func (t *lookupTable[T]) tryLookup(x uint32) uint32 {
	if v := t.resolved[x]; v != 0 {
		if v&marker > 0 {
			return v // Already marked for resolve.
		}
		return v - 1 // Already resolved.
	}
	u := t.newUnresolved(x) | marker
	t.resolved[x] = u
	return u
}

func (t *lookupTable[T]) newUnresolved(rid uint32) uint32 {
	t.unresolved = append(t.unresolved, rid)
	x := len(t.values)
	if x < cap(t.values) {
		// Try to reuse previously allocated value.
		t.values = t.values[:x+1]
	} else {
		var v T
		t.values = append(t.values, v)
	}
	return uint32(x)
}

func (t *lookupTable[T]) storeResolved(i int, rid uint32) {
	// The index is incremented to avoid 0 because it is
	// used as sentinel and indicates absence (resolved is
	// a sparse slice initialized with the maximal expected
	// size). Correspondingly, lookupResolved should
	// decrement the index on read.
	t.resolved[t.unresolved[i]] = rid + 1
}

func (t *lookupTable[T]) lookupResolved(x uint32) uint32 {
	if x&marker > 0 {
		return t.resolved[t.unresolved[x&markerMask]] - 1
	}
	return x // Already resolved.
}

// updateResolved loads indices from buf to resolved.
// It is expected that the order matches values.
func (t *lookupTable[T]) updateResolved() {
	for i, rid := range t.unresolved {
		t.resolved[rid] = t.buf[i] + 1
	}
}

func (t *lookupTable[T]) initSorted() {
	// Gather and sort references to unresolved values.
	t.buf = grow(t.buf, len(t.unresolved))
	copy(t.buf, t.unresolved)
	sort.Slice(t.buf, func(i, j int) bool {
		return t.buf[i] < t.buf[j]
	})
}

func (t *lookupTable[T]) iter() *lookupTableIterator[T] {
	t.initSorted()
	return &lookupTableIterator[T]{table: t}
}

type lookupTableIterator[T any] struct {
	table *lookupTable[T]
	cur   uint32
}

func (t *lookupTableIterator[T]) Next() bool {
	return t.cur < uint32(len(t.table.buf))
}

func (t *lookupTableIterator[T]) At() uint32 {
	x := t.table.buf[t.cur]
	t.cur++
	return x
}

func (t *lookupTableIterator[T]) setValue(v T) {
	u := t.table.resolved[t.table.buf[t.cur-1]]
	t.table.values[u&markerMask] = v
}

func (t *lookupTableIterator[T]) Close() error { return nil }

func (t *lookupTableIterator[T]) Err() error { return nil }

func grow[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n, 2*n)
	}
	return s[:n]
}
