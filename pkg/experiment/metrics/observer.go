package metrics

import (
	"sort"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"

	"github.com/grafana/pyroscope/pkg/experiment/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type SampleObserver struct {
	state *observerState

	recordingTime  int64
	externalLabels labels.Labels

	exporter Exporter
	ruler    Ruler
}

type observerState struct {
	tenant     string
	recordings []*Recording
}

type Recording struct {
	rule  *RecordingRule
	data  map[model.Fingerprint]*Sample
	state *recordingState
}

type recordingState struct {
	fp      *model.Fingerprint
	matches bool
	sample  *Sample
}

type Sample struct {
	Labels labels.Labels
	Value  float64
}

func NewSampleObserver(recordingTime int64, exporter Exporter, ruler Ruler, labels ...labels.Label) *SampleObserver {
	return &SampleObserver{
		recordingTime:  recordingTime,
		externalLabels: labels,
		exporter:       exporter,
		ruler:          ruler,
	}
}

func (o *SampleObserver) initObserver(tenant string) {
	recordingRules := o.ruler.RecordingRules(tenant)

	o.state = &observerState{
		tenant:     tenant,
		recordings: make([]*Recording, len(recordingRules)),
	}

	for i, rule := range recordingRules {
		o.state.recordings[i] = &Recording{
			rule:  rule,
			data:  make(map[model.Fingerprint]*Sample),
			state: &recordingState{},
		}
	}
}

func (o *SampleObserver) Observe(row block.ProfileEntry) {
	tenant := row.Dataset.TenantID()
	if o.state == nil {
		o.initObserver(tenant)
	}
	if o.state.tenant != row.Dataset.TenantID() {
		// new tenant to observe, flush data of previous tenant and restart the observer
		o.flush()
		o.initObserver(tenant)
	}
	for _, recording := range o.state.recordings {
		if recording.state.fp == nil || *recording.state.fp != row.Fingerprint {
			// new batch of rows, let's precompute its state for this recording
			recording.initState(row.Fingerprint, row.Labels, o.externalLabels)
		}
		if recording.state.matches {
			recording.state.sample.Value += float64(row.Row.TotalValue())
		}
	}
}

func (o *SampleObserver) flush() {
	timeSeries := make([]prompb.TimeSeries, 0)
	for _, recording := range o.state.recordings {
		for _, sample := range recording.data {
			ts := prompb.TimeSeries{
				Labels: make([]prompb.Label, 0, len(sample.Labels)),
				Samples: []prompb.Sample{
					{
						Value:     sample.Value,
						Timestamp: o.recordingTime,
					},
				},
			}
			for _, l := range sample.Labels {
				ts.Labels = append(ts.Labels, prompb.Label{
					Name:  l.Name,
					Value: l.Value,
				})
			}
			timeSeries = append(timeSeries, ts)
		}
	}
	if len(timeSeries) > 0 {
		_ = o.exporter.Send(o.state.tenant, timeSeries)
	}
	o.state = nil
}

func (o *SampleObserver) Close() {
	if o.state != nil {
		o.flush()
	}
}

func (r *Recording) initState(fp model.Fingerprint, rowLabels phlaremodel.Labels, externalLabels labels.Labels) {
	r.state.fp = &fp
	labelsMap := map[string]string{}
	for _, label := range rowLabels {
		labelsMap[label.Name] = label.Value
	}
	r.state.matches = r.matches(labelsMap)
	if !r.state.matches {
		return
	}

	exportedLabels := generateExportedLabels(labelsMap, r, externalLabels)
	sort.Sort(exportedLabels)
	aggregatedFp := model.Fingerprint(exportedLabels.Hash())

	sample, ok := r.data[aggregatedFp]
	if !ok {
		sample = &Sample{
			Labels: exportedLabels,
			Value:  0,
		}
		r.data[aggregatedFp] = sample
	}
	r.state.sample = sample
}

func generateExportedLabels(labelsMap map[string]string, rec *Recording, externalLabels labels.Labels) labels.Labels {
	exportedLabels := make(labels.Labels, 0, len(externalLabels)+len(rec.rule.ExternalLabels)+len(rec.rule.KeepLabels))

	// Add observer's external labels
	for _, label := range externalLabels {
		exportedLabels = append(exportedLabels, label)
	}

	// Add rule's external labels
	for _, label := range rec.rule.ExternalLabels {
		exportedLabels = append(exportedLabels, label)
	}

	// Keep the groupBy labels if present
	for _, label := range rec.rule.KeepLabels {
		labelValue, ok := labelsMap[label]
		if ok {
			exportedLabels = append(exportedLabels, labels.Label{
				Name:  label,
				Value: labelValue,
			})
		}
	}
	return exportedLabels
}

func (r *Recording) matches(labelsMap map[string]string) bool {
	for _, matcher := range r.rule.Matchers {
		if !matcher.Matches(labelsMap[matcher.Name]) {
			return false
		}
	}
	return true
}
