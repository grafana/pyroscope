package phlaredb

import (
	"context"
	"math"
	"os"
	"path/filepath"

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
	Profiles() []parquet.RowGroup
	Index() IndexReader
	// Symbols() SymbolReader
}

type SymbolReader interface {
	// todo
}

func Compact(ctx context.Context, src []BlockReader, dst string) (block.Meta, error) {
	if len(src) <= 1 {
		return block.Meta{}, errors.New("not enough blocks to compact")
	}
	meta := block.NewMeta()
	blockPath := filepath.Join(dst, meta.ULID.String())
	if err := os.MkdirAll(blockPath, 0o777); err != nil {
		return block.Meta{}, err
	}
	indexPath := filepath.Join(blockPath, block.IndexFilename)
	indexw, err := prepareIndexWriter(ctx, indexPath, src)
	if err != nil {
		return block.Meta{}, err
	}
	profilePath := filepath.Join(blockPath, (&schemav1.ProfilePersister{}).Name()+block.ParquetSuffix)
	profileFile, err := os.OpenFile(profilePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return block.Meta{}, err
	}
	profileWriter := newProfileWriter(profileFile)

	// todo new symbdb

	rowsIt := newMergeRowProfileIterator(src)
	rowsIt = newSeriesRewriter(rowsIt, indexw)
	rowsIt = newSymbolsRewriter(rowsIt)
	reader := phlareparquet.NewIteratorRowReader(newRowsIterator(rowsIt))

	_, _, err = phlareparquet.CopyAsRowGroups(profileWriter, reader, defaultParquetConfig.MaxBufferRowCount)
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
	// todo: block meta
	if _, err := meta.WriteToFile(util.Logger, blockPath); err != nil {
		return block.Meta{}, err
	}
	return *meta, nil
}

type profileRow struct {
	timeNanos int64

	seriesRef uint32
	labels    phlaremodel.Labels
	fp        model.Fingerprint
	row       schemav1.ProfileRow
}

type profileRowIterator struct {
	profiles iter.Iterator[parquet.Row]
	index    IndexReader
	err      error

	currentRow profileRow
	chunks     []index.ChunkMeta
}

func newProfileRowIterator(profiles iter.Iterator[parquet.Row], idx IndexReader) *profileRowIterator {
	return &profileRowIterator{
		profiles: profiles,
		index:    idx,
		currentRow: profileRow{
			seriesRef: math.MaxUint32,
		},
		chunks: make([]index.ChunkMeta, 1),
	}
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
	fp, err := p.index.Series(storage.SeriesRef(p.currentRow.seriesRef), &p.currentRow.labels, &p.chunks)
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

func newMergeRowProfileIterator(src []BlockReader) iter.Iterator[profileRow] {
	its := make([]iter.Iterator[profileRow], len(src))
	for i, s := range src {
		// todo: may be we could merge rowgroups in parallel but that requires locking.
		reader := parquet.MultiRowGroup(s.Profiles()...).Rows()
		its[i] = newProfileRowIterator(
			phlareparquet.NewBufferedRowReaderIterator(reader, 1024),
			s.Index(),
		)
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
	}
}

type symbolsRewriter struct {
	iter.Iterator[profileRow]
}

// todo remap symbols & ingest symbols
func newSymbolsRewriter(it iter.Iterator[profileRow]) *symbolsRewriter {
	return &symbolsRewriter{
		Iterator: it,
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
}

func newSeriesRewriter(it iter.Iterator[profileRow], indexw *index.Writer) *seriesRewriter {
	return &seriesRewriter{
		Iterator: it,
		indexw:   indexw,
	}
}

func (s *seriesRewriter) Next() bool {
	if !s.Iterator.Next() {
		if s.previousFp != 0 {
			if err := s.indexw.AddSeries(s.seriesRef, s.labels, s.previousFp, s.currentChunkMeta); err != nil {
				s.err = err
				return false
			}
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
