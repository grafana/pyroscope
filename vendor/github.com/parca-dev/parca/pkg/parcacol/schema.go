// Copyright 2022 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parcacol

import (
	"github.com/polarsignals/arcticdb/dynparquet"
	"github.com/segmentio/parquet-go"
)

const (
	SchemaName = "parca"
	// The columns are sorted by their name in the schema too.
	ColumnDuration       = "duration"
	ColumnLabels         = "labels"
	ColumnName           = "name"
	ColumnPeriod         = "period"
	ColumnPeriodType     = "period_type"
	ColumnPeriodUnit     = "period_unit"
	ColumnPprofLabels    = "pprof_labels"
	ColumnPprofNumLabels = "pprof_num_labels"
	ColumnSampleType     = "sample_type"
	ColumnSampleUnit     = "sample_unit"
	ColumnStacktrace     = "stacktrace"
	ColumnTimestamp      = "timestamp"
	ColumnValue          = "value"
)

func Schema() *dynparquet.Schema {
	return dynparquet.NewSchema(
		SchemaName,
		[]dynparquet.ColumnDefinition{
			{
				Name:          ColumnDuration,
				StorageLayout: parquet.Int(64),
				Dynamic:       false,
			}, {
				Name:          ColumnLabels,
				StorageLayout: parquet.Encoded(parquet.Optional(parquet.String()), &parquet.RLEDictionary),
				Dynamic:       true,
			}, {
				Name:          ColumnName,
				StorageLayout: parquet.Encoded(parquet.String(), &parquet.RLEDictionary),
				Dynamic:       false,
			}, {
				Name:          ColumnPeriod,
				StorageLayout: parquet.Int(64),
				Dynamic:       false,
			}, {
				Name:          ColumnPeriodType,
				StorageLayout: parquet.Encoded(parquet.String(), &parquet.RLEDictionary),
				Dynamic:       false,
			}, {
				Name:          ColumnPeriodUnit,
				StorageLayout: parquet.Encoded(parquet.String(), &parquet.RLEDictionary),
				Dynamic:       false,
			}, {
				Name:          ColumnPprofLabels,
				StorageLayout: parquet.Encoded(parquet.Optional(parquet.String()), &parquet.RLEDictionary),
				Dynamic:       true,
			}, {
				Name:          ColumnPprofNumLabels,
				StorageLayout: parquet.Optional(parquet.Int(64)),
				Dynamic:       true,
			}, {
				Name:          ColumnSampleType,
				StorageLayout: parquet.Encoded(parquet.String(), &parquet.RLEDictionary),
				Dynamic:       false,
			}, {
				Name:          ColumnSampleUnit,
				StorageLayout: parquet.Encoded(parquet.String(), &parquet.RLEDictionary),
				Dynamic:       false,
			}, {
				Name:          ColumnStacktrace,
				StorageLayout: parquet.Encoded(parquet.String(), &parquet.RLEDictionary),
				Dynamic:       false,
			}, {
				Name:          ColumnTimestamp,
				StorageLayout: parquet.Int(64),
				Dynamic:       false,
			}, {
				Name:          ColumnValue,
				StorageLayout: parquet.Int(64),
				Dynamic:       false,
			},
		},
		[]dynparquet.SortingColumn{
			dynparquet.Ascending(ColumnName),
			dynparquet.Ascending(ColumnSampleType),
			dynparquet.Ascending(ColumnSampleUnit),
			dynparquet.Ascending(ColumnPeriodType),
			dynparquet.Ascending(ColumnPeriodUnit),
			dynparquet.NullsFirst(dynparquet.Ascending(ColumnLabels)),
			dynparquet.NullsFirst(dynparquet.Ascending(ColumnStacktrace)),
			dynparquet.Ascending(ColumnTimestamp),
			dynparquet.NullsFirst(dynparquet.Ascending(ColumnPprofLabels)),
			dynparquet.NullsFirst(dynparquet.Ascending(ColumnPprofNumLabels)),
		},
	)
}
