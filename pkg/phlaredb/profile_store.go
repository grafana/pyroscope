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

	"github.com/apache/arrow/go/v15/arrow"
	"github.com/apache/arrow/go/v15/arrow/array"
	"github.com/apache/arrow/go/v15/arrow/memory"
	"github.com/apache/arrow/go/v15/arrow/ipc"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/runutil"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util/build"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	writeBufferSize = 3 << 20 // 3MB
)

type Config struct {
	MaxBufferRowCount int  // Maximum number of rows to buffer before flushing
	MaxRowGroupBytes  uint64  // Maximum size of a row group in bytes
}

type headMetrics struct {
	sizeBytes                  *prometheus.GaugeVec
	samples                    prometheus.Gauge
	writtenProfileSegments    *prometheus.CounterVec
	writtenProfileSegmentsBytes prometheus.Histogram
}

type profilesIndex struct {
	mutex          sync.RWMutex
	profilesPerFP  map[uint64]*seriesWithProfiles
}

type seriesWithProfiles struct {
	lbs phlaremodel.Labels
	profiles []*schemav1.InMemoryProfile
}

type profileStore struct {
	size      atomic.Uint64
	totalSize atomic.Uint64

	logger  log.Logger
	cfg     *Config
	metrics *headMetrics

	path      string
	pool      memory.Allocator
	schema    *arrow.Schema
	writer    *ipc.FileWriter

	// lock serializes appends to the slice
	profilesLock sync.Mutex
	slice        []schemav1.InMemoryProfile

	// Rows lock synchronises access to the on-disk row groups
	rowsLock    sync.RWMutex
	rowsFlushed uint64
	rowGroups   []*rowGroupOnDisk
	index       *profilesIndex

	flushing       *atomic.Bool
	flushQueue     chan int
	closeOnce      sync.Once
	flushWg        sync.WaitGroup
	flushBuffer    []schemav1.InMemoryProfile
	flushBufferLbs []phlaremodel.Labels
	onFlush        func()
}

func newProfileStore(phlarectx context.Context) *profileStore {
	schema := createArrowSchema()
	
	s := &profileStore{
		logger:     phlarecontext.Logger(phlarectx),
		metrics:    contextHeadMetrics(phlarectx),
		pool:       memory.NewGoAllocator(),
		schema:     schema,
		flushing:   atomic.NewBool(false),
		flushQueue: make(chan int),
		index:      &profilesIndex{
			profilesPerFP: make(map[uint64]*seriesWithProfiles),
		},
	}
	
	s.flushWg.Add(1)
	go s.cutRowGroupLoop()
	
	return s
}

func createArrowSchema() *arrow.Schema {
	return arrow.NewSchema(
		[]arrow.Field{
			{Name: "ID", Type: arrow.BinaryType},
			{Name: "TimeNanos", Type: arrow.Int64Type},
			{Name: "SeriesFingerprint", Type: arrow.Uint64Type},
			{Name: "SeriesIndex", Type: arrow.Int32Type},
			// Add other fields based on your Profile structure
		},
		nil,
	)
}

// RowGroup represents a group of rows in Arrow format
type rowGroupOnDisk struct {
	reader     *ipc.FileReader
	file       *os.File
	numRows    int64
	seriesIndexes rowRangesWithSeriesIndex
}

func newRowGroupOnDisk(path string) (*rowGroupOnDisk, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "opening row groups segment file %s", path)
	}

	reader, err := ipc.NewFileReader(file)
	if err != nil {
		file.Close()
		return nil, errors.Wrapf(err, "creating Arrow reader for %s", path)
	}

	return &rowGroupOnDisk{
		reader: reader,
		file:   file,
		numRows: reader.NumRows(),
	}, nil
}

func (r *rowGroupOnDisk) Close() error {
	r.reader.Close()
	if err := r.file.Close(); err != nil {
		return err
	}

	if err := os.Remove(r.file.Name()); err != nil {
		return errors.Wrap(err, "deleting row group segment file")
	}

	return nil
}

// ArrowWriter implements the RowWriter interface for Arrow
type ArrowWriter struct {
	writer     *ipc.FileWriter
	builder    *array.RecordBuilder
	numRows    int64
}

func NewArrowWriter(schema *arrow.Schema, writer *ipc.FileWriter) *ArrowWriter {
	return &ArrowWriter{
		writer:  writer,
		builder: array.NewRecordBuilder(memory.NewGoAllocator(), schema),
	}
}

