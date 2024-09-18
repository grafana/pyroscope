package util

import (
	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/prometheus/model/labels"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

var seps = []byte{'\xff'}

// StableHash is a labels hashing implementation which is guaranteed to not change over time.
// This function should be used whenever labels hashing backward compatibility must be guaranteed.
func StableHash(ls labels.Labels) uint64 {
	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024)
	for i, v := range ls {
		if len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) {
			// If labels entry is 1KB+ do not allocate whole entry.
			h := xxhash.New()
			_, _ = h.Write(b)
			for _, v := range ls[i:] {
				_, _ = h.WriteString(v.Name)
				_, _ = h.Write(seps)
				_, _ = h.WriteString(v.Value)
				_, _ = h.Write(seps)
			}
			return h.Sum64()
		}

		b = append(b, v.Name...)
		b = append(b, seps[0])
		b = append(b, v.Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b)
}

// MergeLabelNames merges multiple LabelNamesResponses into a single response.
// The result is sorted by label name.
func MergeLabelNames(responses []*typesv1.LabelNamesResponse) *typesv1.LabelNamesResponse {
	nameCount := 0
	includeCardinality := true
	for _, r := range responses {
		nameCount += len(r.Names)

		// Cardinality and names should have a 1:1 mapping. If they don't, we
		// can assume cardinality was not requested.
		if len(r.Names) != len(r.EstimatedCardinality) {
			includeCardinality = false
		}
	}

	uniqueNames := make(map[string]int64, nameCount)
	for _, r := range responses {
		for i, name := range r.Names {
			cardinality := int64(0)
			if includeCardinality {
				cardinality = r.EstimatedCardinality[i]
			}

			if _, ok := uniqueNames[name]; !ok {
				uniqueNames[name] = cardinality
			} else {
				uniqueNames[name] += cardinality
			}
		}
	}

	uniqueRes := &typesv1.LabelNamesResponse{
		Names: make([]string, 0, len(uniqueNames)),
	}

	if includeCardinality {
		uniqueRes.EstimatedCardinality = make([]int64, 0, len(uniqueNames))
		for name, cardinality := range uniqueNames {
			uniqueRes.Names = append(uniqueRes.Names, name)
			uniqueRes.EstimatedCardinality = append(uniqueRes.EstimatedCardinality, cardinality)
		}
	} else {
		for name := range uniqueNames {
			uniqueRes.Names = append(uniqueRes.Names, name)
		}
	}

	SortLabelNamesResponse(uniqueRes)
	return uniqueRes
}
