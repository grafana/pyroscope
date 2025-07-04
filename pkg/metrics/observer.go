package metrics

import (
	"sort"

	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"

	"github.com/grafana/pyroscope/pkg/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type SampleObserver struct {
	state *observerState

	recordingTime  int64
	externalLabels labels.Labels

	exporter Exporter
	ruler    Ruler
}

type observerState struct {
	// tenant state
	tenant     string
	recordings []*recording

	// dataset state
	dataset           string
	targetRecordings  []*recording
	targetStrings     map[string][]*recording
	targetLocations   map[uint32]map[*recording]struct{}
	seenLocations     int
	targetStacktraces map[uint32]map[*recording]struct{}

	// series state
	fingerprint   model.Fingerprint
	recordSymbols bool
}

type recording struct {
	rule  *phlaremodel.RecordingRule
	data  map[model.Fingerprint]*prompb.TimeSeries
	state *recordingState
}

type recordingState struct {
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
		state: &observerState{
			recordings:        make([]*recording, 0),
			targetRecordings:  make([]*recording, 0),
			targetStrings:     make(map[string][]*recording),
			targetLocations:   make(map[uint32]map[*recording]struct{}),
			targetStacktraces: make(map[uint32]map[*recording]struct{}),
		},
	}
}

func (o *SampleObserver) initTenantState(tenant string) {
	o.state.tenant = tenant
	recordingRules := o.ruler.RecordingRules(tenant)

	for _, rule := range recordingRules {
		o.state.recordings = append(o.state.recordings, &recording{
			rule:  rule,
			data:  make(map[model.Fingerprint]*prompb.TimeSeries),
			state: &recordingState{},
		})
	}

	// force a dataset reset
	o.state.dataset = ""
}

func (o *SampleObserver) initDatasetState(dataset string) {
	// New dataset imply new symbols, and new subset of rules that can target the dataset
	o.state.targetStrings = make(map[string][]*recording)
	o.state.targetLocations = make(map[uint32]map[*recording]struct{})
	o.state.targetStacktraces = make(map[uint32]map[*recording]struct{})
	o.state.seenLocations = 0
	o.state.dataset = dataset
	o.state.targetRecordings = o.state.targetRecordings[:0]
	for _, rec := range o.state.recordings {
		// storing the subset of the recording that matter to this dataset:
		if rec.matchesServiceName(dataset) {
			o.state.targetRecordings = append(o.state.targetRecordings, rec)
			if rec.rule.FunctionName != "" {
				// create a lookup for functions names that matter
				if _, exists := o.state.targetStrings[rec.rule.FunctionName]; !exists {
					o.state.targetStrings[rec.rule.FunctionName] = make([]*recording, 0)
				}
				o.state.targetStrings[rec.rule.FunctionName] = append(o.state.targetStrings[rec.rule.FunctionName], rec)
			}
		}
	}
}

// Evaluate manages three kind of states.
//   - Per tenant state:
//     Gets initialized on new tenant. It fetches tenant's rules and creates a new recording for each rule.
//     Data of old state is flushed to the exporter.
//   - Per dataset state:
//     Gets initialized on new dataset. It holds the subset of rules that matter to that dataset, and some set of
//     pointers symbol-to-rule.
//   - Per series (or batch of rows) state:
//     Holds the fingerprint of the series (every batch of rows of the same fingerprint), and whether there's a matching
//     rule that requires symbols to be observed.
//     In addition, the state of every recording is computed, i.e. whether the rule matches the new batch of rows, and
//     a reference of the sample to be aggregated to. (Note that every rule will eventually create multiple single-sample (aggregated) series,
//     depending on the rule.GroupBy space. More info in initState).
//
// This call is not thread-safe
func (o *SampleObserver) Evaluate(row block.ProfileEntry) func() {
	// Detect a tenant switch
	tenant := row.Dataset.TenantID()
	if o.state.tenant != row.Dataset.TenantID() {
		// new tenant to observe, flush data of previous tenant and init new tenant state
		o.flush()
		o.initTenantState(tenant)
	}

	// Detect a dataset switch
	if o.state.dataset != row.Dataset.Name() {
		o.initDatasetState(row.Dataset.Name())
	}

	// Detect a series switch
	if o.state.fingerprint != row.Fingerprint {
		// New series. Handle state.
		o.initSeriesState(row)
	}
	return func() {
		o.observe(row)
	}
}