func (w *ArrowWriter) WriteRows(rows []schemav1.InMemoryProfile) error {
	// Build Arrow record from profiles
	for _, profile := range rows {
		// Add data to builders
		w.builder.Field(0).(*array.BinaryBuilder).Append(profile.ID[:])
		w.builder.Field(1).(*array.Int64Builder).Append(profile.TimeNanos)
		w.builder.Field(2).(*array.Uint64Builder).Append(profile.SeriesFingerprint)
		w.builder.Field(3).(*array.Int32Builder).Append(0) // SeriesIndex placeholder
	}

	record := w.builder.NewRecord()
	defer record.Release()

	if err := w.writer.Write(record); err != nil {
		return err
	}

	w.numRows += int64(len(rows))
	w.builder.Clear()
	return nil
}

func (w *ArrowWriter) Close() error {
	w.builder.Release()
	return w.writer.Close()
}

func (s *profileStore) Name() string {
	return "Arrow-based profile store"
}

func (s *profileStore) Size() uint64 {
	return s.totalSize.Load()
}

func (s *profileStore) MemorySize() uint64 {
	return s.size.Load()
}

// resets the store
func (s *profileStore) Init(path string, cfg *Config, metrics *headMetrics) (err error) {
	// close previous iteration
	if err := s.Close(); err != nil {
		return err
	}
	s.flushQueue = make(chan int)
	s.closeOnce = sync.Once{}
	s.flushWg.Add(1)
	go s.cutRowGroupLoop()

	// create index
	s.index, err = newProfileIndex(32, s.metrics)
	if err != nil {
		return err
	}

	s.path = path
	s.cfg = cfg
	s.metrics = metrics

	s.slice = s.slice[:0]

	s.rowsFlushed = 0

	return nil
}

func (s *profileStore) Close() error {
	if s.flushQueue != nil {
		s.closeOnce.Do(func() {
			close(s.flushQueue)
		})

		s.flushWg.Wait()
	}
	return nil
}

func (s *profileStore) RowGroups() []array.RecordReader {
	s.rowsLock.RLock()
	defer s.rowsLock.RUnlock()
	
	readers := make([]array.RecordReader, len(s.rowGroups))
	for i, rg := range s.rowGroups {
		readers[i] = rg.reader
	}
	return readers
}

// Flush writes row groups and the index to files on disk.
// The call is thread-safe for reading but adding new profiles
// should not be allowed during and after the call.
func (s *profileStore) Flush(ctx context.Context) (numRows uint64, numRowGroups uint64, err error) {
	if err := s.Close(); err != nil {
		return 0, 0, err
	}
	if err = s.cutRowGroup(len(s.slice)); err != nil {
		return 0, 0, err
	}

	indexPath := filepath.Join(
		s.path,
		block.IndexFilename,
	)

	rowRangerPerRG, err := s.index.writeTo(ctx, indexPath)
	if err != nil {
		return 0, 0, err
	}

	arrowPath := filepath.Join(
		s.path,
		s.Name()+".arrow",
	)
	s.rowsLock.Lock()
	for idx, ranges := range rowRangerPerRG {
		s.rowGroups[idx].seriesIndexes = ranges
	}
	s.rowsLock.Unlock()
	
	numRows, numRowGroups, err = s.writeRowGroups(arrowPath, s.rowGroups)
	if err != nil {
		return 0, 0, err
	}
	// Row groups are closed and removed on an explicit DeleteRowGroups call.
	return numRows, numRowGroups, nil
}

func (s *profileStore) DeleteRowGroups() error {
	s.rowsLock.Lock()
	defer s.rowsLock.Unlock()
	for _, rg := range s.rowGroups {
		if err := rg.Close(); err != nil {
			return err
		}
	}
	s.rowGroups = s.rowGroups[:0]
	return nil
}

func (s *profileStore) prepareFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, err
	}

	writer, err := ipc.NewFileWriter(file, ipc.WithSchema(s.schema))
	if err != nil {
		file.Close()
		return nil, err
	}
	
	s.writer = writer
	return file, nil
}

