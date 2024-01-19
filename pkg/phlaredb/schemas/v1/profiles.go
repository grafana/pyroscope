package v1

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"unsafe"

	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlareparquet "github.com/grafana/pyroscope/pkg/parquet"
)

const (
	SeriesIndexColumnName         = "SeriesIndex"
	TimeNanosColumnName           = "TimeNanos"
	StacktracePartitionColumnName = "StacktracePartition"
	TotalValueColumnName          = "TotalValue"
	SamplesColumnName             = "Samples"
)

var (
	stringRef   = parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)
	pprofLabels = parquet.List(phlareparquet.Group{
		phlareparquet.NewGroupField("Key", stringRef),
		phlareparquet.NewGroupField("Str", parquet.Optional(stringRef)),
		phlareparquet.NewGroupField("Num", parquet.Optional(parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked))),
		phlareparquet.NewGroupField("NumUnit", parquet.Optional(stringRef)),
	})
	sampleField = phlareparquet.Group{
		phlareparquet.NewGroupField("StacktraceID", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField("Value", parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField("Labels", pprofLabels),
		phlareparquet.NewGroupField("SpanID", parquet.Optional(parquet.Encoded(parquet.Uint(64), &parquet.RLEDictionary))),
	}
	ProfilesSchema = parquet.NewSchema("Profile", phlareparquet.Group{
		phlareparquet.NewGroupField("ID", parquet.UUID()),
		phlareparquet.NewGroupField(SeriesIndexColumnName, parquet.Encoded(parquet.Uint(32), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField(StacktracePartitionColumnName, parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField(TotalValueColumnName, parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField(SamplesColumnName, parquet.List(sampleField)),
		phlareparquet.NewGroupField("DropFrames", parquet.Optional(stringRef)),
		phlareparquet.NewGroupField("KeepFrames", parquet.Optional(stringRef)),
		phlareparquet.NewGroupField(TimeNanosColumnName, parquet.Timestamp(parquet.Nanosecond)),
		phlareparquet.NewGroupField("DurationNanos", parquet.Optional(parquet.Int(64))),
		phlareparquet.NewGroupField("Period", parquet.Optional(parquet.Int(64))),
		phlareparquet.NewGroupField("Comments", parquet.List(stringRef)),
		phlareparquet.NewGroupField("DefaultSampleType", parquet.Optional(parquet.Int(64))),
	})
	DownsampledProfilesSchema = parquet.NewSchema("DownsampledProfile", phlareparquet.Group{
		phlareparquet.NewGroupField(SeriesIndexColumnName, parquet.Encoded(parquet.Uint(32), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField(StacktracePartitionColumnName, parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField(TotalValueColumnName, parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField(SamplesColumnName, parquet.List(
			phlareparquet.Group{
				phlareparquet.NewGroupField("StacktraceID", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
				phlareparquet.NewGroupField("Value", parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)),
			})),
		phlareparquet.NewGroupField(TimeNanosColumnName, parquet.Timestamp(parquet.Nanosecond)),
	})

	sampleStacktraceIDColumnPath = strings.Split("Samples.list.element.StacktraceID", ".")
	SampleValueColumnPath        = strings.Split("Samples.list.element.Value", ".")
	sampleSpanIDColumnPath       = strings.Split("Samples.list.element.SpanID", ".")

	maxProfileRow               parquet.Row
	seriesIndexColIndex         int
	stacktraceIDColIndex        int
	valueColIndex               int
	timeNanoColIndex            int
	stacktracePartitionColIndex int

	downsampledValueColIndex int

	ErrColumnNotFound = fmt.Errorf("column path not found")
)

func init() {
	maxProfileRow = deconstructMemoryProfile(InMemoryProfile{
		SeriesIndex: math.MaxUint32,
		TimeNanos:   math.MaxInt64,
	}, maxProfileRow)
	seriesCol, ok := ProfilesSchema.Lookup(SeriesIndexColumnName)
	if !ok {
		panic(fmt.Errorf("SeriesIndex index column not found"))
	}
	seriesIndexColIndex = seriesCol.ColumnIndex
	timeCol, ok := ProfilesSchema.Lookup(TimeNanosColumnName)
	if !ok {
		panic(fmt.Errorf("TimeNanos column not found"))
	}
	timeNanoColIndex = timeCol.ColumnIndex
	stacktraceIDCol, ok := ProfilesSchema.Lookup(sampleStacktraceIDColumnPath...)
	if !ok {
		panic(fmt.Errorf("StacktraceID column not found"))
	}
	stacktraceIDColIndex = stacktraceIDCol.ColumnIndex
	valueCol, ok := ProfilesSchema.Lookup(SampleValueColumnPath...)
	if !ok {
		panic(fmt.Errorf("Sample.Value column not found"))
	}
	valueColIndex = valueCol.ColumnIndex
	stacktracePartitionCol, ok := ProfilesSchema.Lookup(StacktracePartitionColumnName)
	if !ok {
		panic(fmt.Errorf("StacktracePartition column not found"))
	}
	stacktracePartitionColIndex = stacktracePartitionCol.ColumnIndex

	downsampledValueCol, ok := DownsampledProfilesSchema.Lookup(SampleValueColumnPath...)
	if !ok {
		panic(fmt.Errorf("Sample.Value column not found"))
	}
	downsampledValueColIndex = downsampledValueCol.ColumnIndex
}

type SampleColumns struct {
	StacktraceID parquet.LeafColumn
	Value        parquet.LeafColumn
	SpanID       parquet.LeafColumn
}

func (c *SampleColumns) Resolve(schema *parquet.Schema) error {
	var err error
	if c.StacktraceID, err = ResolveColumnByPath(schema, sampleStacktraceIDColumnPath); err != nil {
		return err
	}
	if c.Value, err = ResolveColumnByPath(schema, SampleValueColumnPath); err != nil {
		return err
	}
	// Optional.
	c.SpanID, _ = ResolveColumnByPath(schema, sampleSpanIDColumnPath)
	return nil
}

func (c *SampleColumns) HasSpanID() bool {
	return c.SpanID.Node != nil
}

func ResolveColumnByPath(schema *parquet.Schema, path []string) (parquet.LeafColumn, error) {
	if c, ok := schema.Lookup(path...); ok {
		return c, nil
	}
	return parquet.LeafColumn{}, fmt.Errorf("%w: %v", ErrColumnNotFound, path)
}

type Sample struct {
	StacktraceID uint64             `parquet:",delta"`
	Value        int64              `parquet:",delta"`
	Labels       []*profilev1.Label `parquet:",list"`
	SpanID       uint64             `parquet:",optional"`
}

type Profile struct {
	// A unique UUID per ingested profile
	ID uuid.UUID `parquet:",uuid"`

	// SeriesIndex references the underlying series and is generated when
	// writing the TSDB index. The SeriesIndex is different from block to
	// block.
	SeriesIndex uint32 `parquet:",delta"`

	// StacktracePartition is the partition ID of the stacktrace table that this profile belongs to.
	StacktracePartition uint64 `parquet:",delta"`

	// TotalValue is the sum of all values in the profile.
	TotalValue uint64 `parquet:",delta"`

	// SeriesFingerprint references the underlying series and is purely based
	// on the label values. The value is consistent for the same label set (so
	// also between different blocks).
	SeriesFingerprint model.Fingerprint `parquet:"-"`

	// The set of samples recorded in this profile.
	Samples []*Sample `parquet:",list"`

	// frames with Function.function_name fully matching the following
	// regexp will be dropped from the samples, along with their successors.
	DropFrames int64 `parquet:",optional"` // Index into string table.
	// frames with Function.function_name fully matching the following
	// regexp will be kept, even if it matches drop_frames.
	KeepFrames int64 `parquet:",optional"` // Index into string table.
	// Time of collection (UTC) represented as nanoseconds past the epoch.
	TimeNanos int64 `parquet:",delta,timestamp(nanosecond)"`
	// Duration of the profile, if a duration makes sense.
	DurationNanos int64 `parquet:",delta,optional"`
	// The number of events between sampled occurrences.
	Period int64 `parquet:",optional"`
	// Freeform text associated to the profile.
	Comments []int64 `parquet:",list"` // Indices into string table.
	// Index into the string table of the type of the preferred sample
	// value. If unset, clients should default to the last sample value.
	DefaultSampleType int64 `parquet:",optional"`
}

func (p Profile) Timestamp() model.Time {
	return model.TimeFromUnixNano(p.TimeNanos)
}

func (p Profile) Total() int64 {
	var total int64
	for _, sample := range p.Samples {
		total += sample.Value
	}
	return total
}

type ProfilePersister struct{}

func (*ProfilePersister) Name() string {
	return "profiles"
}

func (*ProfilePersister) Schema() *parquet.Schema {
	return ProfilesSchema
}

func (*ProfilePersister) SortingColumns() parquet.SortingOption {
	return parquet.SortingColumns(
		parquet.Ascending("SeriesIndex"),
		parquet.Ascending("TimeNanos"),
		parquet.Ascending("Samples", "list", "element", "StacktraceID"),
	)
}

func (*ProfilePersister) Deconstruct(row parquet.Row, id uint64, s *Profile) parquet.Row {
	row = ProfilesSchema.Deconstruct(row, s)
	return row
}

func (*ProfilePersister) Reconstruct(row parquet.Row) (id uint64, s *Profile, err error) {
	var profile Profile
	if err := ProfilesSchema.Reconstruct(&profile, row); err != nil {
		return 0, nil, err
	}
	return 0, &profile, nil
}

type SliceRowReader[T any] struct {
	slice     []T
	serialize func(T, parquet.Row) parquet.Row
}

func NewProfilesRowReader(slice []*Profile) *SliceRowReader[*Profile] {
	return &SliceRowReader[*Profile]{
		slice: slice,
		serialize: func(p *Profile, r parquet.Row) parquet.Row {
			return ProfilesSchema.Deconstruct(r, p)
		},
	}
}

func (r *SliceRowReader[T]) ReadRows(rows []parquet.Row) (n int, err error) {
	if len(r.slice) == 0 {
		return 0, io.EOF
	}
	if len(rows) > len(r.slice) {
		rows = rows[:len(r.slice)]
		err = io.EOF
	}
	for pos, p := range r.slice[:len(rows)] {
		// Serialize the row. Note that the row may
		// be already initialized and contain values,
		// therefore it must be reset.
		row := rows[pos][:0]
		rows[pos] = r.serialize(p, row)
		n++
	}
	r.slice = r.slice[len(rows):]
	return n, err
}

type InMemoryProfile struct {
	// A unique UUID per ingested profile
	ID uuid.UUID

	// SeriesIndex references the underlying series and is generated when
	// writing the TSDB index. The SeriesIndex is different from block to
	// block.
	SeriesIndex uint32

	// StacktracePartition is the partition ID of the stacktrace table that this profile belongs to.
	StacktracePartition uint64

	// TotalValue is the sum of all values in the profile.
	TotalValue uint64

	// SeriesFingerprint references the underlying series and is purely based
	// on the label values. The value is consistent for the same label set (so
	// also between different blocks).
	SeriesFingerprint model.Fingerprint

	// frames with Function.function_name fully matching the following
	// regexp will be dropped from the samples, along with their successors.
	DropFrames int64
	// frames with Function.function_name fully matching the following
	// regexp will be kept, even if it matches drop_frames.
	KeepFrames int64
	// Time of collection (UTC) represented as nanoseconds past the epoch.
	TimeNanos int64
	// Duration of the profile, if a duration makes sense.
	DurationNanos int64
	// The number of events between sampled occurrences.
	Period int64
	// Freeform text associated to the profile.
	Comments []int64
	// Index into the string table of the type of the preferred sample
	// value. If unset, clients should default to the last sample value.
	DefaultSampleType int64

	Samples Samples
}

type Samples struct {
	StacktraceIDs []uint32
	Values        []uint64
	// Span associated with samples.
	// Optional: Spans == nil, if not present.
	Spans []uint64
}

func NewSamples(size int) Samples {
	return Samples{
		StacktraceIDs: make([]uint32, 0, size),
		Values:        make([]uint64, 0, size),
	}
}

func NewSamplesFromMap(m map[uint32]int64) Samples {
	s := Samples{
		StacktraceIDs: make([]uint32, len(m)),
		Values:        make([]uint64, len(m)),
	}
	var i int
	for k, v := range m {
		s.StacktraceIDs[i] = k
		s.Values[i] = uint64(v)
		i++
	}
	sort.Sort(s)
	return s
}

// Compact zero samples and optionally duplicates.
func (s Samples) Compact(dedupe bool) Samples {
	if len(s.StacktraceIDs) == 0 {
		return s
	}
	if dedupe {
		s = trimDuplicateSamples(s)
	}
	return trimZeroAndNegativeSamples(s)
}

func (s Samples) Clone() Samples {
	return cloneSamples(s)
}

func trimDuplicateSamples(samples Samples) Samples {
	sort.Sort(samples)
	n := 0
	for j := 1; j < len(samples.StacktraceIDs); j++ {
		if samples.StacktraceIDs[n] == samples.StacktraceIDs[j] {
			samples.Values[n] += samples.Values[j]
		} else {
			n++
			samples.StacktraceIDs[n] = samples.StacktraceIDs[j]
			samples.Values[n] = samples.Values[j]
		}
	}
	return Samples{
		StacktraceIDs: samples.StacktraceIDs[:n+1],
		Values:        samples.Values[:n+1],
	}
}

func trimZeroAndNegativeSamples(samples Samples) Samples {
	n := 0
	for j, v := range samples.Values {
		if v > 0 {
			samples.Values[n] = v
			samples.StacktraceIDs[n] = samples.StacktraceIDs[j]
			if len(samples.Spans) > 0 {
				samples.Spans[n] = samples.Spans[j]
			}
			n++
		}
	}
	s := Samples{
		StacktraceIDs: samples.StacktraceIDs[:n],
		Values:        samples.Values[:n],
	}
	if len(samples.Spans) > 0 {
		s.Spans = samples.Spans[:n]
	}
	return s
}

func cloneSamples(samples Samples) Samples {
	return Samples{
		StacktraceIDs: copySlice(samples.StacktraceIDs),
		Values:        copySlice(samples.Values),
		Spans:         copySlice(samples.Spans),
	}
}

func (s Samples) Less(i, j int) bool {
	return s.StacktraceIDs[i] < s.StacktraceIDs[j]
}

func (s Samples) Swap(i, j int) {
	s.StacktraceIDs[i], s.StacktraceIDs[j] = s.StacktraceIDs[j], s.StacktraceIDs[i]
	s.Values[i], s.Values[j] = s.Values[j], s.Values[i]
	if len(s.Spans) > 0 {
		s.Spans[i], s.Spans[j] = s.Spans[j], s.Spans[i]
	}
}

func (s Samples) Len() int {
	return len(s.StacktraceIDs)
}

type SamplesBySpanID Samples

func (s SamplesBySpanID) Less(i, j int) bool {
	return s.Spans[i] < s.Spans[j]
}

func (s SamplesBySpanID) Swap(i, j int) {
	s.StacktraceIDs[i], s.StacktraceIDs[j] = s.StacktraceIDs[j], s.StacktraceIDs[i]
	s.Values[i], s.Values[j] = s.Values[j], s.Values[i]
	if len(s.Spans) > 0 {
		s.Spans[i], s.Spans[j] = s.Spans[j], s.Spans[i]
	}
}

func (s SamplesBySpanID) Len() int {
	return len(s.Spans)
}

func (s Samples) Sum() uint64 {
	var sum uint64
	for _, v := range s.Values {
		sum += v
	}
	return sum
}

// TODO(kolesnikovae): Consider map alternatives.

// SampleMap is a map of partitioned samples structured
// as follows: partition => stacktrace_id => value
type SampleMap map[uint64]map[uint32]int64

func (m SampleMap) Partition(p uint64) map[uint32]int64 {
	s, ok := m[p]
	if !ok {
		s = make(map[uint32]int64, 128)
		m[p] = s
	}
	return s
}

func (m SampleMap) AddSamples(partition uint64, samples Samples) {
	p := m.Partition(partition)
	for i, sid := range samples.StacktraceIDs {
		p[sid] += int64(samples.Values[i])
	}
}

func (m SampleMap) WriteSamples(partition uint64, dst *Samples) {
	p, ok := m[partition]
	if !ok {
		return
	}
	dst.StacktraceIDs = dst.StacktraceIDs[:0]
	dst.Values = dst.Values[:0]
	for k, v := range p {
		dst.StacktraceIDs = append(dst.StacktraceIDs, k)
		dst.Values = append(dst.Values, uint64(v))
	}
}

const profileSize = uint64(unsafe.Sizeof(InMemoryProfile{}))

func (p InMemoryProfile) Size() uint64 {
	size := profileSize + uint64(cap(p.Comments)*8)
	// 4 bytes for stacktrace id and 8 bytes for each stacktrace value
	return size + uint64(cap(p.Samples.StacktraceIDs)*(4+8))
}

func (p InMemoryProfile) Timestamp() model.Time {
	return model.TimeFromUnixNano(p.TimeNanos)
}

func (p InMemoryProfile) Total() int64 {
	var total int64
	for _, sample := range p.Samples.Values {
		total += int64(sample)
	}
	return total
}

func copySlice[T any](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func NewInMemoryProfilesRowReader(slice []InMemoryProfile) *SliceRowReader[InMemoryProfile] {
	return &SliceRowReader[InMemoryProfile]{
		slice:     slice,
		serialize: deconstructMemoryProfile,
	}
}

func deconstructMemoryProfile(imp InMemoryProfile, row parquet.Row) parquet.Row {
	var (
		col    = -1
		newCol = func() int {
			col++
			return col
		}
		totalCols = 8 + (7 * len(imp.Samples.StacktraceIDs)) + len(imp.Comments)
	)
	if cap(row) < totalCols {
		row = make(parquet.Row, 0, totalCols)
	}
	row = row[:0]
	row = append(row, parquet.FixedLenByteArrayValue(imp.ID[:]).Level(0, 0, newCol()))
	row = append(row, parquet.Int32Value(int32(imp.SeriesIndex)).Level(0, 0, newCol()))
	row = append(row, parquet.Int64Value(int64(imp.StacktracePartition)).Level(0, 0, newCol()))
	row = append(row, parquet.Int64Value(int64(imp.TotalValue)).Level(0, 0, newCol()))

	newCol()
	repetition := -1
	if len(imp.Samples.Values) == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, col))
	}
	for i := range imp.Samples.StacktraceIDs {
		if repetition < 1 {
			repetition++
		}
		row = append(row, parquet.Int64Value(int64(imp.Samples.StacktraceIDs[i])).Level(repetition, 1, col))
	}

	newCol()
	repetition = -1
	if len(imp.Samples.Values) == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, col))
	}
	for i := range imp.Samples.Values {
		if repetition < 1 {
			repetition++
		}
		row = append(row, parquet.Int64Value(int64(imp.Samples.Values[i])).Level(repetition, 1, col))
	}

	for i := 0; i < 4; i++ {
		newCol()
		repetition := -1
		if len(imp.Samples.Values) == 0 {
			row = append(row, parquet.Value{}.Level(0, 0, col))
		}
		for range imp.Samples.Values {
			if repetition < 1 {
				repetition++
			}
			row = append(row, parquet.Value{}.Level(repetition, 1, col))
		}
	}

	newCol()
	repetition = -1
	if len(imp.Samples.Spans) == 0 {
		// Fill the row with empty entries (one per value).
		if len(imp.Samples.Values) == 0 {
			row = append(row, parquet.Value{}.Level(0, 0, col))
		}
		for range imp.Samples.Values {
			if repetition < 1 {
				repetition++
			}
			row = append(row, parquet.Value{}.Level(repetition, 1, col))
		}
	} else {
		for i := range imp.Samples.Spans {
			if repetition < 1 {
				repetition++
			}
			row = append(row, parquet.Int64Value(int64(imp.Samples.Spans[i])).Level(repetition, 2, col))
		}
	}

	if imp.DropFrames == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, newCol()))
	} else {
		row = append(row, parquet.Int64Value(imp.DropFrames).Level(0, 1, newCol()))
	}
	if imp.KeepFrames == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, newCol()))
	} else {
		row = append(row, parquet.Int64Value(imp.KeepFrames).Level(0, 1, newCol()))
	}
	row = append(row, parquet.Int64Value(imp.TimeNanos).Level(0, 0, newCol()))
	if imp.DurationNanos == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, newCol()))
	} else {
		row = append(row, parquet.Int64Value(imp.DurationNanos).Level(0, 1, newCol()))
	}
	if imp.Period == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, newCol()))
	} else {
		row = append(row, parquet.Int64Value(imp.Period).Level(0, 1, newCol()))
	}
	newCol()
	if len(imp.Comments) == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, col))
	}
	repetition = -1
	for i := range imp.Comments {
		if repetition < 1 {
			repetition++
		}
		row = append(row, parquet.Int64Value(imp.Comments[i]).Level(repetition, 1, col))
	}
	if imp.DefaultSampleType == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, newCol()))
	} else {
		row = append(row, parquet.Int64Value(imp.DefaultSampleType).Level(0, 1, newCol()))
	}
	return row
}

