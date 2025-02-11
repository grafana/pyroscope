package metrics

import (
	"sort"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type Recorder struct {
	Recordings        []*Recording
	recordingTime     int64
	pyroscopeInstance string
}

type Recording struct {
	rule  RecordingRule
	data  map[AggregatedFingerprint]*Sample
	state *recordingState
}

type recordingState struct {
	fp      *model.Fingerprint
	matches bool
	sample  *Sample
}

func (r *Recording) InitState(fp model.Fingerprint, lbls phlaremodel.Labels, pyroscopeInstance string, recordingTime int64) {
	r.state.fp = &fp
	labelsMap := map[string]string{}
	for _, label := range lbls {
		labelsMap[label.Name] = label.Value
	}
	r.state.matches = r.matches(labelsMap)
	if !r.state.matches {
		return
	}

	exportedLabels := generateExportedLabels(labelsMap, r, pyroscopeInstance)
	sort.Sort(exportedLabels)
	aggregatedFp := AggregatedFingerprint(exportedLabels.Hash())
	sample, ok := r.data[aggregatedFp]
	if !ok {
		sample = newSample(exportedLabels, recordingTime)
		r.data[aggregatedFp] = sample
	}
	r.state.sample = sample
}

type AggregatedFingerprint model.Fingerprint

type Sample struct {
	Labels    labels.Labels
	Value     float64
	Timestamp int64
}

func NewRecorder(recordingRules []*RecordingRule, recordingTime int64, pyroscopeInstance string) *Recorder {
	recordings := make([]*Recording, len(recordingRules))
	for i, rule := range recordingRules {
		recordings[i] = &Recording{
			rule: *rule,
			data: make(map[AggregatedFingerprint]*Sample),
			state: &recordingState{
				fp: nil,
			},
		}
	}
	return &Recorder{
		Recordings:        recordings,
		recordingTime:     recordingTime,
		pyroscopeInstance: pyroscopeInstance,
	}
}

func (r *Recorder) RecordRow(fp model.Fingerprint, lbls phlaremodel.Labels, totalValue int64) {
	for _, recording := range r.Recordings {
		if recording.state.fp == nil || *recording.state.fp != fp {
			recording.InitState(fp, lbls, r.pyroscopeInstance, r.recordingTime)
		}
		if !recording.state.matches {
			continue
		}
		recording.state.sample.Value += float64(totalValue)
	}
}

func newSample(exportedLabels labels.Labels, time int64) *Sample {
	return &Sample{
		Labels:    exportedLabels,
		Value:     float64(0),
		Timestamp: time,
	}
}

func generateExportedLabels(labelsMap map[string]string, rec *Recording, pyroscopeInstance string) labels.Labels {
	exportedLabels := labels.Labels{
		labels.Label{
			Name:  "__name__",
			Value: rec.rule.metricName,
		},
		labels.Label{
			Name:  "pyroscope_instance",
			Value: pyroscopeInstance,
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

func (r *Recording) matches(labelsMap map[string]string) bool {
	if r.rule.profileType != labelsMap["__profile_type__"] {
		return false
	}
	for _, matcher := range r.rule.matchers {
		if !matcher.Matches(labelsMap[matcher.Name]) {
			return false
		}
	}
	return true
}