// cutRowGroups gets called, when a patrticular row group has been finished
// and it will flush it to disk. The caller of cutRowGroups should be holding
// the write lock.
//
// Writes are not allowed during cutting the rows, but readers are not blocked
// during the most of the time: only after the rows are written to disk do we
// block them for a short time (via rowsLock).
//
// TODO(kolesnikovae): Make the lock more selective. The call takes long time,
// if disk I/O is slow, which causes ingestion timeouts and impacts distributor
// push latency, and memory consumption, transitively.
// See index.cutRowGroup: we could find a way to not flush all the in-memory
// profiles, including ones added since the start of the call, but only those
// that were added before certain point (this call). The same for s.slice.
func (s *profileStore) cutRowGroup(count int) (err error) {
	defer func() {
		if err != nil {
			s.metrics.writtenProfileSegments.WithLabelValues("failed").Inc()
		}
	}()

	size := s.loadProfilesToFlush(count)
	if len(s.flushBuffer) == 0 {
		return nil
	}

	path := filepath.Join(
		s.path,
		fmt.Sprintf("%s.%d.arrow", s.Name(), s.rowsFlushed),
	)
	
	// Removes the file if it exists
	if err := os.Remove(path); err == nil {
		level.Warn(s.logger).Log("msg", "deleting row group segment of a failed previous attempt", "path", path)
	}

	f, err := s.prepareFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Create a new Arrow writer for this row group
	writer := NewArrowWriter(s.schema, s.writer)
	
	// Write the profiles
	if err := writer.WriteRows(s.flushBuffer); err != nil {
		return errors.Wrap(err, "write row group segments to disk")
	}

	if err := writer.Close(); err != nil {
		return errors.Wrap(err, "close row group segment writer")
	}

	s.metrics.writtenProfileSegments.WithLabelValues("success").Inc()

	// get row group segment size on disk
	if stat, err := f.Stat(); err == nil {
		s.metrics.writtenProfileSegmentsBytes.Observe(float64(stat.Size()))
	}

	rowGroup, err := newRowGroupOnDisk(path)
	if err != nil {
		return err
	}

	s.rowsLock.Lock()
	defer s.rowsLock.Unlock()
	
	s.rowsFlushed += uint64(writer.numRows)
	s.rowGroups = append(s.rowGroups, rowGroup)
	err = s.index.cutRowGroup(s.flushBuffer)

	s.profilesLock.Lock()
	defer s.profilesLock.Unlock()
	
	for i := range s.slice[:count] {
		s.metrics.samples.Sub(float64(len(s.slice[i].Samples.StacktraceIDs)))
	}
	
	s.slice = copySlice(s.slice[count:])
	currentSize := s.size.Sub(size)
	if err != nil {
		return err
	}

	level.Debug(s.logger).Log("msg", "cut row group segment", "path", path, "numProfiles", writer.numRows)
	s.metrics.sizeBytes.WithLabelValues(s.Name()).Set(float64(currentSize))
	return nil
}

type byLabels struct {
	p   []schemav1.InMemoryProfile
	lbs []phlaremodel.Labels
}

func (b byLabels) Len() int { return len(b.p) }
func (b byLabels) Swap(i, j int) {
	b.p[i], b.p[j] = b.p[j], b.p[i]
	b.lbs[i], b.lbs[j] = b.lbs[j], b.lbs[i]
}

func (by byLabels) Less(i, j int) bool {
	// first compare the labels, if they don't match return
	var (
		pI   = by.p[i]
		pJ   = by.p[j]
		lbsI = by.lbs[i]
		lbsJ = by.lbs[j]
	)
	if cmp := phlaremodel.CompareLabelPairs(lbsI, lbsJ); cmp != 0 {
		return cmp < 0
	}

	// then compare timenanos, if they don't match return
	if pI.TimeNanos < pJ.TimeNanos {
		return true
	} else if pI.TimeNanos > pJ.TimeNanos {
		return false
	}

	// finally use ID as tie breaker
	return bytes.Compare(pI.ID[:], pJ.ID[:]) < 0
}

// loadProfilesToFlush loads and sort profiles to flush into flushBuffer and returns the size of the profiles.
func (s *profileStore) loadProfilesToFlush(count int) uint64 {
	if cap(s.flushBuffer) < count {
		s.flushBuffer = make([]schemav1.InMemoryProfile, 0, count)
	}
	if cap(s.flushBufferLbs) < count {
		s.flushBufferLbs = make([]phlaremodel.Labels, 0, count)
	}
	s.flushBufferLbs = s.flushBufferLbs[:0]
	s.flushBuffer = s.flushBuffer[:0]
	s.profilesLock.Lock()
	s.index.mutex.RLock()
	for i := 0; i < count; i++ {
		profile := s.slice[i]
		s.flushBuffer = append(s.flushBuffer, profile)
		s.flushBufferLbs = append(s.flushBufferLbs, s.index.profilesPerFP[profile.SeriesFingerprint].lbs)
	}
	s.profilesLock.Unlock()
	s.index.mutex.RUnlock()
	// order profiles properly
	sort.Sort(byLabels{p: s.flushBuffer, lbs: s.flushBufferLbs})
	var size uint64
	for _, p := range s.flushBuffer {
		size += p.Size()
	}
	return size
}

