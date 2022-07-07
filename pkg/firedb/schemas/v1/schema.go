package v1

import (
	"github.com/segmentio/parquet-go"

	fireparquet "github.com/grafana/fire/pkg/parquet"
)

func ProfilesSchema() *parquet.Schema {
	var (
		stringRef  = parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)
		sampleType = fireparquet.Group{
			fireparquet.NewGroupField("Type", stringRef),
			fireparquet.NewGroupField("Unit", stringRef),
		}

		pprofLabels = parquet.Repeated(fireparquet.Group{
			fireparquet.NewGroupField("Key", stringRef),
			fireparquet.NewGroupField("Str", parquet.Optional(stringRef)),
			fireparquet.NewGroupField("Num", parquet.Optional(parquet.Int(64))),
			fireparquet.NewGroupField("NumUnit", parquet.Optional(stringRef)),
		})

		sample = fireparquet.Group{
			fireparquet.NewGroupField("StacktraceID", parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)),
			fireparquet.NewGroupField("Values", parquet.Repeated(parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked))),
			fireparquet.NewGroupField("Labels", pprofLabels),
		}
	)

	return parquet.NewSchema("Profile", fireparquet.Group{
		fireparquet.NewGroupField("ID", parquet.UUID()),
		fireparquet.NewGroupField("SeriesIDs", parquet.Repeated(parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked))),
		fireparquet.NewGroupField("Samples", parquet.Repeated(sample)),
		fireparquet.NewGroupField("DropFrames", stringRef),
		fireparquet.NewGroupField("KeepFrames", stringRef),
		fireparquet.NewGroupField("TimeNanos", parquet.Timestamp(parquet.Nanosecond)),
		fireparquet.NewGroupField("DurationNanos", parquet.Int(64)),
		fireparquet.NewGroupField("PeriodType", parquet.Optional(sampleType)),
		fireparquet.NewGroupField("Period", parquet.Int(64)),
		fireparquet.NewGroupField("Comments", parquet.Repeated(stringRef)),
		fireparquet.NewGroupField("DefaultSampleType", parquet.Int(64)),
	})
}
