package v1

import (
	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/segmentio/parquet-go"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	fireparquet "github.com/grafana/fire/pkg/parquet"
)

var (
	stringRef   = parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)
	pprofLabels = parquet.Repeated(fireparquet.Group{
		fireparquet.NewGroupField("Key", stringRef),
		fireparquet.NewGroupField("Str", parquet.Optional(stringRef)),
		fireparquet.NewGroupField("Num", parquet.Optional(parquet.Int(64))),
		fireparquet.NewGroupField("NumUnit", parquet.Optional(stringRef)),
	})
	sampleField = fireparquet.Group{
		fireparquet.NewGroupField("ProfileID", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
		fireparquet.NewGroupField("Values", parquet.Repeated(parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked))),
		fireparquet.NewGroupField("Labels", pprofLabels),
	}
	profilesSchema = parquet.NewSchema("Profile", fireparquet.Group{
		fireparquet.NewGroupField("ID", parquet.UUID()),
		fireparquet.NewGroupField("SeriesRefs", parquet.Repeated(parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked))),
		fireparquet.NewGroupField("Samples", parquet.Repeated(sampleField)),
		fireparquet.NewGroupField("DropFrames", stringRef),
		fireparquet.NewGroupField("KeepFrames", stringRef),
		fireparquet.NewGroupField("TimeNanos", parquet.Timestamp(parquet.Nanosecond)),
		fireparquet.NewGroupField("DurationNanos", parquet.Int(64)),
		fireparquet.NewGroupField("Period", parquet.Int(64)),
		fireparquet.NewGroupField("Comments", parquet.Repeated(stringRef)),
		fireparquet.NewGroupField("DefaultSampleType", parquet.Int(64)),
	})
)

type Sample struct {
	StacktraceID uint64             `parquet:",delta"`
	Values       []int64            `parquet:","`
	Labels       []*profilev1.Label `parquet:","`
}

type Profile struct {
	// A unique UUID per ingested profile
	ID uuid.UUID `parquet:",uuid"`

	// SeriesRefs reference the underlying series in the TSDB index
	SeriesRefs []model.Fingerprint `parquet:","`

	// The set of samples recorded in this profile.
	Samples []*Sample `parquet:","`

	// frames with Function.function_name fully matching the following
	// regexp will be dropped from the samples, along with their successors.
	DropFrames int64 `parquet:","` // Index into string table.
	// frames with Function.function_name fully matching the following
	// regexp will be kept, even if it matches drop_frames.
	KeepFrames int64 `parquet:","` // Index into string table.
	// Time of collection (UTC) represented as nanoseconds past the epoch.
	TimeNanos int64 `parquet:",delta,timestamp(nanosecond)"`
	// Duration of the profile, if a duration makes sense.
	DurationNanos int64 `parquet:",delta"`
	// The number of events between sampled occurrences.
	Period int64 `parquet:","`
	// Freeform text associated to the profile.
	Comment []int64 `parquet:"Comments,"` // Indices into string table.
	// Index into the string table of the type of the preferred sample
	// value. If unset, clients should default to the last sample value.
	DefaultSampleType int64 `parquet:","`
}

type ProfilePersister struct {
}

func (*ProfilePersister) Schema() *parquet.Schema {
	return profilesSchema
}

func (*ProfilePersister) SortingColumns() SortingColumns {
	return parquet.SortingColumns(
		parquet.Ascending("ID"),
		parquet.Ascending("SeriesRefs"),
		parquet.Ascending("TimeNanos"),
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
