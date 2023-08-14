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
	Symbols() symdb.SymbolsReader
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
	symw := symdb.NewSymDB(symdb.DefaultConfig().
		WithDirectory(filepath.Join(blockPath, symdb.DefaultDirName)).
		WithParquetConfig(symdb.ParquetConfig{
			MaxBufferRowCount: defaultParquetConfig.MaxBufferRowCount,
		}))

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

	// flush the index file.
	if err = indexw.Close(); err != nil {
		return block.Meta{}, err
	}

	if err = profileWriter.Close(); err != nil {
		return block.Meta{}, err
	}
	if err = symw.Flush(); err != nil {
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
	rewriters   map[BlockReader]*symdb.Rewriter
	stacktraces []uint32
	err         error

	numSamples uint64
}

func newSymbolsRewriter(it iter.Iterator[profileRow], blocks []BlockReader, w *symdb.SymDB) *symbolsRewriter {
	sr := symbolsRewriter{
		profiles:  it,
		rewriters: make(map[BlockReader]*symdb.Rewriter, len(blocks)),
	}
	for _, r := range blocks {
		sr.rewriters[r] = symdb.NewRewriter(w, r.Symbols())
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
