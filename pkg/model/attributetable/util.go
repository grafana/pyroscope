package attributetable

import (
	"slices"

	"github.com/cespare/xxhash/v2"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// hashLabels computes a hash for a slice of label pairs.
// This is used for identifying unique series.
func hashLabels(labels []*typesv1.LabelPair) uint64 {
	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024)
	for i, v := range labels {
		if len(b)+len(v.Name)+len(v.Value)+2 >= cap(b) {
			// If labels entry is 1KB+ do not allocate whole entry.
			h := xxhash.New()
			_, _ = h.Write(b)
			for _, v := range labels[i:] {
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

var seps = []byte{'\xff'}

// compareLabelPairs compares two slices of label pairs lexicographically.
func compareLabelPairs(a, b []*typesv1.LabelPair) int {
	l := len(a)
	if len(b) < l {
		l = len(b)
	}

	for i := 0; i < l; i++ {
		if a[i].Name < b[i].Name {
			return -1
		} else if a[i].Name > b[i].Name {
			return 1
		}

		if a[i].Value < b[i].Value {
			return -1
		} else if a[i].Value > b[i].Value {
			return 1
		}
	}

	// If all labels are equal so far, the shorter slice is "less"
	if len(a) < len(b) {
		return -1
	} else if len(a) > len(b) {
		return 1
	}

	return 0
}

// mergeAnnotations merges two slices of profile annotations, removing duplicates.
func mergeAnnotations(a, b []*typesv1.ProfileAnnotation) []*typesv1.ProfileAnnotation {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	merged := append(a, b...)

	slices.SortFunc(merged, func(a, b *typesv1.ProfileAnnotation) int {
		if a.Key < b.Key {
			return -1
		}
		if a.Key > b.Key {
			return 1
		}
		if a.Value < b.Value {
			return -1
		}
		if a.Value > b.Value {
			return 1
		}
		return 0
	})

	// Remove duplicates in-place
	j := 0
	for i := 1; i < len(merged); i++ {
		if merged[j].Key != merged[i].Key || merged[j].Value != merged[i].Value {
			j++
			merged[j] = merged[i]
		}
	}

	return merged[:j+1]
}