func NewMergeProfilesRowReader(rowGroups []parquet.RowReader) parquet.RowReader {
	if len(rowGroups) == 0 {
		return phlareparquet.EmptyRowReader
	}
	return phlareparquet.NewMergeRowReader(rowGroups, maxProfileRow, lessProfileRows)
}

func lessProfileRows(r1, r2 parquet.Row) bool {
	// We can directly lookup the series index column and compare it
	// because it's after only fixed length column
	sv1, sv2 := r1[seriesIndexColIndex].Uint32(), r2[seriesIndexColIndex].Uint32()
	if sv1 != sv2 {
		return sv1 < sv2
	}
	// we need to find the TimeNanos column and compare it
	// but it's after repeated columns, so we search from the end to avoid
	// going through samples
	var ts1, ts2 int64
	for i := len(r1) - 1; i >= 0; i-- {
		if r1[i].Column() == timeNanoColIndex {
			ts1 = r1[i].Int64()
			break
		}
	}
	for i := len(r2) - 1; i >= 0; i-- {
		if r2[i].Column() == timeNanoColIndex {
			ts2 = r2[i].Int64()
			break
		}
	}
	return ts1 < ts2
}

type ProfileRow parquet.Row

func (p ProfileRow) SeriesIndex() uint32 {
	return p[seriesIndexColIndex].Uint32()
}

