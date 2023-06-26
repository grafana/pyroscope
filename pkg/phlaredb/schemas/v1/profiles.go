package v1

import (
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
)

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
