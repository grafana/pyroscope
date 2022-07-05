package firedb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	"github.com/segmentio/parquet-go"
)

type stringConversionTable []int64

func (t stringConversionTable) rewrite(idx *int64) {
	originalValue := int(*idx)
	newValue := t[originalValue]
	*idx = newValue
}

type idConversionTable map[int64]int64

func (t idConversionTable) rewrite(idx *int64) {
	pos := *idx
	*idx = t[pos]
}

func (t idConversionTable) rewriteUint64(idx *uint64) {
	pos := *idx
	*idx = uint64(t[int64(pos)])
}

type stringsHelper struct {
}

func (_ *stringsHelper) key(s string) string {
	return s
}

func (_ *stringsHelper) addToRewriter(r *rewriter, m idConversionTable) {
	r.strings = make(stringConversionTable, len(m))
	for x, y := range m {
		r.strings[x] = y
	}
}

func (_ *stringsHelper) rewrite(*rewriter, string) error {
	return nil
}

type functionsKey struct {
	Name       int64
	SystemName int64
	Filename   int64
	StartLine  int64
}

type functionsHelper struct {
}

func (_ *functionsHelper) key(f *profilev1.Function) functionsKey {
	return functionsKey{
		Name:       f.Name,
		SystemName: f.SystemName,
		Filename:   f.Filename,
		StartLine:  f.StartLine,
	}
}

func (_ *functionsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.functions = elemRewriter
}

func (_ *functionsHelper) rewrite(r *rewriter, f *profilev1.Function) error {
	r.strings.rewrite(&f.Filename)
	r.strings.rewrite(&f.Name)
	r.strings.rewrite(&f.SystemName)
	return nil
}

type mappingsHelper struct {
}

type mappingsKey struct {
	MemoryStart     uint64
	MemoryLimit     uint64
	FileOffset      uint64
	Filename        int64 // Index into string table
	BuildId         int64 // Index into string table
	HasFunctions    bool
	HasFilenames    bool
	HasLineNumbers  bool
	HasInlineFrames bool
}

func (_ *mappingsHelper) key(m *profilev1.Mapping) mappingsKey {
	return mappingsKey{
		MemoryStart:     m.MemoryStart,
		MemoryLimit:     m.MemoryLimit,
		FileOffset:      m.FileOffset,
		Filename:        m.Filename,
		BuildId:         m.BuildId,
		HasFunctions:    m.HasFunctions,
		HasFilenames:    m.HasFilenames,
		HasLineNumbers:  m.HasFunctions,
		HasInlineFrames: m.HasInlineFrames,
	}
}

func (_ *mappingsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.mappings = elemRewriter
}

func (_ *mappingsHelper) rewrite(r *rewriter, m *profilev1.Mapping) error {
	r.strings.rewrite(&m.Filename)
	r.strings.rewrite(&m.BuildId)
	return nil
}

type locationsKey struct {
	MappingId uint64
	Address   uint64
	LinesHash string
}

type locationsHelper struct {
}

func (_ *locationsHelper) key(l *profilev1.Location) locationsKey {
	return locationsKey{
		Address:   l.Address,
		MappingId: l.MappingId,
		LinesHash: "TODO", // TODO: Implement me to avoid crashes
	}
}

