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
	"fmt"
	"sort"

	"github.com/polarsignals/arcticdb/dynparquet"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/segmentio/parquet-go"
)

// Sample is a decomposed pprof Sample that will be appended to columnar storage.
// All fields are columns that are sorted by their name in the schema too.
type Sample struct {
	Duration       int64
	Labels         labels.Labels
	Name           string
	Period         int64
	PeriodType     string
	PeriodUnit     string
	PprofLabels    map[string]string
	PprofNumLabels map[string]int64
	SampleType     string
	SampleUnit     string
	Stacktrace     []byte
	Timestamp      int64
	Value          int64
}

type Samples []*Sample

func (s Samples) ToBuffer(schema *dynparquet.Schema) (*dynparquet.Buffer, error) {
	names := s.SampleLabelNames()
	pprofLabels := s.pprofLabelsNames()
	pprofNumLabels := s.pprofNumLabelsNames()

	pb, err := schema.NewBuffer(map[string][]string{
		ColumnLabels:         names,
		ColumnPprofLabels:    pprofLabels,
		ColumnPprofNumLabels: pprofNumLabels,
	})
	if err != nil {
		return nil, err
	}

	var r parquet.Row
	for _, sample := range s {
		r = sample.ToParquetRow(schema, r[:0], names, pprofLabels, pprofNumLabels)
		_, err := pb.WriteRows([]parquet.Row{r})
		if err != nil {
			return nil, err
		}
	}

	return pb, nil
}

func (s Samples) SampleLabelNames() []string {
	names := []string{}
	seen := map[string]struct{}{}

	for _, sample := range s {
		for _, label := range sample.Labels {
			if _, ok := seen[label.Name]; !ok {
				names = append(names, label.Name)
				seen[label.Name] = struct{}{}
			}
		}
	}
	sort.Strings(names)

	return names
}

func (s Samples) pprofLabelsNames() []string {
	names := []string{}
	seen := map[string]struct{}{}

	for _, sample := range s {
		for name := range sample.PprofLabels {
			if _, ok := seen[name]; !ok {
				names = append(names, name)
				seen[name] = struct{}{}
			}
		}
	}
	sort.Strings(names)

	return names
}

func (s Samples) pprofNumLabelsNames() []string {
	names := []string{}
	seen := map[string]struct{}{}

	for _, sample := range s {
		for name := range sample.PprofNumLabels {
			if _, ok := seen[name]; !ok {
				names = append(names, name)
				seen[name] = struct{}{}
			}
		}
	}
	sort.Strings(names)

	return names
}

func (s Sample) ToParquetRow(schema *dynparquet.Schema, row parquet.Row, labelNames, pprofLabelNames, pprofNumLabelNames []string) parquet.Row {
	// schema.Columns() returns a sorted list of all columns.
	// We match on the column's name to insert the correct values.
	// We track the columnIndex to insert each column at the correct index.
	columnIndex := 0
	for _, column := range schema.Columns() {
		switch column.Name {
		case ColumnDuration:
			row = append(row, parquet.ValueOf(s.Duration).Level(0, 0, columnIndex))
			columnIndex++
		case ColumnName:
			row = append(row, parquet.ValueOf(s.Name).Level(0, 0, columnIndex))
			columnIndex++
		case ColumnPeriod:
			row = append(row, parquet.ValueOf(s.Period).Level(0, 0, columnIndex))
			columnIndex++
		case ColumnPeriodType:
			row = append(row, parquet.ValueOf(s.PeriodType).Level(0, 0, columnIndex))
			columnIndex++
		case ColumnPeriodUnit:
			row = append(row, parquet.ValueOf(s.PeriodUnit).Level(0, 0, columnIndex))
			columnIndex++
		case ColumnSampleType:
			row = append(row, parquet.ValueOf(s.SampleType).Level(0, 0, columnIndex))
			columnIndex++
		case ColumnSampleUnit:
			row = append(row, parquet.ValueOf(s.SampleUnit).Level(0, 0, columnIndex))
			columnIndex++
		case ColumnStacktrace:
			row = append(row, parquet.ValueOf(s.Stacktrace).Level(0, 0, columnIndex))
			columnIndex++
		case ColumnTimestamp:
			row = append(row, parquet.ValueOf(s.Timestamp).Level(0, 0, columnIndex))
			columnIndex++
		case ColumnValue:
			row = append(row, parquet.ValueOf(s.Value).Level(0, 0, columnIndex))
			columnIndex++

		// All remaining cases take care of dynamic columns
		case ColumnLabels:
			labelNamesLen := len(labelNames)
			i, j := 0, 0
			for i < labelNamesLen {
				if labelNames[i] == s.Labels[j].Name {
					value := parquet.ValueOf(s.Labels[j].Value).Level(0, 1, columnIndex)
					row = append(row, value)
					columnIndex++
					i++
					j++

					if j >= len(s.Labels) {
						for ; i < labelNamesLen; i++ {
							row = append(row, parquet.ValueOf(nil).Level(0, 0, columnIndex))
						}
						break
					}
				} else {
					// If nothing matches we add a NULL to the column
					row = append(row, parquet.ValueOf(nil).Level(0, 0, columnIndex))
					columnIndex++
					i++
				}
			}
		case ColumnPprofLabels:
			for _, name := range pprofLabelNames {
				if value, ok := s.PprofLabels[name]; ok {
					row = append(row, parquet.ValueOf(value).Level(0, 1, columnIndex))
					columnIndex++
				} else {
					row = append(row, parquet.ValueOf(nil).Level(0, 0, columnIndex))
					columnIndex++
				}
			}
		case ColumnPprofNumLabels:
			for _, name := range pprofNumLabelNames {
				if value, ok := s.PprofNumLabels[name]; ok {
					row = append(row, parquet.ValueOf(value).Level(0, 1, columnIndex))
					columnIndex++
				} else {
					row = append(row, parquet.ValueOf(nil).Level(0, 0, columnIndex))
					columnIndex++
				}
			}
		default:
			panic(fmt.Errorf("conversion not implement for column: %s", column.Name))
		}
	}

	return row
}
