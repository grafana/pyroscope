package phlaredb

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/runutil"
	"github.com/pkg/errors"
	"github.com/segmentio/parquet-go"
	"go.uber.org/atomic"

	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/util/build"
)

type profileGetter struct {
	p *schemav1.Profile
}

func (pg *profileGetter) Get() *schemav1.Profile {
	if pg.p != nil {
		return pg.p
	}

	return nil
}

type profileStore struct {
	slice   []*schemav1.Profile
	getters []*profileGetter
	size    atomic.Uint64
	lock    sync.RWMutex

	index *profilesIndex

	persister schemav1.Persister[*schemav1.Profile]
	helper    storeHelper[*schemav1.Profile]

	logger log.Logger
	cfg    *ParquetConfig

	writer *parquet.GenericWriter[*schemav1.Profile]
	buffer *parquet.GenericBuffer[*schemav1.Profile]

	path        string
	rowsFlushed uint64

	rowGroups []*rowGroupOnDisk
}

func newProfileStore(phlarectx context.Context, cfg *ParquetConfig, index *profilesIndex) *profileStore {
	var s = &profileStore{
		logger:    phlarecontext.Logger(phlarectx),
		cfg:       cfg,
		persister: &schemav1.ProfilePersister{},
		helper:    &profilesHelper{},
		index:     index,
	}

	// Initialize writer on /dev/null
	// TODO: Reuse parquet.Writer beyond life time of the head.
	s.writer = parquet.NewGenericWriter[*schemav1.Profile](io.Discard, s.persister.Schema(),
		parquet.ColumnPageBuffers(parquet.NewFileBufferPool(os.TempDir(), "phlaredb-parquet-buffers*")),
		parquet.CreatedBy("github.com/grafana/phlare/", build.Version, build.Revision),
	)

	return s
}

func (s *profileStore) Name() string {
	return s.persister.Name()
}

func (s *profileStore) Size() int64 {
	return int64(s.size.Load())
}

// Starts the ingestion loop.
func (s *profileStore) Reset(path string) error {
	// close previous iteration
	if err := s.Close(); err != nil {
		return err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.path = path

	s.rowsFlushed = 0

	return nil
}

func (s *profileStore) Close() error {
	return nil
}

func copyRowGroupsFromFile(path string, writer parquet.RowGroupWriter) error {
	sourceFile, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "opening row groups segment file %s", path)
	}
	defer runutil.CloseWithErrCapture(&err, sourceFile, "closing row groups segment file %s", path)

	stats, err := sourceFile.Stat()
	if err != nil {
		return errors.Wrapf(err, "getting stat of row groups segment file %s", path)
	}

	sourceParquet, err := parquet.OpenFile(sourceFile, stats.Size())
	if err != nil {
		return errors.Wrapf(err, "reading parquet of row groups segment file %s", path)
	}

	for pos, rg := range sourceParquet.RowGroups() {
		_, err := writer.WriteRowGroup(rg)
		if err != nil {
			return errors.Wrapf(err, "writing row group %d of row groups segment file %s", pos, path)
		}

	}

	sourceParquet.RowGroups()
	return nil
}

func (s *profileStore) RowGroups() (rowGroups []parquet.RowGroup) {
	rowGroups = make([]parquet.RowGroup, len(s.rowGroups))
	for pos := range rowGroups {
		rowGroups[pos] = s.rowGroups[pos]
	}
	return rowGroups
}

func (s *profileStore) Flush() (numRows uint64, numRowGroups uint64, err error) {
	// close ingest loop
	if err := s.Close(); err != nil {
		return 0, 0, err
	}

	if _, err := s.cutRowGroup(); err != nil {
		return 0, 0, err
	}

	path := filepath.Join(
		s.path,
		s.persister.Name()+block.ParquetSuffix,
	)

	numRows, numRowGroups, err = s.writeRowGroups(path, s.RowGroups())
	if err != nil {
		return 0, 0, err
	}

	// cleanup row groups, which need cleaning
	for _, rg := range s.rowGroups {
		if err := rg.Close(); err != nil {
			return 0, 0, err
		}
	}
	s.rowGroups = s.rowGroups[:0]

	return numRows, numRowGroups, nil
}