// Update writeRowGroups to use the new interfaces
func (s *profileStore) writeRowGroups(path string, rowGroups []array.RecordReader) (n uint64, numRowGroups uint64, err error) {
	f, err := s.prepareFile(path)
	if err != nil {
		return 0, 0, err
	}
	defer runutil.CloseWithErrCapture(&err, f, "closing arrow file")

	writer := NewArrowWriter(s.schema, s.writer)
	defer writer.Close()

	// Create a record batch reader for each row group
	readers := make([]array.RecordReader, len(rowGroups))
	for i, rg := range rowGroups {
		readers[i] = rg
	}

	// Create a record batch reader that concatenates all row groups
	reader := array.NewConcatRecordReader(s.schema, readers)
	defer reader.Release()

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, err
		}

		// Write the record batch
		if err := s.writer.Write(record); err != nil {
			return 0, 0, err
		}
		n += uint64(record.NumRows())
		
		// Check if we need to start a new row group
		if n >= uint64(s.cfg.MaxBufferRowCount) {
			if err := s.writer.Close(); err != nil {
				return 0, 0, err
			}
			numRowGroups++
			
			// Create a new file writer for the next row group
			if err := s.writer.Reset(f); err != nil {
				return 0, 0, err
			}
		}
	}

	// Flush any remaining data as a final row group
	if n > 0 {
		if err := s.writer.Close(); err != nil {
			return 0, 0, err
		}
		numRowGroups++
	}

	s.rowsFlushed += n
	return n, numRowGroups, nil
}

func (s *profileStore) ingest(_ context.Context, profiles []schemav1.InMemoryProfile, lbs phlaremodel.Labels, profileName string) error {
	s.profilesLock.Lock()
	defer s.profilesLock.Unlock()

	for pos, p := range profiles {
		if !s.flushing.Load() {
			// check if row group is full
			if s.cfg.MaxBufferRowCount > 0 && len(s.slice) >= s.cfg.MaxBufferRowCount ||
				s.cfg.MaxRowGroupBytes > 0 && s.size.Load() >= s.cfg.MaxRowGroupBytes {
				s.flushing.Store(true)
				s.flushQueue <- len(s.slice)
			}
		}

		// add profile to the index
		s.index.Add(&p, lbs, profileName)

		// increase size of stored data
		addedBytes := profiles[pos].Size()
		s.metrics.sizeBytes.WithLabelValues(s.Name()).Set(float64(s.size.Add(addedBytes)))
		s.totalSize.Add(addedBytes)

		// add to slice
		s.slice = append(s.slice, p)
		s.metrics.samples.Add(float64(len(p.Samples.StacktraceIDs)))

	}

	return nil
}

func (s *profileStore) cutRowGroupLoop() {
	defer s.flushWg.Done()
	for n := range s.flushQueue {
		if err := s.cutRowGroup(n); err != nil {
			level.Error(s.logger).Log("msg", "cutting row group", "err", err)
		}
		s.flushing.Store(false)
		if s.onFlush != nil {
			s.onFlush()
		}
	}
}

func copySlice[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in)
	return out
}

// ProfileIterator provides iteration over profiles using Arrow
type ProfileIterator struct {
	reader     array.RecordReader
	record     arrow.Record
	currentRow int64
	totalRows  int64
}

func NewProfileIterator(reader array.RecordReader) (*ProfileIterator, error) {
	record, err := reader.Read()
	if err != nil && err != io.EOF {
		return nil, err
	}
	
	return &ProfileIterator{
		reader:     reader,
		record:     record,
		currentRow: 0,
		totalRows:  record.NumRows(),
	}, nil
}

func (it *ProfileIterator) Next() bool {
	if it.currentRow >= it.totalRows {
		// Try to read next record batch
		record, err := it.reader.Read()
		if err != nil {
			return false
		}
		
		if it.record != nil {
			it.record.Release()
		}
		
		it.record = record
		it.currentRow = 0
		it.totalRows = record.NumRows()
	}
	
	return it.currentRow < it.totalRows
}

