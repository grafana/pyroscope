package pprof

import (
	"fmt"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

func ExtractMetrics(p *profilev1.Profile, fn func(labels.Labels, int64)) {
	var m map[string]int64
	si := cpuNanosSampleTypeIndex(p)
	if si < 0 {
		return
	}
	for _, sample := range p.Sample {
		slices.RemoveInPlace(sample.Label, func(label *profilev1.Label, i int) bool {
			s := p.StringTable[label.Key]
			if !strings.HasPrefix(s, pprofMetricLabelPrefix) {
				return false
			}
			if m == nil {
				m = make(map[string]int64, len(p.Sample)/16)
			}
			m[s[len(pprofMetricLabelPrefix):]] += sample.Value[si]
			return true
		})
	}
	for name, value := range m {
		l, err := parser.ParseMetric(name)
		if err != nil {
			fmt.Println(err)
			continue
		}
		fn(l, value)
	}
}

const pprofMetricLabelPrefix = "__m_"

func cpuNanosSampleTypeIndex(p *profilev1.Profile) int {
	for i, sampleType := range p.SampleType {
		if p.StringTable[sampleType.Type] == "cpu" &&
			p.StringTable[sampleType.Unit] == "nanoseconds" {
			return i
		}
	}
	return -1
}