// cutRowGroups gets called, when a patrticular row group has been finished and it will flush it to dosk.
// TODO: call writeRowGroups asynchronously
func (s *profileStore) cutRowGroup() (n uint64, err error) {
	// do nothing with empty buffer
	bufferRowNums := s.buffer.NumRows()
	if bufferRowNums == 0 {
		return 0, nil
	}

	// sort the buffer
	sort.Sort(s.buffer)

	path := filepath.Join(
		s.path,
		fmt.Sprintf("%s.%d%s", s.persister.Name(), s.rowsFlushed, block.ParquetSuffix),
	)

	n, _, err = s.writeRowGroups(path, []parquet.RowGroup{s.buffer})
	if err != nil {
		return n, errors.Wrap(err, "write row group segment to disk")
	}

	rowGroup, err := newRowGroupOnDisk(path)
	if err != nil {
		return 0, err
	}
	s.rowGroups = append(s.rowGroups, rowGroup)

	s.buffer.Reset()

	return n, nil
}

func (s *profileStore) writeRowGroups(path string, rowGroups []parquet.RowGroup) (n uint64, numRowGroups uint64, err error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, 0, err
	}
	defer runutil.CloseWithErrCapture(&err, file, "failed to close rowGroup file")
	s.writer.Reset(file)

	for rgN, rg := range rowGroups {
		level.Debug(s.logger).Log("msg", "writing row group", "path", path, "row_group_number", rgN, "rows", rg.NumRows())

		nInt64, err := s.writer.WriteRowGroup(rg)
		if err != nil {
			return 0, 0, err
		}
		n += uint64(nInt64)
		numRowGroups += 1
	}

	if err := s.writer.Close(); err != nil {
		return 0, 0, err
	}

	s.rowsFlushed += n

	return n, numRowGroups, nil
}

func (s *profileStore) ingest(ctx context.Context, elems []*schemav1.Profile, rewriter *rewriter) error {

	// rewrite elements
	for pos := range elems {
		if err := s.helper.rewrite(rewriter, elems[pos]); err != nil {
			return err
		}

		s.getters = append(s.getters, &profileGetter{p: elems[pos]})

		s.index.addWithGetter(elems[pos].GetId(), s.getters[len(s.getters)-1])
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.slice = append(s.slice, elems...)

	s.index

	// TODO: Increase the size
	return nil
}

func (s *profileStore) NumRows() int64 {
	return s.buffer.NumRows() + int64(s.rowsFlushed)
}

func (s *profileStore) Root() *parquet.Column {
	panic("Root() is not implemented for head store")
	return nil
}

func (s *profileStore) Schema() *parquet.Schema {
	return s.persister.Schema()
}

type rowGroupOnDisk struct {
	parquet.RowGroup
	file *os.File
}

func newRowGroupOnDisk(path string) (*rowGroupOnDisk, error) {
	var (
		r   = &rowGroupOnDisk{}
		err error
	)

	// now open the row group file, so we are able to read the row group back in
	r.file, err = os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "opening row groups segment file %s", path)
	}

	stats, err := r.file.Stat()
	if err != nil {
		return nil, errors.Wrapf(err, "getting stat of row groups segment file %s", path)
	}

	segmentParquet, err := parquet.OpenFile(r.file, stats.Size())
	if err != nil {
		return nil, errors.Wrapf(err, "reading parquet of row groups segment file %s", path)
	}

	rowGroups := segmentParquet.RowGroups()
	if len(rowGroups) != 1 {
		return nil, errors.Wrapf(err, "segement file expected to have exactly one row group (actual %d)", len(rowGroups))
	}

	r.RowGroup = rowGroups[0]

	return r, nil

}

func (r *rowGroupOnDisk) Close() error {
	if err := r.file.Close(); err != nil {
		return err
	}

	if err := os.Remove(r.file.Name()); err != nil {
		return errors.Wrap(err, "deleting row group segment file")
	}

	return nil
}