func (o *SampleObserver) initSeriesState(row block.ProfileEntry) {
	o.state.fingerprint = row.Fingerprint
	o.state.recordSymbols = false

	labelsMap := map[string]string{}
	for _, label := range row.Labels {
		labelsMap[label.Name] = label.Value
	}

	for _, rec := range o.state.targetRecordings {
		rec.initState(labelsMap, o.externalLabels, o.recordingTime)
		if rec.state.matches && rec.rule.FunctionName != "" {
			o.state.recordSymbols = true
		}
	}
}

// ObserveSymbols will skip observation if no rule evaluated true for matchers.
// At the end of this process we'll have a map stacktraceId -> matching rule, so later we can get stacktraces from the
// row and quickly look up for matching rules
func (o *SampleObserver) ObserveSymbols(strings []string, functions []schemav1.InMemoryFunction, locations []schemav1.InMemoryLocation, stacktraceValues [][]int32, stacktraceIds []uint32) {
	if !o.state.recordSymbols {
		return
	}

	for ; o.state.seenLocations < len(locations); o.state.seenLocations++ {
		for _, line := range locations[o.state.seenLocations].Line {
			recs, hit := o.state.targetStrings[strings[functions[line.FunctionId].Name]]
			if hit {
				targetLocation, exists := o.state.targetLocations[uint32(o.state.seenLocations)]
				if !exists {
					targetLocation = make(map[*recording]struct{})
					o.state.targetLocations[uint32(o.state.seenLocations)] = targetLocation
				}
				for _, rec := range recs {
					targetLocation[rec] = struct{}{}
				}
			}
		}
	}
	if len(o.state.targetLocations) == 0 {
		return
	}
	for i, stacktrace := range stacktraceValues {
		for _, locationId := range stacktrace {
			recs, hit := o.state.targetLocations[uint32(locationId)]
			if hit {
				targetStacktrace, exists := o.state.targetStacktraces[stacktraceIds[i]]
				if !exists {
					targetStacktrace = make(map[*recording]struct{})
					o.state.targetStacktraces[stacktraceIds[i]] = targetStacktrace
				}
				for rec := range recs {
					targetStacktrace[rec] = struct{}{}
				}
			}
		}
	}
}

func (o *SampleObserver) observe(row block.ProfileEntry) {
	// Totals are computed as follows: for every rule that matches the series, we add the TotalValue
	for _, rec := range o.state.targetRecordings {
		if rec.state.matches && rec.rule.FunctionName == "" {
			rec.state.sample.Value += float64(row.Row.TotalValue())
		}
	}
	// On the other hand, functions are computed from the lookup tables only if the series hit some rule.
	if o.state.recordSymbols {
		row.Row.ForStacktraceIdsAndValues(func(ids []parquet.Value, values []parquet.Value) {
			for i, id := range ids {
				for rec := range o.state.targetStacktraces[id.Uint32()] {
					if rec.state.matches {
						rec.state.sample.Value += float64(values[i].Int64())
					}
				}
			}
		})
	}
}

func (o *SampleObserver) flush() {
	if len(o.state.recordings) == 0 {
		return
	}
	timeSeries := make([]prompb.TimeSeries, 0)
	for _, rec := range o.state.recordings {
		for _, series := range rec.data {
			timeSeries = append(timeSeries, *series)
		}
	}
	if len(timeSeries) > 0 {
		_ = o.exporter.Send(o.state.tenant, timeSeries)
	}
	o.state.recordings = o.state.recordings[:0]
}

func (o *SampleObserver) Close() {
	o.flush()
}

// initState compute labelsMap for quick lookups. Then check whether row matches the filters
// if filters match, then labels to export are computed, and fetch/create the series where the value needs to be
// aggregated. This state is hold for the following rows with the same fingerprint, so we can observe those faster
func (r *recording) initState(labelsMap map[string]string, externalLabels labels.Labels, recordingTime int64) {
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
	exportedLabels = append(exportedLabels, externalLabels...)
	exportedLabels = append(exportedLabels, rec.rule.ExternalLabels...)
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

func (r *recording) matchesServiceName(dataset string) bool {
	for _, matcher := range r.rule.Matchers {
		if matcher.Name == "service_name" && !matcher.Matches(dataset) {
			return false
		}
	}
	return true
}

func (r *recording) matches(labelsMap map[string]string) bool {
	for _, matcher := range r.rule.Matchers {
		if !matcher.Matches(labelsMap[matcher.Name]) {
			return false
		}
	}
	return true
}
