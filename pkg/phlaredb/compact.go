package phlaredb

import (
	"context"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/oklog/ulid"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/segmentio/parquet-go"

	"github.com/grafana/phlare/pkg/iter"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	phlareparquet "github.com/grafana/phlare/pkg/parquet"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/phlaredb/tsdb/index"
	"github.com/grafana/phlare/pkg/util"
	"github.com/grafana/phlare/pkg/util/loser"
)

type BlockReader interface {
	Meta() block.Meta
	Profiles() []parquet.RowGroup
	Index() IndexReader
	// todo symbdb
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

	// todo new symbdb

	rowsIt, err := newMergeRowProfileIterator(src)
	if err != nil {
		return block.Meta{}, err
	}
	seriesRewriter := newSeriesRewriter(rowsIt, indexw)
	symbolsRewriter := newSymbolsRewriter(seriesRewriter)
	reader := phlareparquet.NewIteratorRowReader(newRowsIterator(symbolsRewriter))

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

// todo implement and tests
func metaFilesFromDir(dir string) ([]block.File, error) {
	var files []block.File
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		switch filepath.Ext(info.Name()) {
		case strings.TrimPrefix(block.ParquetSuffix, "."):
			// todo parquet file
		case filepath.Ext(block.IndexFilename):
			// todo tsdb index file
		default:
			// todo other files
		}
		return nil
	})
	return files, err
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
}

type profileRowIterator struct {
	profiles    iter.Iterator[parquet.Row]
	index       IndexReader
	allPostings index.Postings
	err         error

	currentRow profileRow
	chunks     []index.ChunkMeta
}

func newProfileRowIterator(reader parquet.RowReader, idx IndexReader) (*profileRowIterator, error) {
	k, v := index.AllPostingsKey()
	allPostings, err := idx.Postings(k, nil, v)
	if err != nil {
		return nil, err
	}
	return &profileRowIterator{
		profiles:    phlareparquet.NewBufferedRowReaderIterator(reader, 1024),
		index:       idx,
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
		it, err := newProfileRowIterator(
			reader,
			s.Index(),
		)
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

type noopStacktraceRewriter struct{}

func (noopStacktraceRewriter) RewriteStacktraces(src, dst []uint32) error {
	copy(dst, src)
	return nil
}

type StacktraceRewriter interface {
	RewriteStacktraces(src, dst []uint32) error
}

type symbolsRewriter struct {
	iter.Iterator[profileRow]
	err error

	rewriter   StacktraceRewriter
	src, dst   []uint32
	numSamples uint64
}

// todo remap symbols & ingest symbols
func newSymbolsRewriter(it iter.Iterator[profileRow]) *symbolsRewriter {
	return &symbolsRewriter{
		Iterator: it,
		rewriter: noopStacktraceRewriter{},
	}
}

func (s *symbolsRewriter) NumSamples() uint64 {
	return s.numSamples
}

func (s *symbolsRewriter) Next() bool {
	if !s.Iterator.Next() {
		return false
	}
	var err error
	s.Iterator.At().row.ForStacktraceIDsValues(func(values []parquet.Value) {
		s.numSamples += uint64(len(values))
		s.loadStacktracesID(values)
		err = s.rewriter.RewriteStacktraces(s.src, s.dst)
		if err != nil {
			return
		}
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

func (s *symbolsRewriter) Err() error {
	if s.err != nil {
		return s.err
	}
	return s.Iterator.Err()
}

func (s *symbolsRewriter) loadStacktracesID(values []parquet.Value) {
	if cap(s.src) < len(values) {
		s.src = make([]uint32, len(values)*2)
		s.dst = make([]uint32, len(values)*2)
	}
	s.src = s.src[:len(values)]
	s.dst = s.dst[:len(values)]
	for i := range values {
		s.src[i] = values[i].Uint32()
	}
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
