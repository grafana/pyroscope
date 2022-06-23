package util

import (
	"bytes"
	"strconv"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
	"github.com/prometheus/prometheus/promql/parser"
)

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
func CompareLabelPair(a []*commonv1.LabelPair, b []*commonv1.LabelPair) int {
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