func (it *ProfileIterator) At() *schemav1.InMemoryProfile {
	if it.record == nil {
		return nil
	}

	profile := &schemav1.InMemoryProfile{}
	
	// Extract values from Arrow columns
	idCol := it.record.Column(0).(*array.Binary)
	timeCol := it.record.Column(1).(*array.Int64)
	fpCol := it.record.Column(2).(*array.Uint64)
	
	copy(profile.ID[:], idCol.Value(int(it.currentRow)))
	profile.TimeNanos = timeCol.Value(int(it.currentRow))
	profile.SeriesFingerprint = fpCol.Value(int(it.currentRow))
	
	it.currentRow++
	return profile
}

func (it *ProfileIterator) Close() error {
	if it.record != nil {
		it.record.Release()
	}
	return it.reader.Release()
}

// QueryableProfileStore interface updates
type QueryableProfileStore interface {
	SelectProfiles(ctx context.Context, req *query.SelectProfilesRequest) (*ProfileIterator, error)
}

func (s *profileStore) SelectProfiles(ctx context.Context, req *query.SelectProfilesRequest) (*ProfileIterator, error) {
	s.rowsLock.RLock()
	defer s.rowsLock.RUnlock()

	// Create readers for each matching row group
	var readers []array.RecordReader
	
	// First check in-memory profiles
	if len(s.slice) > 0 {
		memReader, err := s.createMemoryReader(s.slice)
		if err != nil {
			return nil, err
		}
		readers = append(readers, memReader)
	}

	// Then check on-disk row groups
	for _, rg := range s.rowGroups {
		if matches := s.rowGroupMatchesQuery(rg, req); matches {
			readers = append(readers, rg.reader)
		}
	}

	if len(readers) == 0 {
		return NewProfileIterator(array.NewEmptyRecordReader(s.schema))
	}

	// Concatenate all readers
	reader := array.NewConcatRecordReader(s.schema, readers)
	return NewProfileIterator(reader)
}

// Helper function to create a record reader from in-memory profiles
func (s *profileStore) createMemoryReader(profiles []schemav1.InMemoryProfile) (array.RecordReader, error) {
	builder := array.NewRecordBuilder(s.pool, s.schema)
	defer builder.Release()

	for _, p := range profiles {
		builder.Field(0).(*array.BinaryBuilder).Append(p.ID[:])
		builder.Field(1).(*array.Int64Builder).Append(p.TimeNanos)
		builder.Field(2).(*array.Uint64Builder).Append(p.SeriesFingerprint)
		builder.Field(3).(*array.Int32Builder).Append(0) // SeriesIndex placeholder
	}

	record := builder.NewRecord()
	return array.NewRecordReader(s.schema, []arrow.Record{record})
}

// Helper function to check if a row group matches query criteria
func (s *profileStore) rowGroupMatchesQuery(rg *rowGroupOnDisk, req *query.SelectProfilesRequest) bool {
	// Implement filtering logic based on query requirements
	// For now, return true to include all row groups
	return true
}

// rowRangesWithSeriesIndex represents a range of rows associated with a series index
type rowRangesWithSeriesIndex struct {
	startRow    int64
	endRow      int64
	seriesIndex int32
}

// Helper function to create a new empty record reader with the store's schema
func (s *profileStore) newEmptyReader() array.RecordReader {
	return array.NewEmptyRecordReader(s.schema)
}

// Add missing newProfileIndex function
func newProfileIndex(size int, metrics *headMetrics) (*profilesIndex, error) {
	return &profilesIndex{
		mutex:          sync.RWMutex{},
		profilesPerFP:  make(map[uint64]*seriesWithProfiles, size),
	}, nil
}

// Add missing contextHeadMetrics function
func contextHeadMetrics(ctx context.Context) *headMetrics {
	reg := prometheus.NewRegistry()
	return &headMetrics{
		sizeBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "profile_store_size_bytes",
			Help: "Size of profile store in bytes",
		}, []string{"store"}),
		samples: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "profile_store_samples",
			Help: "Number of samples in profile store",
		}),
		writtenProfileSegments: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "profile_store_written_segments_total",
			Help: "Total number of profile segments written",
		}, []string{"status"}),
		writtenProfileSegmentsBytes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "profile_store_written_segments_bytes",
			Help:    "Size of written profile segments in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10),
		}),
	}
}

