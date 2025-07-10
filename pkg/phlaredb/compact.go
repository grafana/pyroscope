package phlaredb

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/runutil"
	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/parquet-go/parquet-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"

	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/downsample"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/loser"
)

type BlockReader interface {
	Open(context.Context) error
	Meta() block.Meta
	Profiles() ProfileReader
	Index() IndexReader
	Symbols() symdb.SymbolsReader
	Close() error
}

type ProfileReader interface {
	io.ReaderAt
	Schema() *parquet.Schema
	Root() *parquet.Column
	RowGroups() []parquet.RowGroup
}

type CompactWithSplittingOpts struct {
	Src                []BlockReader
	Dst                string
	SplitCount         uint64
	StageSize          uint64
	SplitBy            SplitByFunc
	DownsamplerEnabled bool
	Logger             log.Logger
}

func Compact(ctx context.Context, src []BlockReader, dst string) (meta block.Meta, err error) {
	metas, err := CompactWithSplitting(ctx, CompactWithSplittingOpts{
		Src:                src,
		Dst:                dst,
		SplitCount:         1,
		StageSize:          0,
		SplitBy:            SplitByFingerprint,
		DownsamplerEnabled: true,
		Logger:             util.Logger,
	})
	if err != nil {
		return block.Meta{}, err
	}
	return metas[0], nil
}

func CompactWithSplitting(ctx context.Context, opts CompactWithSplittingOpts) (
	[]block.Meta, error,
) {
	if len(opts.Src) <= 1 && opts.SplitCount == 1 {
		return nil, errors.New("not enough blocks to compact")
	}
	if opts.SplitCount == 0 {
		opts.SplitCount = 1
	}
	if opts.StageSize == 0 || opts.StageSize > opts.SplitCount {
		opts.StageSize = opts.SplitCount
	}
	var (
		writers  = make([]*blockWriter, opts.SplitCount)
		srcMetas = make([]block.Meta, len(opts.Src))
		outMetas = make([]block.Meta, 0, len(writers))
		err      error
	)
	for i, b := range opts.Src {
		srcMetas[i] = b.Meta()
	}

	symbolsCompactor := newSymbolsCompactor(opts.Dst, symdb.FormatV2)
	defer runutil.CloseWithLogOnErr(util.Logger, symbolsCompactor, "close symbols compactor")

	outMeta := compactMetas(srcMetas...)
	for _, stage := range splitStages(len(writers), int(opts.StageSize)) {
		for _, idx := range stage {
			if writers[idx], err = createBlockWriter(blockWriterOpts{
				dst:                opts.Dst,
				meta:               outMeta,
				splitCount:         opts.SplitCount,
				shard:              idx,
				rewriterFn:         symbolsCompactor.Rewriter,
				downsamplerEnabled: opts.DownsamplerEnabled,
				logger:             opts.Logger,
			}); err != nil {
				return nil, fmt.Errorf("create block writer: %w", err)
			}
		}
		var metas []block.Meta
		sp, ctx := opentracing.StartSpanFromContext(ctx, "compact.Stage", opentracing.Tag{Key: "stage", Value: stage})
		if metas, err = compact(ctx, writers, opts.Src, opts.SplitBy, opts.SplitCount); err != nil {
			sp.Finish()
			ext.LogError(sp, err)
			return nil, err
		}
		sp.Finish()
		outMetas = append(outMetas, metas...)
		// Writers are already closed, and must be GCed.
		for j := range writers {
			writers[j] = nil
		}
	}

	return outMetas, nil
}

// splitStages splits n into sequences of size s:
// For n=7, s=3: [[0 1 2] [3 4 5] [6]]
func splitStages(n, s int) (stages [][]int) {
	for i := 0; i < n; i += s {
		end := i + s
		if end > n {
			end = n
		}
		b := make([]int, end-i)
		for j := i; j < end; j++ {
			b[j-i] = j
		}
		stages = append(stages, b)
	}
	return stages
}

func createBlockWriter(opts blockWriterOpts) (*blockWriter, error) {
	meta := opts.meta.Clone()
	meta.ULID = ulid.MustNew(meta.ULID.Time(), rand.Reader)
	if opts.splitCount > 1 {
		if meta.Labels == nil {
			meta.Labels = make(map[string]string)
		}
		meta.Labels[sharding.CompactorShardIDLabel] = sharding.FormatShardIDLabelValue(uint64(opts.shard), opts.splitCount)
	}
	opts.meta = *meta
	return newBlockWriter(opts)
}

