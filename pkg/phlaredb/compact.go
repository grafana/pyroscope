package phlaredb

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/oklog/ulid"
	"github.com/parquet-go/parquet-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"

	"github.com/grafana/dskit/multierror"
	"github.com/grafana/dskit/runutil"

	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/loser"
)

type BlockReader interface {
	Meta() block.Meta
	Profiles() []parquet.RowGroup
	Index() IndexReader
	Symbols() symdb.SymbolsReader
	Close() error
}

func Compact(ctx context.Context, src []BlockReader, dst string) (meta block.Meta, err error) {
	metas, err := CompactWithSplitting(ctx, src, 1, dst)
	if err != nil {
		return block.Meta{}, err
	}
	return metas[0], nil
}

func CompactWithSplitting(ctx context.Context, src []BlockReader, shardsCount uint64, dst string) (
	[]block.Meta, error,
) {
	if shardsCount == 0 {
		shardsCount = 1
	}
	if len(src) <= 1 && shardsCount == 1 {
		return nil, errors.New("not enough blocks to compact")
	}
	var (
		writers  = make([]*blockWriter, shardsCount)
		shardBy  = shardByFingerprint
		srcMetas = make([]block.Meta, len(src))
		err      error
	)
	for i, b := range src {
		srcMetas[i] = b.Meta()
	}

	outBlocksTime := ulid.Now()
	outMeta := compactMetas(srcMetas...)

	// create the shards writers
	for i := range writers {
		meta := outMeta.Clone()
		meta.ULID = ulid.MustNew(outBlocksTime, rand.Reader)
		writers[i], err = newBlockWriter(dst, meta)
		if err != nil {
			return nil, fmt.Errorf("create block writer: %w", err)
		}
	}

	rowsIt, err := newMergeRowProfileIterator(src)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithLogOnErr(util.Logger, rowsIt, "close rows iterator")

	// iterate and splits the rows into series.
	for rowsIt.Next() {
		r := rowsIt.At()
		shard := int(shardBy(r, shardsCount))
		if err := writers[shard].WriteRow(r); err != nil {
			return nil, err
		}
	}
	if err := rowsIt.Err(); err != nil {
		return nil, err
	}

	// Close all blocks
	errs := multierror.New()
	for _, w := range writers {
		if err := w.Close(ctx); err != nil {
			errs.Add(err)
		}
	}

	out := make([]block.Meta, 0, len(writers))
	for shard, w := range writers {
		if w.meta.Stats.NumSamples > 0 {
			if shardsCount > 1 {
				w.meta.Labels[sharding.CompactorShardIDLabel] = sharding.FormatShardIDLabelValue(uint64(shard), shardsCount)
			}
			out = append(out, *w.meta)
		}
	}

	// Returns all Metas
	return out, errs.Err()
}

var shardByFingerprint = func(r profileRow, shardsCount uint64) uint64 {
	return uint64(r.fp) % shardsCount
}

type blockWriter struct {
	indexRewriter   *indexRewriter
	symbolsRewriter *symbolsRewriter
	profilesWriter  *profilesWriter
	path            string
	meta            *block.Meta
	totalProfiles   uint64
	min, max        int64
}

func newBlockWriter(dst string, meta *block.Meta) (*blockWriter, error) {
	blockPath := filepath.Join(dst, meta.ULID.String())

	if err := os.MkdirAll(blockPath, 0o777); err != nil {
		return nil, err
	}

	profileWriter, err := newProfileWriter(blockPath)
	if err != nil {
		return nil, err
	}

	return &blockWriter{
		indexRewriter:   newIndexRewriter(blockPath),
		symbolsRewriter: newSymbolsRewriter(blockPath),
		profilesWriter:  profileWriter,
		path:            blockPath,
		meta:            meta,
		min:             math.MaxInt64,
		max:             math.MinInt64,
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
	bw.totalProfiles++
	if r.timeNanos < bw.min {
		bw.min = r.timeNanos
	}
	if r.timeNanos > bw.max {
		bw.max = r.timeNanos
	}
	return nil
}

func (bw *blockWriter) Close(ctx context.Context) error {
	if err := bw.indexRewriter.Close(ctx); err != nil {
		return err
	}
	if err := bw.symbolsRewriter.Close(); err != nil {
		return err
	}
	if err := bw.profilesWriter.Close(); err != nil {
		return err
	}
	metaFiles, err := metaFilesFromDir(bw.path)
	if err != nil {
		return err
	}
	bw.meta.Files = metaFiles
	bw.meta.Stats.NumProfiles = bw.totalProfiles
	bw.meta.Stats.NumSeries = bw.indexRewriter.NumSeries()
	bw.meta.Stats.NumSamples = bw.symbolsRewriter.NumSamples()
	bw.meta.Compaction.Deletable = bw.totalProfiles == 0
	bw.meta.MinTime = model.TimeFromUnixNano(bw.min)
	bw.meta.MaxTime = model.TimeFromUnixNano(bw.max)
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

type symbolsRewriter struct {
	rewriters   map[BlockReader]*symdb.Rewriter
	w           *symdb.SymDB
	stacktraces []uint32

	numSamples uint64
}

func newSymbolsRewriter(path string) *symbolsRewriter {
	return &symbolsRewriter{
		w: symdb.NewSymDB(symdb.DefaultConfig().
			WithDirectory(filepath.Join(path, symdb.DefaultDirName)).
			WithParquetConfig(symdb.ParquetConfig{
				MaxBufferRowCount: defaultParquetConfig.MaxBufferRowCount,
			})),
		rewriters: make(map[BlockReader]*symdb.Rewriter),
	}
}

func (s *symbolsRewriter) NumSamples() uint64 { return s.numSamples }

func (s *symbolsRewriter) ReWriteRow(profile profileRow) error {
	var err error
	profile.row.ForStacktraceIDsValues(func(values []parquet.Value) {
		s.loadStacktracesID(values)
		r, ok := s.rewriters[profile.blockReader]
		if !ok {
			r = symdb.NewRewriter(s.w, profile.blockReader.Symbols())
			s.rewriters[profile.blockReader] = r
		}
		if err = r.Rewrite(profile.row.StacktracePartitionID(), s.stacktraces); err != nil {
			return
		}
		s.numSamples += uint64(len(values))
		for i, v := range values {
			// FIXME: the original order is not preserved, which will affect encoding.
			values[i] = parquet.Int64Value(int64(s.stacktraces[i])).Level(v.RepetitionLevel(), v.DefinitionLevel(), v.Column())
		}
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *symbolsRewriter) Close() error {
	return s.w.Flush()
}

func (s *symbolsRewriter) loadStacktracesID(values []parquet.Value) {
	s.stacktraces = grow(s.stacktraces, len(values))
	for i := range values {
		s.stacktraces[i] = values[i].Uint32()
	}
}
