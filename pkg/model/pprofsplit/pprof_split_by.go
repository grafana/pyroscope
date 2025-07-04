package pprofsplit

import (
	"strings"

	"github.com/prometheus/prometheus/model/relabel"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

// VisitSampleSeriesBy visits samples in a profile, splitting them by the
// specified labels. Labels shared by all samples in a group (intersection)
// are passed to the visitor, while the rest of the labels are added to each
// sample.
func VisitSampleSeriesBy(
	profile *profilev1.Profile,
	labels phlaremodel.Labels,
	rules []*relabel.Config,
	visitor SampleSeriesVisitor,
	names ...string,
) error {
	m := &sampleSeriesMerger{
		visitor: visitor,
		profile: profile,
		names:   names,
		groups:  make(map[string]*groupBy),
	}
	if err := VisitSampleSeries(profile, labels, rules, m); err != nil {
		return err
	}
	if len(m.groups) == 0 {
		return nil
	}
	for _, k := range m.order {
		group := m.groups[k]
		// For simplicity, we allocate a new slice of samples
		// and delegate ownership to the visitor.
		var size int
		for _, s := range group.samples {
			size += len(s.samples)
		}
		samples := make([]*profilev1.Sample, 0, size)
		// All sample groups share group.labels:
		// we use them as series labels.
		for _, s := range group.samples {
			s.labels = s.labels.Subtract(group.labels)
			m.addLabelsToSamples(s)
			samples = append(samples, s.samples...)
		}
		m.visitor.VisitSampleSeries(group.labels, samples)
	}
	return nil
}

type sampleSeriesMerger struct {
	visitor SampleSeriesVisitor
	profile *profilev1.Profile
	names   []string
	groups  map[string]*groupBy
	order   []string
	strings map[string]int64
}

type groupBy struct {
	labels  phlaremodel.Labels
	samples []sampleGroup
	init    bool
}

type sampleGroup struct {
	labels  phlaremodel.Labels
	samples []*profilev1.Sample
}

func (m *sampleSeriesMerger) ValidateLabels(labels phlaremodel.Labels) error {
	return m.visitor.ValidateLabels(labels)
}

func (m *sampleSeriesMerger) VisitProfile(labels phlaremodel.Labels) {
	m.visitor.VisitProfile(labels)
}

func (m *sampleSeriesMerger) VisitSampleSeries(labels phlaremodel.Labels, samples []*profilev1.Sample) {
	k := groupKey(labels, m.names...)
	group, ok := m.groups[k]
	if !ok {
		group = &groupBy{labels: labels.Clone()}
		m.order = append(m.order, k)
		m.groups[k] = group
	} else {
		group.labels = group.labels.Intersect(labels)
	}
	group.samples = append(group.samples, sampleGroup{
		labels:  labels,
		samples: samples,
	})
}

func (m *sampleSeriesMerger) Discarded(profiles, bytes int) {
	m.visitor.Discarded(profiles, bytes)
}

func groupKey(labels phlaremodel.Labels, by ...string) string {
	var b strings.Builder
	for _, name := range by {
		for i := range labels {
			if labels[i] != nil && labels[i].Name == name {
				if b.Len() > 0 {
					b.WriteByte(',')
				}
				b.WriteString(labels[i].Value)
				break
			}
		}
	}
	return b.String()
}

func (m *sampleSeriesMerger) addLabelsToSamples(s sampleGroup) {
	sampleLabels := make([]*profilev1.Label, len(s.labels))
	for i, label := range s.labels {
		sampleLabels[i] = &profilev1.Label{
			Key: m.string(label.Name),
			Str: m.string(label.Value),
		}
	}
	// We can't reuse labels here.
	for _, sample := range s.samples {
		for i := range sampleLabels {
			sample.Label = append(sample.Label, sampleLabels[i].CloneVT())
		}
	}
}

func (m *sampleSeriesMerger) string(s string) int64 {
	if m.strings == nil {
		m.strings = make(map[string]int64, len(m.profile.StringTable))
		for i, str := range m.profile.StringTable {
			m.strings[str] = int64(i)
		}
	}
	i, ok := m.strings[s]
	if !ok {
		i = int64(len(m.profile.StringTable))
		m.strings[s] = i
		m.profile.StringTable = append(m.profile.StringTable, s)
	}
	return i
}