func compact(ctx context.Context, writers []*blockWriter, readers []BlockReader, splitBy SplitByFunc, splitCount uint64) ([]block.Meta, error) {
	rowsIt, err := newMergeRowProfileIterator(readers)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithLogOnErr(util.Logger, rowsIt, "close rows iterator")
	// iterate and splits the rows into series.
	for rowsIt.Next() {
		r := rowsIt.At()
		shard := int(splitBy(r, splitCount))
		w := writers[shard]
		if w == nil {
			continue
		}
		if err = w.WriteRow(r); err != nil {
			return nil, err
		}
	}
	if err = rowsIt.Err(); err != nil {
		return nil, err
	}
	sp, ctx := opentracing.StartSpanFromContext(ctx, "compact.Close")
	defer sp.Finish()

	// Close all blocks
	errs := multierror.New()
	for _, w := range writers {
		if w == nil {
			continue
		}
		if err = w.Close(ctx); err != nil {
			errs.Add(err)
		}
	}

	out := make([]block.Meta, 0, len(writers))
	for _, w := range writers {
		if w == nil {
			continue
		}
		if w.meta.Stats.NumSamples > 0 {
			out = append(out, *w.meta)
		}
	}

	// Returns all Metas
	return out, errs.Err()
}

type SplitByFunc func(r profileRow, shardsCount uint64) uint64

var SplitByFingerprint = func(r profileRow, shardsCount uint64) uint64 {
	return uint64(r.fp) % shardsCount
}

var SplitByStacktracePartition = func(r profileRow, shardsCount uint64) uint64 {
	return r.row.StacktracePartitionID() % shardsCount
}

type blockWriter struct {
	indexRewriter   *indexRewriter
	symbolsRewriter SymbolsRewriter
	profilesWriter  *profilesWriter
	downsampler     *downsample.Downsampler
	path            string
	meta            *block.Meta
	totalProfiles   uint64
}

type SymbolsRewriter interface {
	ReWriteRow(profile profileRow) error
	Close() (uint64, error)
}

type SymbolsRewriterFn func(blockPath string) SymbolsRewriter

type blockWriterOpts struct {
	dst                string
	splitCount         uint64
	shard              int
	meta               block.Meta
	rewriterFn         SymbolsRewriterFn
	downsamplerEnabled bool
	logger             log.Logger
}

func newBlockWriter(opts blockWriterOpts) (*blockWriter, error) {
	blockPath := filepath.Join(opts.dst, opts.meta.ULID.String())

	if err := os.MkdirAll(blockPath, 0o777); err != nil {
		return nil, err
	}

	profileWriter, err := newProfileWriter(blockPath)
	if err != nil {
		return nil, err
	}

	var downsampler *downsample.Downsampler
	if opts.downsamplerEnabled && opts.meta.Compaction.Level > 2 {
		level.Debug(opts.logger).Log("msg", "downsampling enabled for block writer", "path", blockPath)
		downsampler, err = downsample.NewDownsampler(blockPath, opts.logger)
		if err != nil {
			return nil, err
		}
	}

	return &blockWriter{
		indexRewriter:   newIndexRewriter(blockPath),
		symbolsRewriter: opts.rewriterFn(blockPath),
		profilesWriter:  profileWriter,
		downsampler:     downsampler,
		path:            blockPath,
		meta:            &opts.meta,
	}, nil
}

func (bw *blockWriter) WriteRow(r profileRow) error {
	err := bw.indexRewriter.ReWriteRow(r)
	if err != nil {
		return err
	}
	err = bw.symbolsRewriter.ReWriteRow(r)
	if err != nil {
		return err
	}

	if err := bw.profilesWriter.WriteRow(r); err != nil {
		return err
	}
	if bw.downsampler != nil {
		err := bw.downsampler.AddRow(r.row, r.fp)
		if err != nil {
			return err
		}
	}
	bw.totalProfiles++
	return nil
}

func (bw *blockWriter) Close(ctx context.Context) error {
	if err := bw.indexRewriter.Close(ctx); err != nil {
		return err
	}
	numSamples, err := bw.symbolsRewriter.Close()
	if err != nil {
		return err
	}
	if err := bw.profilesWriter.Close(); err != nil {
		return err
	}
	if bw.downsampler != nil {
		if err := bw.downsampler.Close(); err != nil {
			return err
		}
	}
	metaFiles, err := metaFilesFromDir(bw.path)
	if err != nil {
		return err
	}
	bw.meta.Files = metaFiles
	bw.meta.Stats.NumProfiles = bw.totalProfiles
	bw.meta.Stats.NumSeries = bw.indexRewriter.NumSeries()
	bw.meta.Stats.NumSamples = numSamples
	bw.meta.Compaction.Deletable = bw.totalProfiles == 0
	if _, err := bw.meta.WriteToFile(util.Logger, bw.path); err != nil {
		return err
	}
	return nil
}

