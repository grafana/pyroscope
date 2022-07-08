package v1

import (
	"github.com/google/uuid"
	"github.com/segmentio/parquet-go"

	"github.com/prometheus/common/model"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
	fireparquet "github.com/grafana/fire/pkg/parquet"
)

type Sample struct {
	StacktraceID uint64             `parquet:","`
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

func ProfilesSchema() *parquet.Schema {
	var (
		stringRef = parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)

		pprofLabels = parquet.Repeated(fireparquet.Group{
			fireparquet.NewGroupField("Key", stringRef),
			fireparquet.NewGroupField("Str", parquet.Optional(stringRef)),
			fireparquet.NewGroupField("Num", parquet.Optional(parquet.Int(64))),
			fireparquet.NewGroupField("NumUnit", parquet.Optional(stringRef)),
		})

		sample = fireparquet.Group{
			fireparquet.NewGroupField("StacktraceID", parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked)),
			fireparquet.NewGroupField("Values", parquet.Repeated(parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked))),
			fireparquet.NewGroupField("Labels", pprofLabels),
		}
	)

	return parquet.NewSchema("Profile", fireparquet.Group{
		fireparquet.NewGroupField("ID", parquet.UUID()),
		fireparquet.NewGroupField("SeriesRefs", parquet.Repeated(parquet.Encoded(parquet.Uint(64), &parquet.DeltaBinaryPacked))),
		fireparquet.NewGroupField("Samples", parquet.Repeated(sample)),
		fireparquet.NewGroupField("DropFrames", stringRef),
		fireparquet.NewGroupField("KeepFrames", stringRef),
		fireparquet.NewGroupField("TimeNanos", parquet.Timestamp(parquet.Nanosecond)),
		fireparquet.NewGroupField("DurationNanos", parquet.Int(64)),
		fireparquet.NewGroupField("Period", parquet.Int(64)),
		fireparquet.NewGroupField("Comments", parquet.Repeated(stringRef)),
		fireparquet.NewGroupField("DefaultSampleType", parquet.Int(64)),
	})
}

type Stacktrace struct {
	LocationIDs []uint64 `parquet:","`
}
