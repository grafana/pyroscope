package testhelper

import (
	"github.com/google/uuid"
	"github.com/prometheus/common/model"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/labels"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
)

func NewProfileSchema(p *profilev1.Profile, name string) ([]schemav1.InMemoryProfile, []phlaremodel.Labels) {
	var (
		lbls, seriesRefs = labels.CreateProfileLabels(p, &typesv1.LabelPair{Name: model.MetricNameLabel, Value: name})
		ps               = make([]schemav1.InMemoryProfile, len(lbls))
	)
	for idxType := range lbls {
		ps[idxType] = schemav1.InMemoryProfile{
			ID:                uuid.New(),
			TimeNanos:         p.TimeNanos,
			Comments:          p.Comment,
			DurationNanos:     p.DurationNanos,
			DropFrames:        p.DropFrames,
			KeepFrames:        p.KeepFrames,
			Period:            p.Period,
			DefaultSampleType: p.DefaultSampleType,
		}
		hashes := pprof.SampleHasher{}.Hashes(p.Sample)
		ps[idxType].Samples = schemav1.Samples{
			StacktraceIDs: make([]uint32, len(p.Sample)),
			Values:        make([]uint64, len(p.Sample)),
		}
		for i, s := range p.Sample {
			ps[idxType].Samples.Values[i] = uint64(s.Value[idxType])
			ps[idxType].Samples.StacktraceIDs[i] = uint32(hashes[i])

		}
		ps[idxType].SeriesFingerprint = seriesRefs[idxType]
	}
	return ps, lbls
}
