package metrics

import (
	"fmt"
	"sort"

	"github.com/cespare/xxhash/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type SampleObserver struct {
	exporter          *Exporter
	tenant            string
	Recordings        []*Recording
	recordingTime     int64
	pyroscopeInstance string
	logger            log.Logger
}

type Recording struct {
	rule  RecordingRule
	data  map[model.Fingerprint]*Sample
	state *recordingState
}

type recordingState struct {
	fp      *model.Fingerprint
	matches bool
	sample  *Sample
}

func NewSampleObserver(meta *metastorev1.BlockMeta, logger log.Logger) *SampleObserver {
	recordingTime := int64(ulid.MustParse(meta.Id).Time())
	pyroscopeInstance := pyroscopeInstanceHash(meta.Shard, meta.CreatedBy)
	return &SampleObserver{
		recordingTime:     recordingTime,
		pyroscopeInstance: pyroscopeInstance,
		logger:            logger,
		exporter:          &Exporter{},
	}
}

func pyroscopeInstanceHash(shard uint32, createdBy int32) string {
	buf := make([]byte, 0, 8)
	buf = append(buf, byte(shard>>24), byte(shard>>16), byte(shard>>8), byte(shard))
	buf = append(buf, byte(createdBy>>24), byte(createdBy>>16), byte(createdBy>>8), byte(createdBy))
	return fmt.Sprintf("%x", xxhash.Sum64(buf))
}

func (o *SampleObserver) Init(tenant string) {
	o.tenant = tenant
	recordingRules := recordingRulesFromTenant(o.tenant)
	o.Recordings = make([]*Recording, len(recordingRules))
	for i, rule := range recordingRules {
		o.Recordings[i] = &Recording{
			rule: *rule,
			data: make(map[model.Fingerprint]*Sample),
			state: &recordingState{
				fp: nil,
			},
		}
	}

}

func (o *SampleObserver) Observe(row block.ProfileEntry) {
	for _, recording := range o.Recordings {
		if recording.state.fp == nil || *recording.state.fp != row.Fingerprint {
			recording.InitState(row.Fingerprint, row.Labels, o.pyroscopeInstance, o.recordingTime)
		}
		if !recording.state.matches {
			continue
		}
		recording.state.sample.Value += float64(row.Row.TotalValue())
	}
}

func (o *SampleObserver) Flush() error {
	recs := o.Recordings
	o.Recordings = nil
	go func(tenant string, recordings []*Recording) {
		timeSeries := make([]prompb.TimeSeries, 0)
		for _, recording := range recordings {
			for _, sample := range recording.data {
				ts := prompb.TimeSeries{
					Labels: make([]prompb.Label, 0, len(sample.Labels)),
					Samples: []prompb.Sample{
						{
							Value:     sample.Value,
							Timestamp: sample.Timestamp,
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

		if err := o.exporter.Send(tenant, timeSeries); err != nil {
			level.Error(o.logger).Log("msg", "error sending recording metrics", "err", err)
		}
	}(o.tenant, recs)
	return nil
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
	aggregatedFp := model.Fingerprint(exportedLabels.Hash())
	sample, ok := r.data[aggregatedFp]
	if !ok {
		sample = newSample(exportedLabels, recordingTime)
		r.data[aggregatedFp] = sample
	}
	r.state.sample = sample
}

type Sample struct {
	Labels    labels.Labels
	Value     float64
	Timestamp int64
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
