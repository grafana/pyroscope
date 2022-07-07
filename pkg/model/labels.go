package model

import (
	"bytes"
	"sort"
	"strconv"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
)

var seps = []byte{'\xff'}

// Labels is a sorted set of labels. Order has to be guaranteed upon
// instantiation.
type Labels []*commonv1.LabelPair

func (ls Labels) Len() int           { return len(ls) }
func (ls Labels) Swap(i, j int)      { ls[i], ls[j] = ls[j], ls[i] }
func (ls Labels) Less(i, j int) bool { return ls[i].Name < ls[j].Name }

// Hash returns a hash value for the label set.
func (ls Labels) Hash() uint64 {
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

// HashForLabels returns a hash value for the labels matching the provided names.
// 'names' have to be sorted in ascending order.
func (ls Labels) HashForLabels(b []byte, names ...string) (uint64, []byte) {
	b = b[:0]
	i, j := 0, 0
	for i < len(ls) && j < len(names) {
		if names[j] < ls[i].Name {
			j++
		} else if ls[i].Name < names[j] {
			i++
		} else {
			b = append(b, ls[i].Name...)
			b = append(b, seps[0])
			b = append(b, ls[i].Value...)
			b = append(b, seps[0])
			i++
			j++
		}
	}
	return xxhash.Sum64(b), b
}

// HashWithoutLabels returns a hash value for all labels except those matching
// the provided names.
// 'names' have to be sorted in ascending order.
func (ls Labels) HashWithoutLabels(b []byte, names ...string) (uint64, []byte) {
	b = b[:0]
	j := 0
	for i := range ls {
		for j < len(names) && names[j] < ls[i].Name {
			j++
		}
		if ls[i].Name == labels.MetricName || (j < len(names) && ls[i].Name == names[j]) {
			continue
		}
		b = append(b, ls[i].Name...)
		b = append(b, seps[0])
		b = append(b, ls[i].Value...)
		b = append(b, seps[0])
	}
	return xxhash.Sum64(b), b
}

// Get returns the value for the label with the given name.
// Returns an empty string if the label doesn't exist.
func (ls Labels) Get(name string) string {
	for _, l := range ls {
		if l.Name == name {
			return l.Value
		}
	}
	return ""
}

// LabelPairsString returns a string representation of the label pairs.
func LabelPairsString(lbs []*commonv1.LabelPair) string {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, l := range lbs {
		if i > 0 {
			b.WriteByte(',')
			b.WriteByte(' ')
		}
		b.WriteString(l.Name)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(l.Value))
	}
	b.WriteByte('}')
	return b.String()
}

// StringToLabelsPairs converts a string representation of label pairs to a slice of label pairs.
func StringToLabelsPairs(s string) ([]*commonv1.LabelPair, error) {
	matchers, err := parser.ParseMetricSelector(s)
	if err != nil {
		return nil, err
	}
	result := make([]*commonv1.LabelPair, len(matchers))
	for i := range matchers {
		result[i] = &commonv1.LabelPair{
			Name:  matchers[i].Name,
			Value: matchers[i].Value,
		}
	}
	return result, nil
}

// LabelsFromStrings creates new labels from pairs of strings.
func LabelsFromStrings(ss ...string) Labels {
	if len(ss)%2 != 0 {
		panic("invalid number of strings")
	}
	var res Labels
	for i := 0; i < len(ss); i += 2 {
		res = append(res, &commonv1.LabelPair{Name: ss[i], Value: ss[i+1]})
	}

	sort.Sort(res)
	return res
}

// CloneLabelPairs clones the label pairs.
func CloneLabelPairs(lbs []*commonv1.LabelPair) []*commonv1.LabelPair {
	result := make([]*commonv1.LabelPair, len(lbs))
	for i, l := range lbs {
		result[i] = &commonv1.LabelPair{
			Name:  l.Name,
			Value: l.Value,
		}
	}
	return result
}

// Compare compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
func CompareLabelPairs(a []*commonv1.LabelPair, b []*commonv1.LabelPair) int {
	l := len(a)
	if len(b) < l {
		l = len(b)
	}

	for i := 0; i < l; i++ {
		if a[i].Name != b[i].Name {
			if a[i].Name < b[i].Name {
				return -1
			}
			return 1
		}
		if a[i].Value != b[i].Value {
			if a[i].Value < b[i].Value {
				return -1
			}
			return 1
		}
	}
	// If all labels so far were in common, the set with fewer labels comes first.
	return len(a) - len(b)
}
