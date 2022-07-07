package firedb

import (
	"github.com/google/uuid"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

type Sample struct {
	StacktraceID uint64             `parquet:","`
	Values       []int64            `parquet:","`
	Labels       []*profilev1.Label `parquet:","`
}

type Profile struct {
	// A unique UUID per ingested profile
	ID uuid.UUID `parquet:",dict"`

	// External label references
	ExternalLabels []LabelPairRef `parquet:","`

	// A description of the samples associated with each Sample.value.
	// For a cpu profile this might be:
	//   [["cpu","nanoseconds"]] or [["wall","seconds"]] or [["syscall","count"]]
	// For a heap profile, this might be:
	//   [["allocations","count"], ["space","bytes"]],
	// If one of the values represents the number of events represented
	// by the sample, by convention it should be at index 0 and use
	// sample_type.unit == "count".
	// TODO: Store only single type here
	Types []*profilev1.ValueType `parquet:","`
	// The set of samples recorded in this profile.
	// TODO: Flatten into per type sample
	Samples []*Sample `parquet:","`

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
	Comment []int64 `parquet:"Comments,"` // Indices into string table.
	// Index into the string table of the type of the preferred sample
	// value. If unset, clients should default to the last sample value.
	DefaultSampleType int64 `parquet:","`
}

type profilesHelper struct{}

func (*profilesHelper) key(s *Profile) profilesKey {
	id := s.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return profilesKey{
		ID: id,
	}

}

func (*profilesHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (*profilesHelper) rewrite(r *rewriter, s *Profile) error {

	for pos := range s.Types {
		r.strings.rewrite(&s.Types[pos].Type)
		r.strings.rewrite(&s.Types[pos].Unit)
	}

	for pos := range s.Comment {
		r.strings.rewrite(&s.Comment[pos])
	}

	r.strings.rewrite(&s.DropFrames)
	r.strings.rewrite(&s.KeepFrames)

	return nil
}

type profilesKey struct {
	ID uuid.UUID
}
