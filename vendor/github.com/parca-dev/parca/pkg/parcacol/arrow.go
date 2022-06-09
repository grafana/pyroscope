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
	"fmt"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"

	"github.com/parca-dev/parca/pkg/metastore"
	"github.com/parca-dev/parca/pkg/profile"
)

func ArrowRecordToStacktraceSamples(
	ctx context.Context,
	metaStore metastore.ProfileMetaStore,
	ar arrow.Record,
	valueColumnName string,
) (*profile.StacktraceSamples, error) {
	// sample is an intermediate representation used before
	// we actually have the profile.Sample assembled from the metastore.
	type sample struct {
		stacktraceID []byte
		locationIDs  [][]byte
		value        int64
	}

	schema := ar.Schema()
	indices := schema.FieldIndices("stacktrace")
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected exactly one stacktrace column, got %d", len(indices))
	}
	stacktraceColumn := ar.Column(indices[0]).(*array.Binary)

	indices = schema.FieldIndices("sum(value)")
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected exactly one value column, got %d", len(indices))
	}
	valueColumn := ar.Column(indices[0]).(*array.Int64)

	rows := int(ar.NumRows())
	samples := make([]*sample, 0, rows)
	stacktraceUUIDs := make([][]byte, 0, rows)
	for i := 0; i < rows; i++ {
		stacktraceID := stacktraceColumn.Value(i)
		value := valueColumn.Value(i)

		stacktraceUUIDs = append(stacktraceUUIDs, stacktraceID)
		samples = append(samples, &sample{
			stacktraceID: stacktraceID,
			value:        value,
		})
	}

	stacktraceMap, err := metaStore.GetStacktraceByIDs(ctx, stacktraceUUIDs...)
	if err != nil {
		return nil, err
	}

	locationUUIDSeen := map[string]struct{}{}
	locationUUIDs := [][]byte{}
	for _, s := range stacktraceMap {
		for _, id := range s.GetLocationIds() {
			if _, seen := locationUUIDSeen[string(id)]; !seen {
				locationUUIDSeen[string(id)] = struct{}{}
				locationUUIDs = append(locationUUIDs, id)
			}
		}
	}

	locationsMap, err := metastore.GetLocationsByIDs(ctx, metaStore, locationUUIDs...)
	if err != nil {
		return nil, err
	}

	for _, s := range samples {
		s.locationIDs = stacktraceMap[string(s.stacktraceID)].LocationIds
	}

	stackSamples := make([]*profile.Sample, 0, len(samples))
	for _, s := range samples {
		stackSample := &profile.Sample{
			Value:    s.value,
			Location: make([]*metastore.Location, 0, len(s.locationIDs)),
		}

		for _, l := range s.locationIDs {
			stackSample.Location = append(stackSample.Location, locationsMap[string(l)])
		}

		stackSamples = append(stackSamples, stackSample)
	}

	return &profile.StacktraceSamples{
		Samples: stackSamples,
	}, nil
}
