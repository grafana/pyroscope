package v1

import (
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/segmentio/parquet-go"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	phlareparquet "github.com/grafana/phlare/pkg/parquet"
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
	}
	profilesSchema = parquet.NewSchema("Profile", phlareparquet.Group{
		phlareparquet.NewGroupField("ID", parquet.UUID()),
		phlareparquet.NewGroupField("SeriesIndex", parquet.Encoded(parquet.Uint(32), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField("StacktracePartition", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
		phlareparquet.NewGroupField("Samples", parquet.List(sampleField)),
		phlareparquet.NewGroupField("DropFrames", parquet.Optional(stringRef)),
		phlareparquet.NewGroupField("KeepFrames", parquet.Optional(stringRef)),
		phlareparquet.NewGroupField("TimeNanos", parquet.Timestamp(parquet.Nanosecond)),
		phlareparquet.NewGroupField("DurationNanos", parquet.Optional(parquet.Int(64))),
		phlareparquet.NewGroupField("Period", parquet.Optional(parquet.Int(64))),
		phlareparquet.NewGroupField("Comments", parquet.List(stringRef)),
		phlareparquet.NewGroupField("DefaultSampleType", parquet.Optional(parquet.Int(64))),
	})

	maxProfileRow       parquet.Row
	seriesIndexColIndex int
	timeNanoColIndex    int
)

func init() {
	maxProfileRow = deconstructMemoryProfile(InMemoryProfile{
		SeriesIndex: math.MaxUint32,
		TimeNanos:   math.MaxInt64,
	}, maxProfileRow)
	seriesCol, ok := profilesSchema.Lookup("SeriesIndex")
	if !ok {
		panic(fmt.Errorf("SeriesIndex index column not found"))
	}
	seriesIndexColIndex = seriesCol.ColumnIndex
	timeCol, ok := profilesSchema.Lookup("TimeNanos")
	if !ok {
		panic(fmt.Errorf("TimeNanos column not found"))
	}
	timeNanoColIndex = timeCol.ColumnIndex
}

type Sample struct {
	StacktraceID uint64             `parquet:",delta"`
	Value        int64              `parquet:",delta"`
	Labels       []*profilev1.Label `parquet:",list"`
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
	return profilesSchema
}

func (*ProfilePersister) SortingColumns() parquet.SortingOption {
	return parquet.SortingColumns(
		parquet.Ascending("SeriesIndex"),
		parquet.Ascending("TimeNanos"),
		parquet.Ascending("Samples", "list", "element", "StacktraceID"),
	)
}

func (*ProfilePersister) Deconstruct(row parquet.Row, id uint64, s *Profile) parquet.Row {
	row = profilesSchema.Deconstruct(row, s)
	return row
}

func (*ProfilePersister) Reconstruct(row parquet.Row) (id uint64, s *Profile, err error) {
	var profile Profile
	if err := profilesSchema.Reconstruct(&profile, row); err != nil {
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
			return profilesSchema.Deconstruct(r, p)
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
		// serialize the row
		rows[pos] = r.serialize(p, rows[pos])
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
}

// Compact zero samples and optionally duplicates.
func (s Samples) Compact(dedupe bool) Samples {
	if len(s.StacktraceIDs) == 0 {
		return s
	}
	if dedupe {
		s = trimDuplicateSamples(s)
	}
	s = trimZeroSamples(s)
	return s
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

func trimZeroSamples(samples Samples) Samples {
	n := 0
	for j, v := range samples.Values {
		if v != 0 {
			samples.Values[n] = v
			samples.StacktraceIDs[n] = samples.StacktraceIDs[j]
			n++
		}
	}
	return Samples{
		StacktraceIDs: samples.StacktraceIDs[:n],
		Values:        samples.Values[:n],
	}
}

func cloneSamples(samples Samples) Samples {
	return Samples{
		StacktraceIDs: copySlice(samples.StacktraceIDs),
		Values:        copySlice(samples.Values),
	}
}

func (s Samples) Less(i, j int) bool {
	return s.StacktraceIDs[i] < s.StacktraceIDs[j]
}

func (s Samples) Swap(i, j int) {
	s.StacktraceIDs[i], s.StacktraceIDs[j] = s.StacktraceIDs[j], s.StacktraceIDs[i]
	s.Values[i], s.Values[j] = s.Values[j], s.Values[i]
}

func (s Samples) Len() int {
	return len(s.StacktraceIDs)
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
		totalCols = 8 + (6 * len(imp.Samples.StacktraceIDs)) + len(imp.Comments)
	)
	if cap(row) < totalCols {
		row = make(parquet.Row, 0, totalCols)
	}
	row = row[:0]
	row = append(row, parquet.FixedLenByteArrayValue(imp.ID[:]).Level(0, 0, newCol()))
	row = append(row, parquet.Int32Value(int32(imp.SeriesIndex)).Level(0, 0, newCol()))
	row = append(row, parquet.Int64Value(int64(imp.StacktracePartition)).Level(0, 0, newCol()))
	newCol()
	if len(imp.Samples.Values) == 0 {
		row = append(row, parquet.Value{}.Level(0, 0, col))
	}
	repetition := -1
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
