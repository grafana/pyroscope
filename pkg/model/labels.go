package model

import (
	"bytes"
	"sort"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
	pmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
)

var seps = []byte{'\xff'}

const (
	LabelNameProfileType    = "__profile_type__"
	LabelNameType           = "__type__"
	LabelNameUnit           = "__unit__"
	LabelNamePeriodType     = "__period_type__"
	LabelNamePeriodUnit     = "__period_unit__"
	LabelNameDelta          = "__delta__"
	LabelNameProfileName    = pmodel.MetricNameLabel
	LabelNameServiceName    = "service_name"
	LabelNameServiceNameK8s = "__meta_kubernetes_pod_annotation_pyroscope_io_service_name"

	labelSep = '\xfe'
)

// Labels is a sorted set of labels. Order has to be guaranteed upon
// instantiation.
type Labels []*typesv1.LabelPair

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

// BytesWithLabels is just as Bytes(), but only for labels matching names.
// 'names' have to be sorted in ascending order.
// It uses an byte invalid character as a separator and so should not be used for printing.
func (ls Labels) BytesWithLabels(buf []byte, names ...string) []byte {
	b := bytes.NewBuffer(buf[:0])
	b.WriteByte(labelSep)
	i, j := 0, 0
	for i < len(ls) && j < len(names) {
		if names[j] < ls[i].Name {
			j++
		} else if ls[i].Name < names[j] {
			i++
		} else {
			if b.Len() > 1 {
				b.WriteByte(seps[0])
			}
			b.WriteString(ls[i].Name)
			b.WriteByte(seps[0])
			b.WriteString(ls[i].Value)
			i++
			j++
		}
	}
	return b.Bytes()
}

func (ls Labels) ToPrometheusLabels() labels.Labels {
	res := make([]labels.Label, len(ls))
	for i, l := range ls {
		res[i] = labels.Label{Name: l.Name, Value: l.Value}
	}
	return res
}

func (ls Labels) WithoutPrivateLabels() Labels {
	res := make([]*typesv1.LabelPair, 0, len(ls))
	for _, l := range ls {
		if !strings.HasPrefix(l.Name, "__") {
			res = append(res, l)
		}
	}
	return res
}

// WithLabels returns a subset of Labels that matches match with the provided label names.
func (ls Labels) WithLabels(names ...string) Labels {
	matchedLabels := Labels{}

	nameSet := make(map[string]struct{}, len(names))
	for _, n := range names {
		nameSet[n] = struct{}{}
	}

	for _, v := range ls {
		if _, ok := nameSet[v.Name]; ok {
			matchedLabels = append(matchedLabels, v)
		}
	}

	return matchedLabels
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

func (ls Labels) Clone() Labels {
	result := make(Labels, len(ls))
	for i, l := range ls {
		result[i] = &typesv1.LabelPair{
			Name:  l.Name,
			Value: l.Value,
		}
	}
	return result
}

// LabelPairsString returns a string representation of the label pairs.
func LabelPairsString(lbs []*typesv1.LabelPair) string {
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
func StringToLabelsPairs(s string) ([]*typesv1.LabelPair, error) {
	matchers, err := parser.ParseMetricSelector(s)
	if err != nil {
		return nil, err
	}
	result := make([]*typesv1.LabelPair, len(matchers))
	for i := range matchers {
		result[i] = &typesv1.LabelPair{
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
		res = append(res, &typesv1.LabelPair{Name: ss[i], Value: ss[i+1]})
	}

	sort.Sort(res)
	return res
}

// CloneLabelPairs clones the label pairs.
func CloneLabelPairs(lbs []*typesv1.LabelPair) []*typesv1.LabelPair {
	result := make([]*typesv1.LabelPair, len(lbs))
	for i, l := range lbs {
		result[i] = &typesv1.LabelPair{
			Name:  l.Name,
			Value: l.Value,
		}
	}
	return result
}

// Compare compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
func CompareLabelPairs(a []*typesv1.LabelPair, b []*typesv1.LabelPair) int {
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

// LabelsBuilder allows modifying Labels.
type LabelsBuilder struct {
	base Labels
	del  []string
	add  []*typesv1.LabelPair
}

// NewLabelsBuilder returns a new LabelsBuilder.
func NewLabelsBuilder(base Labels) *LabelsBuilder {
	b := &LabelsBuilder{
		del: make([]string, 0, 5),
		add: make([]*typesv1.LabelPair, 0, 5),
	}
	b.Reset(base)
	return b
}

// Reset clears all current state for the builder.
func (b *LabelsBuilder) Reset(base Labels) {
	b.base = base
	b.del = b.del[:0]
	b.add = b.add[:0]
	for _, l := range b.base {
		if l.Value == "" {
			b.del = append(b.del, l.Name)
		}
	}
}

// Del deletes the label of the given name.
func (b *LabelsBuilder) Del(ns ...string) *LabelsBuilder {
	for _, n := range ns {
		for i, a := range b.add {
			if a.Name == n {
				b.add = append(b.add[:i], b.add[i+1:]...)
			}
		}
		b.del = append(b.del, n)
	}
	return b
}

// Set the name/value pair as a label.
func (b *LabelsBuilder) Set(n, v string) *LabelsBuilder {
	if v == "" {
		// Empty labels are the same as missing labels.
		return b.Del(n)
	}
	for i, a := range b.add {
		if a.Name == n {
			b.add[i].Value = v
			return b
		}
	}
	b.add = append(b.add, &typesv1.LabelPair{Name: n, Value: v})

	return b
}

// Labels returns the labels from the builder. If no modifications
// were made, the original labels are returned.
func (b *LabelsBuilder) Labels() Labels {
	if len(b.del) == 0 && len(b.add) == 0 {
		return b.base
	}

	// In the general case, labels are removed, modified or moved
	// rather than added.
	res := make(Labels, 0, len(b.base))
Outer:
	for _, l := range b.base {
		for _, n := range b.del {
			if l.Name == n {
				continue Outer
			}
		}
		for _, la := range b.add {
			if l.Name == la.Name {
				continue Outer
			}
		}
		res = append(res, l)
	}
	res = append(res, b.add...)
	sort.Sort(res)

	return res
}
