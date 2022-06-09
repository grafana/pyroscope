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
	"context"
	"errors"
	"sort"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/polarsignals/arcticdb/dynparquet"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/parca-dev/parca/pkg/metastore"
	parcaprofile "github.com/parca-dev/parca/pkg/profile"
)

func InsertProfileIntoTable(ctx context.Context, logger log.Logger, table Table, ls labels.Labels, prof *parcaprofile.Profile) (int, error) {
	if prof.Meta.Timestamp == 0 {
		return 0, errors.New("timestamp must not be zero")
	}

	buf, err := FlatProfileToBuffer(logger, ls, table.Schema(), prof)
	if err != nil {
		return 0, err
	}

	_, err = table.InsertBuffer(ctx, buf)
	return len(prof.FlatSamples), err
}

func FlatProfileToBuffer(logger log.Logger, inLs labels.Labels, schema *dynparquet.Schema, prof *parcaprofile.Profile) (*dynparquet.Buffer, error) {
	rows, err := FlatProfileToRows(inLs, prof)
	if err != nil {
		return nil, err
	}

	level.Debug(logger).Log("msg", "writing sample", "timestamp", prof.Meta.Timestamp)

	buf, err := rows.ToBuffer(schema)
	if err != nil {
		return nil, err
	}

	buf.Sort()

	// This is necessary because sorting a buffer makes concurrent reading not
	// safe as the internal pages are cyclically sorted at read time. Cloning
	// executes the cyclic sort once and makes the resulting buffer safe for
	// concurrent reading as it no longer has to perform the cyclic sorting at
	// read time. This should probably be improved in the parquet library.
	buf, err = buf.Clone()
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func FlatProfileToRows(inLs labels.Labels, prof *parcaprofile.Profile) (Samples, error) {
	ls := make(labels.Labels, 0, len(inLs))
	name := ""
	for _, l := range inLs {
		if l.Name == "__name__" {
			name = l.Value
		} else {
			ls = append(ls, l)
		}
	}
	if name == "" {
		return nil, ErrMissingNameLabel
	}
	sort.Sort(ls)

	rows := make(Samples, 0, len(prof.FlatSamples))
	for _, s := range prof.FlatSamples {
		pprofLabels := make(map[string]string, len(s.Label))
		for name, values := range s.Label {
			if len(values) != 1 {
				panic("expected exactly one value per pprof label")
			}
			pprofLabels[name] = values[0]
		}
		pprofNumLabels := make(map[string]int64, len(s.NumLabel))
		for name, values := range s.NumLabel {
			if len(values) != 1 {
				panic("expected exactly one value per pprof num label")
			}
			pprofNumLabels[name] = values[0]
		}

		rows = append(rows, &Sample{
			Name:           name,
			SampleType:     prof.Meta.SampleType.Type,
			SampleUnit:     prof.Meta.SampleType.Unit,
			PeriodType:     prof.Meta.PeriodType.Type,
			PeriodUnit:     prof.Meta.PeriodType.Unit,
			PprofLabels:    pprofLabels,
			PprofNumLabels: pprofNumLabels,
			Labels:         ls,
			Stacktrace:     extractLocationIDs(s.Location),
			Timestamp:      prof.Meta.Timestamp,
			Duration:       prof.Meta.Duration,
			Period:         prof.Meta.Period,
			Value:          s.Value,
		})
	}

	return rows, nil
}

func extractLocationIDs(locs []*metastore.Location) []byte {
	b := make([]byte, len(locs)*16) // UUID are 16 bytes thus multiply by 16
	index := 0
	for i := len(locs) - 1; i >= 0; i-- {
		copy(b[index:index+16], locs[i].ID[:])
		index += 16
	}
	return b
}
