package testhelper

import (
	"github.com/google/uuid"
	"github.com/prometheus/common/model"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/labels"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func NewProfileSchema(builder *testhelper.ProfileBuilder, name string) ([]schemav1.InMemoryProfile, []phlaremodel.Labels) {
	var (
		p                = builder.Profile
		lbls, seriesRefs = labels.CreateProfileLabels(true, p, &typesv1.LabelPair{Name: model.MetricNameLabel, Value: name})
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
			Annotations: schemav1.Annotations{
				Keys:   make([]string, 0),
				Values: make([]string, 0),
			},
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
		for _, a := range builder.Annotations {
			ps[idxType].Annotations.Keys = append(ps[idxType].Annotations.Keys, a.Key)
			ps[idxType].Annotations.Values = append(ps[idxType].Annotations.Values, a.Value)
		}
	}
	return ps, lbls
}