func (p ProfileRow) StacktracePartitionID() uint64 {
	return p[stacktracePartitionColIndex].Uint64()
}

func (p ProfileRow) TimeNanos() int64 {
	var ts int64
	for i := len(p) - 1; i >= 0; i-- {
		if p[i].Column() == timeNanoColIndex {
			ts = p[i].Int64()
			break
		}
	}
	return ts
}

func (p ProfileRow) SetSeriesIndex(v uint32) {
	p[seriesIndexColIndex] = parquet.Int32Value(int32(v)).Level(0, 0, seriesIndexColIndex)
}

func (p ProfileRow) ForStacktraceIDsValues(fn func([]parquet.Value)) {
	start := -1
	var i int
	for i = 0; i < len(p); i++ {
		col := p[i].Column()
		if col == stacktraceIDColIndex && p[i].DefinitionLevel() == 1 {
			if start == -1 {
				start = i
			}
		}
		if col > stacktraceIDColIndex {
			break
		}
	}
	if start != -1 {
		fn(p[start:i])
	}
}

func (p ProfileRow) ForStacktraceIdsAndValues(fn func([]parquet.Value, []parquet.Value)) {
	startStacktraces := -1
	endStacktraces := -1
	startValues := -1
	endValues := -1
	var i int
	for i = 0; i < len(p); i++ {
		col := p[i].Column()
		if col == stacktraceIDColIndex && p[i].DefinitionLevel() == 1 {
			if startStacktraces == -1 {
				startStacktraces = i
			}
		}
		if col > stacktraceIDColIndex && endStacktraces == -1 {
			endStacktraces = i
		}
		if col == valueColIndex && p[i].DefinitionLevel() == 1 {
			if startValues == -1 {
				startValues = i
			}
		}
		if col > valueColIndex && endValues == -1 {
			endValues = i
			break
		}
	}
	if startStacktraces != -1 && startValues != -1 {
		fn(p[startStacktraces:endStacktraces], p[startValues:endValues])
	}
}

type DownsampledProfileRow parquet.Row

func (p DownsampledProfileRow) ForValues(fn func([]parquet.Value)) {
	start := -1
	var i int
	for i = 0; i < len(p); i++ {
		col := p[i].Column()
		if col == downsampledValueColIndex && p[i].DefinitionLevel() == 1 {
			if start == -1 {
				start = i
			}
		}
		if col > downsampledValueColIndex {
			break
		}
	}
	if start != -1 {
		fn(p[start:i])
	}
}