type profilesWriter struct {
	*parquet.GenericWriter[*schemav1.Profile]
	file *os.File

	buf []parquet.Row
}

func newProfileWriter(path string) (*profilesWriter, error) {
	profilePath := filepath.Join(path, (&schemav1.ProfilePersister{}).Name()+block.ParquetSuffix)
	profileFile, err := os.OpenFile(profilePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	return &profilesWriter{
		GenericWriter: newParquetProfileWriter(profileFile, parquet.MaxRowsPerRowGroup(int64(defaultParquetConfig.MaxBufferRowCount))),
		file:          profileFile,
		buf:           make([]parquet.Row, 1),
	}, nil
}

func (p *profilesWriter) WriteRow(r profileRow) error {
	p.buf[0] = parquet.Row(r.row)
	_, err := p.GenericWriter.WriteRows(p.buf)
	if err != nil {
		return err
	}

	return nil
}

func (p *profilesWriter) Close() error {
	err := p.GenericWriter.Close()
	if err != nil {
		return err
	}
	return p.file.Close()
}

func newIndexRewriter(path string) *indexRewriter {
	return &indexRewriter{
		symbols: make(map[string]struct{}),
		path:    path,
	}
}

type indexRewriter struct {
	series []struct {
		labels phlaremodel.Labels
		fp     model.Fingerprint
	}
	symbols map[string]struct{}
	chunks  []index.ChunkMeta // one chunk per series

	previousFp model.Fingerprint

	path string
}

func (idxRw *indexRewriter) ReWriteRow(r profileRow) error {
	if idxRw.previousFp != r.fp || len(idxRw.series) == 0 {
		series := r.labels.Clone()
		for _, l := range series {
			idxRw.symbols[l.Name] = struct{}{}
			idxRw.symbols[l.Value] = struct{}{}
		}
		idxRw.series = append(idxRw.series, struct {
			labels phlaremodel.Labels
			fp     model.Fingerprint
		}{
			labels: series,
			fp:     r.fp,
		})
		idxRw.chunks = append(idxRw.chunks, index.ChunkMeta{
			MinTime:     r.timeNanos,
			MaxTime:     r.timeNanos,
			SeriesIndex: uint32(len(idxRw.series) - 1),
		})
		idxRw.previousFp = r.fp
	}
	idxRw.chunks[len(idxRw.chunks)-1].MaxTime = r.timeNanos
	r.row.SetSeriesIndex(idxRw.chunks[len(idxRw.chunks)-1].SeriesIndex)
	return nil
}

func (idxRw *indexRewriter) NumSeries() uint64 {
	return uint64(len(idxRw.series))
}

// Close writes the index to given folder.
func (idxRw *indexRewriter) Close(ctx context.Context) error {
	indexw, err := index.NewWriter(ctx, filepath.Join(idxRw.path, block.IndexFilename))
	if err != nil {
		return err
	}

	// Sort symbols
	symbols := make([]string, 0, len(idxRw.symbols))
	for s := range idxRw.symbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// Add symbols
	for _, symbol := range symbols {
		if err := indexw.AddSymbol(symbol); err != nil {
			return err
		}
	}

	// Add Series
	for i, series := range idxRw.series {
		if err := indexw.AddSeries(storage.SeriesRef(i), series.labels, series.fp, idxRw.chunks[i]); err != nil {
			return err
		}
	}

	return indexw.Close()
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
	parents := make([]block.BlockDesc, 0, len(src))
	minTime, maxTime := model.Latest, model.Earliest
	labels := make(map[string]string)
	for _, b := range src {
		if b.Compaction.Level > highestCompactionLevel {
			highestCompactionLevel = b.Compaction.Level
		}
		for _, s := range b.Compaction.Sources {
			sources[s] = struct{}{}
		}
		parents = append(parents, block.BlockDesc{
			ULID:    b.ULID,
			MinTime: b.MinTime,
			MaxTime: b.MaxTime,
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
	meta.Source = block.CompactorSource
	meta.Compaction = block.BlockMetaCompaction{
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
	meta.ULID = ulid.MustNew(uint64(minTime), rand.Reader)
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
	closer      io.Closer
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
	reader := parquet.NewReader(s.Profiles(), schemav1.ProfilesSchema)
	return &profileRowIterator{
		profiles:         phlareparquet.NewBufferedRowReaderIterator(reader, 32),
		blockReader:      s,
		closer:           reader,
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
	err := p.profiles.Close()
	if p.closer != nil {
		if err := p.closer.Close(); err != nil {
			return err
		}
	}
	return err
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

type symbolsCompactor struct {
	version     symdb.FormatVersion
	rewriters   map[BlockReader]*symdb.Rewriter
	w           *symdb.SymDB
	stacktraces []uint32

	dst     string
	flushed bool
}

func newSymbolsCompactor(path string, version symdb.FormatVersion) *symbolsCompactor {
	if version == symdb.FormatV3 {
		return &symbolsCompactor{
			version: version,
			w: symdb.NewSymDB(symdb.DefaultConfig().
				WithVersion(symdb.FormatV3).
				WithDirectory(path)),
			dst:       path,
			rewriters: make(map[BlockReader]*symdb.Rewriter),
		}
	}
	dst := filepath.Join(path, symdb.DefaultDirName)
	return &symbolsCompactor{
		version: symdb.FormatV2,
		w: symdb.NewSymDB(symdb.DefaultConfig().
			WithVersion(symdb.FormatV2).
			WithDirectory(dst)),
		dst:       dst,
		rewriters: make(map[BlockReader]*symdb.Rewriter),
	}
}

func (s *symbolsCompactor) Rewriter(dst string) SymbolsRewriter {
	return &symbolsRewriter{
		symbolsCompactor: s,
		dst:              dst,
	}
}

type symbolsRewriter struct {
	*symbolsCompactor

	numSamples uint64
	dst        string
}

func (s *symbolsRewriter) NumSamples() uint64 { return s.numSamples }

func (s *symbolsRewriter) ReWriteRow(profile profileRow) error {
	total, err := s.symbolsCompactor.ReWriteRow(profile)
	s.numSamples += total
	return err
}

func (s *symbolsRewriter) Close() (uint64, error) {
	if err := s.symbolsCompactor.Flush(); err != nil {
		return 0, err
	}
	if s.version == symdb.FormatV3 {
		dst := filepath.Join(s.dst, symdb.DefaultFileName)
		src := filepath.Join(s.symbolsCompactor.dst, symdb.DefaultFileName)
		return s.numSamples, util.CopyFile(src, dst)
	} else {
		return s.numSamples, util.CopyDir(s.symbolsCompactor.dst, filepath.Join(s.dst, symdb.DefaultDirName))
	}
}

func (s *symbolsCompactor) ReWriteRow(profile profileRow) (uint64, error) {
	var (
		err              error
		rewrittenSamples uint64
	)
	profile.row.ForStacktraceIDsValues(func(values []parquet.Value) {
		s.loadStacktracesID(values)
		r, ok := s.rewriters[profile.blockReader]
		if !ok {
			r = symdb.NewRewriter(s.w, profile.blockReader.Symbols(), nil)
			s.rewriters[profile.blockReader] = r
		}
		if err = r.Rewrite(profile.row.StacktracePartitionID(), s.stacktraces); err != nil {
			return
		}
		rewrittenSamples += uint64(len(values))
		for i, v := range values {
			// FIXME: the original order is not preserved, which will affect encoding.
			values[i] = parquet.Int64Value(int64(s.stacktraces[i])).Level(v.RepetitionLevel(), v.DefinitionLevel(), v.Column())
		}
	})
	if err != nil {
		return rewrittenSamples, err
	}
	return rewrittenSamples, nil
}

func (s *symbolsCompactor) Flush() error {
	if s.flushed {
		return nil
	}
	if err := s.w.Flush(); err != nil {
		return err
	}
	s.flushed = true
	return nil
}

func (s *symbolsCompactor) Close() error {
	if s.version == symdb.FormatV3 {
		return os.RemoveAll(filepath.Join(s.dst, symdb.DefaultFileName))
	}
	return os.RemoveAll(s.dst)
}

func (s *symbolsCompactor) loadStacktracesID(values []parquet.Value) {
	s.stacktraces = grow(s.stacktraces, len(values))
	for i := range values {
		s.stacktraces[i] = values[i].Uint32()
	}
}
