package phlaredb

import (
	"bytes"
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
	"github.com/prometheus/common/model"
	"github.com/segmentio/parquet-go"
	"go.uber.org/atomic"

	phlaremodel "github.com/grafana/phlare/pkg/model"
	phlarecontext "github.com/grafana/phlare/pkg/phlare/context"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/util/build"
)

type profileGetter struct {
	// first preference a direct profile reference
	p *schemav1.Profile

	// second try read from row group
	rowGroup *rowGroupOnDisk
	rowNum   int64
}

func (pg *profileGetter) Get() *schemav1.Profile {
	if pg.p != nil {
		return pg.p
	}

	if pg.rowGroup == nil {
		return nil
	}

	reader := parquet.NewGenericRowGroupReader[*schemav1.Profile](pg.rowGroup)
	if err := reader.SeekToRow(pg.rowNum - 1); err != nil {
		// TODO: Log error here
		panic(fmt.Errorf("unable to seek row_num=%d from %s: %w", pg.rowNum, pg.rowGroup.file.Name(), err))
	}

	profiles := make([]*schemav1.Profile, 1)
	if _, err := reader.Read(profiles); err != nil {
		// TODO: Log error here
		panic(fmt.Errorf("unable to get row_num=%d from %s: %w", pg.rowNum, pg.rowGroup.file.Name(), err))
	}

	return profiles[0]
}

type profileStore struct {
	slice   []*schemav1.Profile
	getters []*profileGetter
	size    atomic.Uint64
	lock    sync.RWMutex

	metrics *headMetrics

	index *profilesIndex

	persister schemav1.Persister[*schemav1.Profile]
	helper    storeHelper[*schemav1.Profile]

	logger log.Logger
	cfg    *ParquetConfig

	writer *parquet.GenericWriter[*schemav1.Profile]

	path        string
	rowsFlushed uint64

	rowGroups []*rowGroupOnDisk
}

