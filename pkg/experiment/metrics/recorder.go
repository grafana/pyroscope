package metrics

import (
	"sort"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type Recorder struct {
	Recordings        []*Recording
	labelsMaps        map[model.Fingerprint]map[string]string
	recordingTime     int64
	pyroscopeInstance string
}

type Recording struct {
	rule RecordingRule
	fps  map[model.Fingerprint]*AggregatedFingerprint
	data map[AggregatedFingerprint]*TimeSeries
}

type AggregatedFingerprint model.Fingerprint

type TimeSeries struct {
	Labels  labels.Labels
	Samples []Sample
}

type Sample struct {
	Value     float64
	Timestamp int64
}

func NewRecorder(recordingRules []*RecordingRule, recordingTime int64, pyroscopeInstance string) *Recorder {
	recordings := make([]*Recording, len(recordingRules))
	for i, rule := range recordingRules {
		recordings[i] = &Recording{
			rule: *rule,
			fps:  make(map[model.Fingerprint]*AggregatedFingerprint),
			data: make(map[AggregatedFingerprint]*TimeSeries),
		}
	}
	return &Recorder{
		Recordings:        recordings,
		labelsMaps:        make(map[model.Fingerprint]map[string]string),
		recordingTime:     recordingTime,
		pyroscopeInstance: pyroscopeInstance,
	}
}

func (r *Recorder) RecordRow(fp model.Fingerprint, lbls phlaremodel.Labels, totalValue int64) {
	labelsMap := r.getOrCreateLabelsMap(fp, lbls)

	for _, recording := range r.Recordings {
		aggregatedFp, matches := recording.matches(fp, labelsMap)
		if !matches {
			continue
		}
		if aggregatedFp == nil {
			// first time this series appears
			exportedLabels := r.generateExportedLabels(labelsMap, recording)

			sort.Sort(exportedLabels)
			f := AggregatedFingerprint(exportedLabels.Hash())
			aggregatedFp = &f

			recording.fps[fp] = aggregatedFp
			_, ok := recording.data[*aggregatedFp]
			if !ok {
				recording.data[*aggregatedFp] = newTimeSeries(exportedLabels, r.recordingTime)
			}
		}
		recording.data[*aggregatedFp].Samples[0].Value += float64(totalValue)
	}
}

func newTimeSeries(exportedLabels labels.Labels, time int64) *TimeSeries {
	return &TimeSeries{
		Labels: exportedLabels,
		Samples: []Sample{
			{
				Value:     float64(0),
				Timestamp: time,
			},
		},
	}
}

func (r *Recorder) generateExportedLabels(labelsMap map[string]string, rec *Recording) labels.Labels {
	exportedLabels := labels.Labels{
		labels.Label{
			Name:  "__name__",
			Value: rec.rule.metricName,
		},
		labels.Label{
			Name:  "__pyroscope_instance__",
			Value: r.pyroscopeInstance,
		},
	}
	// Add filters as exported labels
	for _, matcher := range rec.rule.matchers {
		exportedLabels = append(exportedLabels, labels.Label{
			Name:  matcher.Name,
			Value: matcher.Value,
		})
	}
	// Keep the expected labels
	for _, label := range rec.rule.keepLabels {
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

func (r *Recording) matches(fp model.Fingerprint, labelsMap map[string]string) (*AggregatedFingerprint, bool) {
	aggregatedFp, seen := r.fps[fp]
	if seen {
		// we've seen this series before
		return aggregatedFp, seen
	}
	if r.rule.profileType != labelsMap["__profile_type__"] {
		return nil, false
	}
	for _, matcher := range r.rule.matchers {
		// assume labels.MatchEqual for every matcher:
		if labelsMap[matcher.Name] != matcher.Value {
			return nil, false
		}
	}
	return nil, true
}

func (r *Recorder) getOrCreateLabelsMap(fp model.Fingerprint, lbls phlaremodel.Labels) map[string]string {
	// get or populate label map
	labelsMap, ok := r.labelsMaps[fp]
	if !ok {
		labelsMap = map[string]string{}
		for _, label := range lbls {
			labelsMap[label.Name] = label.Value
		}
		r.labelsMaps[fp] = labelsMap
	}
	return labelsMap
}
