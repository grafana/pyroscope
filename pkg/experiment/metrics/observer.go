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
	recordings []*recording
}

type recording struct {
	rule  *phlaremodel.RecordingRule
	data  map[model.Fingerprint]*prompb.TimeSeries
	state *recordingState
}

type recordingState struct {
	fp      model.Fingerprint
	matches bool
	sample  *prompb.Sample
}

type Ruler interface {
	// RecordingRules return a validated set of rules for a tenant, with the following guarantees:
	// - a "__name__" label is present among ExternalLabels. It contains a valid prometheus metric name.
	// - a matcher with name "__profile__type__" is present in Matchers
	RecordingRules(tenant string) []*phlaremodel.RecordingRule
}

type Exporter interface {
	Send(tenant string, series []prompb.TimeSeries) error
	Flush()
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
		recordings: make([]*recording, len(recordingRules)),
	}

	for i, rule := range recordingRules {
		o.state.recordings[i] = &recording{
			rule:  rule,
			data:  make(map[model.Fingerprint]*prompb.TimeSeries),
			state: &recordingState{},
		}
	}
}

// Observe manages two kind of states.
//   - Per tenant state:
//     Gets initialized on first/new tenant. It fetches tenant's rules and creates a new recording for each rule.
//     Data of old state is flushed to the exporter.
//   - recording states, per batch of rows:
//     Every recording (hence every rule) has a state that is scoped to every batch of rows of the same fingerprint.
//     When a new row fingerprint is detected, new state is computed for every recording.
//     That state holds whether the rule matches the new batch of rows, and a reference of the sample to
//     be aggregated to. Note that every rule will eventually create multiple single-sample (aggregated) series,
//     depending on the rule.GroupBy space. More info in initState
//
// This call is not thread-safe
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
	for _, rec := range o.state.recordings {
		if rec.state.fp != row.Fingerprint {
			// new batch of rows, let's precompute its state for this recording
			rec.initState(row.Fingerprint, row.Labels, o.externalLabels, o.recordingTime)
		}
		if rec.state.matches {
			rec.state.sample.Value += float64(row.Row.TotalValue())
		}
	}
}

func (o *SampleObserver) flush() {
	timeSeries := make([]prompb.TimeSeries, 0)
	for _, rec := range o.state.recordings {
		for _, series := range rec.data {
			timeSeries = append(timeSeries, *series)
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

// initState compute labelsMap for quick lookups. Then check whether row matches the filters
// if filter match, then labels to export are computed, and fetch/create the series where the value needs to be
// aggregated. This state is hold for the following rows with the same fingerprint, so we can observe those faster
func (r *recording) initState(fp model.Fingerprint, rowLabels phlaremodel.Labels, externalLabels labels.Labels, recordingTime int64) {
	r.state.fp = fp
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

	series, ok := r.data[aggregatedFp]
	if !ok {
		series = newTimeSeries(exportedLabels, recordingTime)
		r.data[aggregatedFp] = series
	}
	r.state.sample = &series.Samples[0]
}

func newTimeSeries(exportedLabels labels.Labels, recordingTime int64) *prompb.TimeSeries {
	// prompb.Labels don't implement sort interface, so we need to use labels.Labels and transform it later
	pbLabels := make([]prompb.Label, 0, len(exportedLabels))
	for _, label := range exportedLabels {
		pbLabels = append(pbLabels, prompb.Label{
			Name:  label.Name,
			Value: label.Value,
		})
	}
	series := &prompb.TimeSeries{
		Labels: pbLabels,
		Samples: []prompb.Sample{
			{
				Value: 0, Timestamp: recordingTime,
			},
		},
	}
	return series
}

func generateExportedLabels(labelsMap map[string]string, rec *recording, externalLabels labels.Labels) labels.Labels {
	exportedLabels := make(labels.Labels, 0, len(externalLabels)+len(rec.rule.ExternalLabels)+len(rec.rule.GroupBy))

	// Add observer's external labels
	for _, label := range externalLabels {
		exportedLabels = append(exportedLabels, label)
	}

	// Add rule's external labels
	for _, label := range rec.rule.ExternalLabels {
		exportedLabels = append(exportedLabels, label)
	}

	// Keep the groupBy labels if present
	for _, label := range rec.rule.GroupBy {
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

func (r *recording) matches(labelsMap map[string]string) bool {
	for _, matcher := range r.rule.Matchers {
		if !matcher.Matches(labelsMap[matcher.Name]) {
			return false
		}
	}
	return true
}
