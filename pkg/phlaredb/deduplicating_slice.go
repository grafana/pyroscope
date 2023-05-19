package phlaredb

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/pkg/errors"
	"github.com/segmentio/parquet-go"
	"go.uber.org/atomic"

	"github.com/grafana/phlare/pkg/phlaredb/block"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/util/build"
)

var int64SlicePool = &sync.Pool{
	New: func() interface{} {
		return make([]int64, 0)
	},
}

var defaultParquetConfig = &ParquetConfig{
	MaxBufferRowCount: 100_000,
	MaxRowGroupBytes:  10 * 128 * 1024 * 1024,
	MaxBlockBytes:     10 * 10 * 128 * 1024 * 1024,
}

type deduplicatingSlice[M Models, K comparable, H Helper[M, K], P schemav1.Persister[M]] struct {
	slice  []M
	size   atomic.Uint64
	lock   sync.RWMutex
	lookup map[K]int64

	persister P
	helper    H

	file    *os.File
	cfg     *ParquetConfig
	metrics *headMetrics
	writer  *parquet.GenericWriter[P]

	rowsFlushed int
}

func (s *deduplicatingSlice[M, K, H, P]) Name() string {
	return s.persister.Name()
}

func (s *deduplicatingSlice[M, K, H, P]) MemorySize() uint64 {
	return s.size.Load()
}

func (s *deduplicatingSlice[M, K, H, P]) Size() uint64 {
	return s.size.Load()
}

func (s *deduplicatingSlice[M, K, H, P]) Init(path string, cfg *ParquetConfig, metrics *headMetrics) error {
	s.cfg = cfg
	s.metrics = metrics
	file, err := os.OpenFile(filepath.Join(path, s.persister.Name()+block.ParquetSuffix), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	s.file = file

	// TODO: Reuse parquet.Writer beyond life time of the head.
	s.writer = parquet.NewGenericWriter[P](file, s.persister.Schema(),
		parquet.ColumnPageBuffers(parquet.NewFileBufferPool(os.TempDir(), "phlaredb-parquet-buffers*")),
		parquet.CreatedBy("github.com/grafana/phlare/", build.Version, build.Revision),
	)
	s.lookup = make(map[K]int64)
	return nil
}

func (s *deduplicatingSlice[M, K, H, P]) Close() error {
	if err := s.writer.Close(); err != nil {
		return errors.Wrap(err, "closing parquet writer")
	}

	if err := s.file.Close(); err != nil {
		return errors.Wrap(err, "closing parquet file")
	}

	return nil
}

func (s *deduplicatingSlice[M, K, H, P]) maxRowsPerRowGroup() int {
	// with empty slice we need to return early
	if len(s.slice) == 0 {
		return 1
	}

	var (
		// average size per row in memory
		bytesPerRow = s.Size() / uint64(len(s.slice))

		// how many rows per RG with average size are fitting in the maxRowGroupBytes, ensure that we at least flush 1 row
		maxRows = s.cfg.MaxRowGroupBytes / bytesPerRow
	)

	if maxRows <= 0 {
		return 1
	}

	return int(maxRows)
}

func (s *deduplicatingSlice[M, K, H, P]) Flush(ctx context.Context) (numRows uint64, numRowGroups uint64, err error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	buffer := parquet.NewBuffer(
		s.persister.Schema(),
		parquet.SortingRowGroupConfig(s.persister.SortingColumns()),
		parquet.ColumnBufferCapacity(s.cfg.MaxBufferRowCount),
	)

	var (
		maxRows = s.maxRowsPerRowGroup()

		rowGroupsFlushed int
		rowsFlushed      int
	)

	for {
		// how many rows of the head still in need of flushing
		rowsToFlush := len(s.slice) - s.rowsFlushed

		if rowsToFlush == 0 {
			break
		}

		// cap max row size by bytes
		if rowsToFlush > maxRows {
			rowsToFlush = maxRows
		}
		// cap max row size by buffer
		if rowsToFlush > s.cfg.MaxBufferRowCount {
			rowsToFlush = s.cfg.MaxBufferRowCount
		}

		rows := make([]parquet.Row, rowsToFlush)
		var slicePos int
		for pos := range rows {
			slicePos = pos + s.rowsFlushed
			rows[pos] = s.persister.Deconstruct(rows[pos], uint64(slicePos), s.slice[slicePos])
		}

		buffer.Reset()
		if _, err = buffer.WriteRows(rows); err != nil {
			return 0, 0, err
		}

		sort.Sort(buffer)

		if _, err = s.writer.WriteRowGroup(buffer); err != nil {
			return 0, 0, err
		}

		s.rowsFlushed += rowsToFlush
		rowsFlushed += rowsToFlush
		rowGroupsFlushed++
	}

	return uint64(rowsFlushed), uint64(rowGroupsFlushed), nil
}

func (s *deduplicatingSlice[M, K, H, P]) ingest(_ context.Context, elems []M, rewriter *rewriter) error {
	var (
		rewritingMap = make(map[int64]int64)
		missing      = int64SlicePool.Get().([]int64)
	)

	// rewrite elements
	for pos := range elems {
		if err := s.helper.rewrite(rewriter, elems[pos]); err != nil {
			return err
		}
	}

	// try to find if element already exists in slice, when supposed to depduplicate
	s.lock.RLock()
	for pos := range elems {
		k := s.helper.key(elems[pos])
		if posSlice, exists := s.lookup[k]; exists {
			rewritingMap[int64(s.helper.setID(uint64(pos), uint64(posSlice), elems[pos]))] = posSlice
		} else {
			missing = append(missing, int64(pos))
		}
	}
	s.lock.RUnlock()

	// if there are missing elements, acquire write lock
	if len(missing) > 0 {
		s.lock.Lock()
		posSlice := int64(len(s.slice))
		for _, pos := range missing {
			// check again if element exists
			k := s.helper.key(elems[pos])
			if posSlice, exists := s.lookup[k]; exists {
				rewritingMap[int64(s.helper.setID(uint64(pos), uint64(posSlice), elems[pos]))] = posSlice
				continue
			}

			// add element to slice/map
			s.slice = append(s.slice, s.helper.clone(elems[pos]))
			s.lookup[k] = posSlice
			rewritingMap[int64(s.helper.setID(uint64(pos), uint64(posSlice), elems[pos]))] = posSlice
			posSlice++

			// increase size of stored data
			s.metrics.sizeBytes.WithLabelValues(s.Name()).Set(float64(s.size.Add(s.helper.size(elems[pos]))))
		}
		s.lock.Unlock()
	}

	// nolint staticcheck
	int64SlicePool.Put(missing[:0])

	// add rewrite information to struct
	s.helper.addToRewriter(rewriter, rewritingMap)

	return nil
}