func newProfileStore(phlarectx context.Context, cfg *ParquetConfig) *profileStore {
	var s = &profileStore{
		logger:    phlarecontext.Logger(phlarectx),
		metrics:   contextHeadMetrics(phlarectx),
		cfg:       cfg,
		persister: &schemav1.ProfilePersister{},
		helper:    &profilesHelper{},
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

func (s *profileStore) Size() uint64 {
	return s.size.Load()
}

// resets the store
func (s *profileStore) Init(path string, cfg *ParquetConfig) (err error) {
	// close previous iteration
	if err := s.Close(); err != nil {
		return err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// create index
	s.index, err = newProfileIndex(32, s.metrics)
	if err != nil {
		return err
	}

	s.path = path
	s.cfg = cfg

	s.getters = s.getters[:0]
	s.slice = s.slice[:0]

	s.rowsFlushed = 0

	return nil
}

func (s *profileStore) Close() error {
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

	if err := s.cutRowGroup(); err != nil {
		return 0, 0, err
	}

	path := filepath.Join(
		s.path,
		s.persister.Name()+block.ParquetSuffix,
	)

	// TODO: Somewhere here we need to make sure the SeriesIndex gets set
	// prepare series id rewritter
	colSeriesIndex, found := s.persister.Schema().Lookup("SeriesIndex")
	if !found {
		return 0, 0, errors.New("SeriesIndex column not found")
	}
	colSeriesFingerprint, found := s.persister.Schema().Lookup("SeriesFingerprint")
	if !found {
		return 0, 0, errors.New("SeriesFingerprint column not found")
	}

	seriesIDRowRewritter := func(rows []parquet.Row) error {
		for _, r := range rows {
			fp := model.Fingerprint(r[colSeriesFingerprint.ColumnIndex].Uint64())
			seriesIndex, found := s.index.seriesIndexes[fp]
			if !found {
				return errors.Errorf("series with fingerprint %d not found in the index, make sure index has already been written", fp)
			}

			// update series index column for each row
			r[colSeriesIndex.ColumnIndex] = parquet.ValueOf(seriesIndex).Level(0, 0, colSeriesIndex.ColumnIndex)

		}
		return nil
	}
	for _, rowGroup := range s.rowGroups {
		rowGroup.seriesIDRowRewriter = seriesIDRowRewritter
	}

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

func (s *profileStore) prepareFile(path string) (closer io.Closer, err error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}
	s.writer.Reset(file)

	return file, err
}

// cutRowGroups gets called, when a patrticular row group has been finished and it will flush it to disk. The caller of cutRowGroups should be holding the write lock.
// TODO: write row groups asynchronously
func (s *profileStore) cutRowGroup() (err error) {
	// do nothing with empty buffer
	bufferRowNums := len(s.slice)
	if bufferRowNums == 0 {
		return nil
	}

	path := filepath.Join(
		s.path,
		fmt.Sprintf("%s.%d%s", s.persister.Name(), s.rowsFlushed, block.ParquetSuffix),
	)

	fileCloser, err := s.prepareFile(path)
	if err != nil {
		return err
	}
	defer runutil.CloseWithErrCapture(&err, fileCloser, "closing row group segment file")

	// note: the slice is already in the right order because it is getting sorted during ingest

	n, err := s.writer.Write(s.slice)
	if err != nil {
		return errors.Wrap(err, "write row group segments to disk")
	}

	if err := s.writer.Close(); err != nil {
		return errors.Wrap(err, "write row group segments to disk")
	}

	s.rowsFlushed += uint64(n)

	rowGroup, err := newRowGroupOnDisk(path)
	if err != nil {
		return err
	}
	s.rowGroups = append(s.rowGroups, rowGroup)

	// Upgrade profile getters to point to correct location
	for pos, pg := range s.getters {
		// TODO: Reuse the profile struct eventually by using a pool
		pg.p = nil

		pg.rowNum = int64(pos) + 1
		pg.rowGroup = rowGroup
	}

	s.slice = s.slice[:0]
	s.getters = s.getters[:0]
	s.size.Store(0)

	return nil
}

func (s *profileStore) writeRowGroups(path string, rowGroups []parquet.RowGroup) (n uint64, numRowGroups uint64, err error) {
	fileCloser, err := s.prepareFile(path)
	if err != nil {
		return 0, 0, err
	}
	defer runutil.CloseWithErrCapture(&err, fileCloser, "closing parquet file")

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

func (s *profileStore) ingest(_ context.Context, profiles []*schemav1.Profile, lbs phlaremodel.Labels, profileName string, rewriter *rewriter) error {

	var (
		getters = make([]*profileGetter, len(profiles))
	)

	for pos := range profiles {
		// rewrite elements
		if err := s.helper.rewrite(rewriter, profiles[pos]); err != nil {
			return err
		}

		// prepare getters
		getters[pos] = &profileGetter{p: profiles[pos]}
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// do a binary search where to insert each profile
	for pos, p := range profiles {
		// check if row group is full
		if s.cfg.MaxBufferRowCount > 0 && len(s.slice) >= s.cfg.MaxBufferRowCount ||
			s.cfg.MaxRowGroupBytes > 0 && s.size.Load() >= s.cfg.MaxRowGroupBytes {
			if err := s.cutRowGroup(); err != nil {
				return err
			}
		}

		// add profile to the index
		s.index.addWithGetter(getters[pos], lbs, profileName)

		// increase size of stored data
		s.size.Add(s.helper.size(profiles[pos]))

		// find correct position in the slice using binary search. The order needs to match the one from the schema
		posProfile, _ := sort.Find(len(s.slice), func(i int) int {
			// first compare the labels, if they don't match return
			if cmp := phlaremodel.CompareLabelPairs(lbs, s.index.profilesPerFP[s.slice[i].SeriesFingerprint].lbs); cmp != 0 {
				return cmp
			}

			// then compare timenanos, if they don't match return
			if s.slice[i].TimeNanos < p.TimeNanos {
				return 1
			} else if s.slice[i].TimeNanos > p.TimeNanos {
				return -1
			}

			// finally use ID as tie breaker
			return bytes.Compare(p.ID[:], s.slice[i].ID[:])
		})

		// insert at the end of the slices
		if len(s.slice) == posProfile {
			s.slice = append(s.slice, p)
			s.getters = append(s.getters, getters[pos])
			continue
		}

		// make room at the correct position
		s.slice = append(s.slice[:posProfile+1], s.slice[posProfile:]...)
		s.getters = append(s.getters[:posProfile+1], s.getters[posProfile:]...)

		s.slice[posProfile] = p
		s.getters[posProfile] = getters[pos]

	}

	return nil
}

func (s *profileStore) NumRows() int64 {
	return int64(len(s.slice)) + int64(s.rowsFlushed)
}

type rowGroupOnDisk struct {
	parquet.RowGroup
	file                *os.File
	seriesIDRowRewriter func([]parquet.Row) error
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

func (r *rowGroupOnDisk) Rows() parquet.Rows {
	return seriesIDRowsRewriter{
		Rows:             r.RowGroup.Rows(),
		seriesIDRewriter: r.seriesIDRowRewriter,
	}
}

type seriesIDRowsRewriter struct {
	parquet.Rows
	seriesIDRewriter func([]parquet.Row) error
}

func (r seriesIDRowsRewriter) ReadRows(rows []parquet.Row) (int, error) {
	n, err := r.Rows.ReadRows(rows)
	if r.seriesIDRewriter == nil || err != nil {
		return n, err
	}

	if err := r.seriesIDRewriter(rows[:n]); err != nil {
		return 0, err
	}

	return n, nil
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