func (_ *locationsHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (_ *locationsHelper) rewrite(r *rewriter, l *profilev1.Location) error {
	r.mappings.rewriteUint64(&l.MappingId)

	for pos := range l.Line {
		r.functions.rewrite(&l.Line[pos].Line)
	}
	return nil
}

type samplesHelper struct {
}

func (_ *samplesHelper) key(s *Sample) samplesKey {
	id := s.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return samplesKey{
		ID: id,
	}

}

func (_ *samplesHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (_ *samplesHelper) rewrite(r *rewriter, s *Sample) error {

	for pos := range s.Types {
		r.strings.rewrite(&s.Types[pos].Type)
		r.strings.rewrite(&s.Types[pos].Unit)
	}

	for _, value := range s.Values {
		for pos := range value.LocationId {
			r.locations.rewriteUint64(&value.LocationId[pos])
		}
		for pos := range value.Label {
			r.strings.rewrite(&value.Label[pos].Key)
			r.strings.rewrite(&value.Label[pos].NumUnit)
			r.strings.rewrite(&value.Label[pos].Str)
		}
	}

	for pos := range s.Comment {
		r.strings.rewrite(&s.Comment[pos])
	}

	r.strings.rewrite(&s.DropFrames)
	r.strings.rewrite(&s.KeepFrames)

	return nil
}

type samplesKey struct {
	ID uuid.UUID
}

type Sample struct {
	// A unique UUID per ingested profile
	ID uuid.UUID `parquet:",dict"`

	// A description of the samples associated with each Sample.value.
	// For a cpu profile this might be:
	//   [["cpu","nanoseconds"]] or [["wall","seconds"]] or [["syscall","count"]]
	// For a heap profile, this might be:
	//   [["allocations","count"], ["space","bytes"]],
	// If one of the values represents the number of events represented
	// by the sample, by convention it should be at index 0 and use
	// sample_type.unit == "count".
	Types []*profilev1.ValueType `parquet:","`
	// The set of samples recorded in this profile.
	Values []*profilev1.Sample `parquet:","`

	// frames with Function.function_name fully matching the following
	// regexp will be dropped from the samples, along with their successors.
	DropFrames int64 `parquet:","` // Index into string table.
	// frames with Function.function_name fully matching the following
	// regexp will be kept, even if it matches drop_frames.
	KeepFrames int64 `parquet:","` // Index into string table.
	// Time of collection (UTC) represented as nanoseconds past the epoch.
	TimeNanos int64 `parquet:",delta"`
	// Duration of the profile, if a duration makes sense.
	DurationNanos int64 `parquet:",delta"`
	// The kind of events between sampled ocurrences.
	// e.g [ "cpu","cycles" ] or [ "heap","bytes" ]
	PeriodType *profilev1.ValueType `parquet:","`
	// The number of events between sampled occurrences.
	Period int64 `parquet:","`
	// Freeform text associated to the profile.
	Comment []int64 `parquet:","` // Indices into string table.
	// Index into the string table of the type of the preferred sample
	// value. If unset, clients should default to the last sample value.
	DefaultSampleType int64 `parquet:","`
}

type Models interface {
	*Sample | *profilev1.Location | *profilev1.Mapping | *profilev1.Function | string
}

type rewriter struct {
	strings   stringConversionTable
	functions idConversionTable
	mappings  idConversionTable
	locations idConversionTable
}

type Helper[M Models, K comparable] interface {
	key(M) K
	addToRewriter(*rewriter, idConversionTable)
	rewrite(*rewriter, M) error
}

type deduplicatingSlice[M Models, K comparable, H Helper[M, K]] struct {
	slice  []M
	lock   sync.RWMutex
	lookup map[K]int64
}

func (s *deduplicatingSlice[M, K, H]) init() {
	s.lookup = make(map[K]int64)
}

func (s *deduplicatingSlice[M, K, H]) ingest(ctx context.Context, elems []M, rewriter *rewriter) error {
	var (
		missing      []int64
		rewritingMap = make(map[int64]int64)
		h            H
	)

	// rewrite elements
	for pos := range elems {
		h.rewrite(rewriter, elems[pos])
	}

	// try to find if element already exists in slice
	s.lock.RLock()
	for pos := range elems {
		k := h.key(elems[pos])
		if posSlice, exists := s.lookup[k]; exists {
			rewritingMap[int64(pos)] = posSlice
		} else {
			missing = append(missing, int64(pos))
		}
	}
	s.lock.RUnlock()

	// if there are missing elements, acquire write lock
	if len(missing) > 0 {
		s.lock.Lock()
		var posSlice = int64(len(s.slice))
		for _, pos := range missing {
			// check again if element exists
			k := h.key(elems[pos])
			if posSlice, exists := s.lookup[k]; exists {
				rewritingMap[int64(pos)] = posSlice
				continue
			}

			// add element to slice/map
			s.slice = append(s.slice, elems[pos])
			s.lookup[k] = posSlice
			rewritingMap[int64(pos)] = posSlice
			posSlice++
		}
		s.lock.Unlock()
	}

	// add rewrite information to struct
	h.addToRewriter(rewriter, rewritingMap)

	return nil
}

type Head struct {
	logger log.Logger

	strings   deduplicatingSlice[string, string, *stringsHelper]
	mappings  deduplicatingSlice[*profilev1.Mapping, mappingsKey, *mappingsHelper]
	functions deduplicatingSlice[*profilev1.Function, functionsKey, *functionsHelper]
	locations deduplicatingSlice[*profilev1.Location, locationsKey, *locationsHelper]
	samples   deduplicatingSlice[*Sample, samplesKey, *samplesHelper]
}

func NewHead() *Head {
	h := &Head{
		logger: log.NewLogfmtLogger(os.Stderr),
	}
	h.strings.init()
	h.mappings.init()
	h.functions.init()
	h.locations.init()
	h.samples.init()
	return h
}

func (h *Head) Ingest(ctx context.Context, p *profilev1.Profile) error {
	rewrites := &rewriter{}

	if err := h.strings.ingest(ctx, p.StringTable, rewrites); err != nil {
		return err
	}

	if err := h.mappings.ingest(ctx, p.Mapping, rewrites); err != nil {
		return err
	}

	if err := h.functions.ingest(ctx, p.Function, rewrites); err != nil {
		return err
	}

	if err := h.locations.ingest(ctx, p.Location, rewrites); err != nil {
		return err
	}

	// TODO: Add ID and External Labels
	sample := &Sample{
		Values:            p.Sample,
		Types:             p.SampleType,
		DropFrames:        p.DropFrames,
		KeepFrames:        p.KeepFrames,
		TimeNanos:         p.TimeNanos,
		DurationNanos:     p.DurationNanos,
		PeriodType:        p.PeriodType,
		Period:            p.Period,
		Comment:           p.Comment,
		DefaultSampleType: p.DefaultSampleType,
	}

	if err := h.samples.ingest(ctx, []*Sample{sample}, rewrites); err != nil {
		return err
	}

	return nil
}

type table struct {
	name string
	rows []any
}

func (h *Head) WriteTo(ctx context.Context, path string) error {
	level.Info(h.logger).Log("msg", "write head to disk", "path", path)

	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !fileInfo.IsDir() {
		return fmt.Errorf("error %s is no directory", path)
	}

	if err := writeToFile(ctx, path, "samples", h.samples.slice); err != nil {
		return err
	}

	if err := writeToFile(ctx, path, "strings", stringSliceToRows(h.strings.slice)); err != nil {
		return err
	}

	if err := writeToFile(ctx, path, "mappings", h.mappings.slice); err != nil {
		return err
	}

	if err := writeToFile(ctx, path, "locations", h.locations.slice); err != nil {
		return err
	}

	if err := writeToFile(ctx, path, "functions", h.functions.slice); err != nil {
		return err
	}

	return nil
}

type stringRow struct {
	ID     uint64 `parquet:",delta"`
	String string `parquet:",dict"`
}

func stringSliceToRows(strs []string) []stringRow {
	rows := make([]stringRow, len(strs))
	for pos := range strs {
		rows[pos].ID = uint64(pos)
		rows[pos].String = strs[pos]
	}

	return rows
}

func writeToFile[T any](ctx context.Context, path string, table string, rows []T) error {
	file, err := os.OpenFile(filepath.Join(path, table+".parquet"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	/* TODO:
	buffer := parquet.NewGenericBuffer[RowType](
		parquet.SortingColumns(
			parquet.Ascending("LastName"),
			parquet.Ascending("FistName"),
		),
	)
	*/

	writerOptions := []parquet.WriterOption{}
	writer := parquet.NewGenericWriter[T](file, writerOptions...)
	if _, err := writer.Write(rows); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}

	return nil
}
