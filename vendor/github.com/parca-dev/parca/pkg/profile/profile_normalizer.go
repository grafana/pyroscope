// Copyright 2021 The Parca Authors
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

package profile

import (
	"sort"

	pb "github.com/parca-dev/parca/gen/proto/go/parca/metastore/v1alpha1"
)

type mapInfo struct {
	m      *pb.Mapping
	offset int64
}

type StacktraceKey []byte

// MakeStacktraceKey generates stacktraceKey to be used as a key for maps.
func MakeStacktraceKey(sample *Sample) StacktraceKey {
	numLocations := len(sample.Location)
	if numLocations == 0 {
		return []byte{}
	}

	locationLength := (16 * numLocations) + (numLocations - 1)

	labelsLength := 0
	labelName := make([]string, 0, len(sample.Label))
	for l, vs := range sample.Label {
		labelName = append(labelName, l)

		labelsLength += len(l) + 2 // +2 for the quotes
		for _, v := range vs {
			labelsLength += len(v) + 2 // +2 for the quotes
		}
		labelsLength += len(vs) - 1 // spaces
		labelsLength += 2           // square brackets
	}
	sort.Strings(labelName)

	numLabelsLength := 0
	numLabelNames := make([]string, 0, len(sample.NumLabel))
	for l, int64s := range sample.NumLabel {
		numLabelNames = append(numLabelNames, l)

		numLabelsLength += len(l) + 2      // +2 for the quotes
		numLabelsLength += 2               // square brackets
		numLabelsLength += 8 * len(int64s) // 8*8=64bit

		if len(sample.NumUnit[l]) > 0 {
			for i := range int64s {
				numLabelsLength += len(sample.NumUnit[l][i]) + 2 // numUnit string +2 for quotes
			}

			numLabelsLength += 2               // square brackets
			numLabelsLength += len(int64s) - 1 // spaces
		}
	}
	sort.Strings(numLabelNames)

	length := locationLength + labelsLength + numLabelsLength
	key := make([]byte, 0, length)

	for i, l := range sample.Location {
		key = append(key, l.ID[:]...)
		if i != len(sample.Location)-1 {
			key = append(key, '|')
		}
	}

	for i := 0; i < len(sample.Label); i++ {
		l := labelName[i]
		vs := sample.Label[l]
		key = append(key, '"')
		key = append(key, l...)
		key = append(key, '"')

		key = append(key, '[')
		for i, v := range vs {
			key = append(key, '"')
			key = append(key, v...)
			key = append(key, '"')
			if i != len(vs)-1 {
				key = append(key, ' ')
			}
		}
		key = append(key, ']')
	}

	for i := 0; i < len(sample.NumLabel); i++ {
		l := numLabelNames[i]
		int64s := sample.NumLabel[l]

		key = append(key, '"')
		key = append(key, l...)
		key = append(key, '"')

		key = append(key, '[')
		for _, v := range int64s {
			// Writing int64 to pre-allocated key by shifting per byte
			for shift := 56; shift >= 0; shift -= 8 {
				key = append(key, byte(v>>shift))
			}
		}
		key = append(key, ']')

		key = append(key, '[')
		for i := range int64s {
			if len(sample.NumUnit[l]) > 0 {
				s := sample.NumUnit[l][i]
				key = append(key, '"')
				key = append(key, s...)
				key = append(key, '"')
				if i != len(int64s)-1 {
					key = append(key, ' ')
				}
			}
		}
		key = append(key, ']')
	}

	return key
}
