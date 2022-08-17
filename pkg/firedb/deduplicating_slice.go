package firedb

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/pkg/errors"
	"github.com/segmentio/parquet-go"
	"go.uber.org/atomic"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
)

var int64SlicePool = &sync.Pool{
	New: func() interface{} {
		return make([]int64, 0)
	},
}

type deduplicatingSlice[M Models, K comparable, H Helper[M, K], P schemav1.Persister[M]] struct {
	slice  []M
	size   atomic.Uint64
	lock   sync.RWMutex
	lookup map[K]int64

	persister P
	helper    H

	file   *os.File
	writer *parquet.Writer

	buffer      *parquet.Buffer
	rowsFlushed int
}

func (s *deduplicatingSlice[M, K, H, P]) Name() string {
	return s.persister.Name()
}

func (s *deduplicatingSlice[M, K, H, P]) Size() uint64 {
	return s.size.Load()
}

func (s *deduplicatingSlice[M, K, H, P]) Init(path string) error {
	file, err := os.OpenFile(filepath.Join(path, s.persister.Name()+".parquet"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	s.file = file

	// TODO: Reuse parquet.Writer beyond life time of the head.
	s.writer = parquet.NewWriter(file, s.persister.Schema(),
		parquet.ColumnPageBuffers(parquet.NewFileBufferPool(os.TempDir(), "firedb-parquet-buffers*")),
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

const maxRowGroupSize = 1000

func (s *deduplicatingSlice[M, K, H, P]) Flush() (int, error) {
	// intialise buffer if not existing
	if s.buffer == nil {
		s.buffer = parquet.NewBuffer(s.persister.Schema(), s.persister.SortingColumns())
	}

	s.lock.RLock()

	var rowsToFlush = len(s.slice) - s.rowsFlushed
	if rowsToFlush > maxRowGroupSize {
		rowsToFlush = maxRowGroupSize
	}

	if rowsToFlush == 0 {
		s.lock.RUnlock()
		return 0, nil
	}

	var rows = make([]parquet.Row, rowsToFlush)

	var slicePos int
	for pos := range rows {
		slicePos = pos + s.rowsFlushed
		rows[pos] = s.persister.Deconstruct(rows[pos], uint64(slicePos), s.slice[slicePos])
	}

	if _, err := s.buffer.WriteRows(rows); err != nil {
		s.lock.RUnlock()
		return 0, err
	}
	s.lock.RUnlock()

	sort.Sort(s.buffer)

	_, err := s.writer.WriteRowGroup(s.buffer)
	if err != nil {
		return 0, err
	}

	s.buffer.Reset()
	s.rowsFlushed += rowsToFlush

	// call myself recursively
	rowsFlushed, err := s.Flush()
	if err != nil {
		return 0, err
	}

	return rowsFlushed + rowsToFlush, nil
}

func (s *deduplicatingSlice[M, K, H, P]) ingest(ctx context.Context, elems []M, rewriter *rewriter) error {
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

	// try to find if element already exists in slice
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
			s.size.Add(s.helper.size(elems[pos]))
		}
		s.lock.Unlock()
	}

	int64SlicePool.Put(missing[:0])

	// add rewrite information to struct
	s.helper.addToRewriter(rewriter, rewritingMap)

	return nil
}

func (s *deduplicatingSlice[M, K, H, P]) getIndex(key K) (int64, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	v, ok := s.lookup[key]
	return v, ok
}
