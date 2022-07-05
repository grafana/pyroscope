package v2

import (
	"github.com/polarsignals/frostdb/dynparquet"
	"github.com/segmentio/parquet-go"
)

const (
	ColumnID            = "id"
	ColumnLabels        = "labels"
	ColumnSampleType    = "sample_type"
	ColumnSampleUnit    = "sample_unit"
	ColumnLocationIDs   = "location_ids"
	ColumnSamples       = "samples"
	ColumnPprofLabels   = "pprof_labels"
	ColumnDropFrames    = "drop_frames"
	ColumnKeepFrames    = "keep_frames"
	ColumnTimeNanos     = "time_nanos"
	ColumnDurationNanos = "duration_nanos"
	ColumnPeriod        = "period"
	ColumnPeriodType    = "period_type"
	ColumnPeriodUnit    = "period_unit"
	ColumnComments      = "comments"
)

func Profiles() *dynparquet.Schema {
	stringRef := parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)

	labels := parquet.Repeated(parquet.Group{
		"Name":  stringRef,
		"Value": stringRef,
	})

	pprofLabels := parquet.Repeated(parquet.Group{
		"Key":     stringRef,
		"Str":     parquet.Optional(stringRef),
		"Num":     parquet.Optional(parquet.Int(64)),
		"NumUnit": parquet.Optional(stringRef),
	})

	return dynparquet.NewSchema(
		"profiles",
		[]dynparquet.ColumnDefinition{
			{
				Name:          ColumnID,
				StorageLayout: parquet.Int(64),
				Dynamic:       false,
			}, {
				Name:          ColumnLabels,
				StorageLayout: labels,
				Dynamic:       false,
			}, {
				Name:          ColumnSampleType,
				StorageLayout: stringRef,
				Dynamic:       false,
			}, {
				Name:          ColumnSampleUnit,
				StorageLayout: stringRef,
				Dynamic:       false,
			}, {
				Name:          ColumnLocationIDs,
				StorageLayout: parquet.Repeated(parquet.Repeated(parquet.Uint(64))),
				Dynamic:       false,
			}, {
				Name:          ColumnSamples,
				StorageLayout: parquet.Repeated(parquet.Encoded(parquet.Int(64), &parquet.DeltaBinaryPacked)),
				Dynamic:       false,
			}, {
				Name:          ColumnPprofLabels,
				StorageLayout: pprofLabels,
				Dynamic:       false,
			}, {
				Name:          ColumnDropFrames,
				StorageLayout: stringRef,
				Dynamic:       false,
			}, {
				Name:          ColumnKeepFrames,
				StorageLayout: stringRef,
				Dynamic:       false,
			}, {
				Name:          ColumnTimeNanos,
				StorageLayout: parquet.Timestamp(parquet.Nanosecond),
				Dynamic:       false,
			}, {
				Name:          ColumnDurationNanos,
				StorageLayout: parquet.Int(64),
				Dynamic:       false,
			}, {
				Name:          ColumnPeriod,
				StorageLayout: parquet.Int(64),
				Dynamic:       false,
			}, {
				Name:          ColumnPeriodType,
				StorageLayout: stringRef,
				Dynamic:       false,
			}, {
				Name:          ColumnPeriodUnit,
				StorageLayout: stringRef,
				Dynamic:       true,
			},
		},
		[]dynparquet.SortingColumn{
			dynparquet.Ascending(ColumnID),
			dynparquet.Ascending(ColumnSampleType),
			dynparquet.Ascending(ColumnSampleUnit),
			dynparquet.Ascending(ColumnPeriodType),
			dynparquet.Ascending(ColumnPeriodUnit),
			dynparquet.NullsFirst(dynparquet.Ascending(ColumnLabels)),
			dynparquet.NullsFirst(dynparquet.Ascending(ColumnLocationIDs)),
			dynparquet.Ascending(ColumnTimeNanos),
			dynparquet.NullsFirst(dynparquet.Ascending(ColumnPprofLabels)),
		},
	)
}
