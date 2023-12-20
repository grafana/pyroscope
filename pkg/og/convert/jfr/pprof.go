package jfr

import (
	"github.com/grafana/jfr-parser/parser"
	"github.com/grafana/jfr-parser/parser/types"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

const (
	sampleTypeCPU        = 0
	sampleTypeWall       = 1
	sampleTypeInTLAB     = 2
	sampleTypeOutTLAB    = 3
	sampleTypeLock       = 4
	sampleTypeThreadPark = 5
	sampleTypeLiveObject = 6
)

func newJfrPprofBuilders(p *parser.Parser, jfrLabels *LabelsSnapshot, piOriginal *storage.PutInput) *jfrPprofBuilders {
	st := piOriginal.StartTime.UnixNano()
	et := piOriginal.EndTime.UnixNano()
	var period int64
	if piOriginal.SampleRate == 0 {
		period = 0
	} else {
		period = 1e9 / int64(piOriginal.SampleRate)
	}

	res := &jfrPprofBuilders{
		parser:        p,
		builders:      make(map[int64]*testhelper.ProfileBuilder),
		labels:        make([]*v1.LabelPair, 0, len(piOriginal.Key.Labels())+5),
		jfrLabels:     jfrLabels,
		timeNanos:     st,
		durationNanos: et - st,
		period:        period,
	}

	for k, v := range piOriginal.Key.Labels() {
		if !phlaremodel.IsLabelAllowedForIngestion(k) {
			continue
		}
		res.labels = append(res.labels, &v1.LabelPair{
			Name:  k,
			Value: v,
		})
	}

	serviceNameLabelName := phlaremodel.LabelNameServiceName
	for _, label := range res.labels {
		if label.Name == serviceNameLabelName {
			serviceNameLabelName = "app_name"
			break
		}
	}

	res.labels = append(res.labels,
		&v1.LabelPair{
			Name:  phlaremodel.LabelNamePyroscopeSpy,
			Value: piOriginal.SpyName,
		},
		&v1.LabelPair{
			Name:  phlaremodel.LabelNameDelta,
			Value: "false",
		},
		&v1.LabelPair{
			Name:  serviceNameLabelName,
			Value: piOriginal.Key.AppName(),
		})

	return res
}

type jfrPprofBuilders struct {
	parser        *parser.Parser
	builders      map[int64]*testhelper.ProfileBuilder
	jfrLabels     *LabelsSnapshot
	labels        []*v1.LabelPair
	timeNanos     int64
	durationNanos int64
	period        int64
}

func (b *jfrPprofBuilders) addStacktrace(sampleType int64, contextID uint64, ref types.StackTraceRef, values []int64) {
	p := b.profileBuilderForSampleType(sampleType)
	st := b.parser.GetStacktrace(ref)
	if st == nil {
		return
	}

	addValues := func(dst []int64) {
		mul := 1
		if sampleType == sampleTypeCPU || sampleType == sampleTypeWall {
			mul = int(b.period)
		}
		for i, value := range values {
			dst[i] += value * int64(mul)
		}
	}

	sample := p.FindExternalSampleWithLabels(uint64(ref), contextID)
	if sample != nil {
		addValues(sample.Value)
		return
	}

	locations := make([]uint64, 0, len(st.Frames))
	for i := 0; i < len(st.Frames); i++ {
		f := st.Frames[i]
		loc, found := p.FindLocationByExternalID(uint32(f.Method))
		if found {
			locations = append(locations, loc)
			continue
		}
		m := b.parser.GetMethod(f.Method)
		if m != nil {

			cls := b.parser.GetClass(m.Type)
			if cls != nil {
				clsName := b.parser.GetSymbolString(cls.Name)
				methodName := b.parser.GetSymbolString(m.Name)
				frame := clsName + "." + methodName
				loc = p.AddExternalFunction(frame, uint32(f.Method))
				locations = append(locations, loc)
			}
			//todo remove Scratch field from the Method
		}
	}
	vs := make([]int64, len(values))
	addValues(vs)
	p.AddExternalSampleWithLabels(locations, vs, contextLabels(contextID, b.jfrLabels), uint64(ref), contextID)
}

func (b *jfrPprofBuilders) profileBuilderForSampleType(sampleType int64) *testhelper.ProfileBuilder {
	if builder, ok := b.builders[sampleType]; ok {
		return builder
	}
	builder := testhelper.NewProfileBuilderWithLabels(b.timeNanos, phlaremodel.Labels(b.labels).Clone())
	builder.DurationNanos = b.durationNanos
	var metric string
	switch sampleType {
	case sampleTypeCPU:
		builder.AddSampleType("cpu", "nanoseconds")
		builder.PeriodType("cpu", "nanoseconds")
		metric = "process_cpu"
	case sampleTypeWall:
		builder.AddSampleType("wall", "nanoseconds")
		builder.PeriodType("wall", "nanoseconds")
		metric = "wall"
	case sampleTypeInTLAB:
		builder.AddSampleType("alloc_in_new_tlab_objects", "count")
		builder.AddSampleType("alloc_in_new_tlab_bytes", "bytes")
		builder.PeriodType("space", "bytes")
		metric = "memory"
	case sampleTypeOutTLAB:
		builder.AddSampleType("alloc_outside_tlab_objects", "count")
		builder.AddSampleType("alloc_outside_tlab_bytes", "bytes")
		builder.PeriodType("space", "bytes")
		metric = "memory"
	case sampleTypeLock:
		builder.AddSampleType("contentions", "count")
		builder.AddSampleType("delay", "nanoseconds")
		builder.PeriodType("mutex", "count")
		metric = "mutex"
	case sampleTypeThreadPark:
		builder.AddSampleType("contentions", "count")
		builder.AddSampleType("delay", "nanoseconds")
		builder.PeriodType("block", "count")
		metric = "block"
	case sampleTypeLiveObject:
		builder.AddSampleType("live", "count")
		builder.PeriodType("objects", "count")
		metric = "memory"
	}
	builder.MetricName(metric)
	b.builders[sampleType] = builder
	return builder
}

func contextLabels(contextID uint64, jfrLabels *LabelsSnapshot) phlaremodel.Labels {
	ctx, ok := jfrLabels.Contexts[int64(contextID)]
	if !ok {
		return nil
	}
	labels := make(phlaremodel.Labels, 0, len(ctx.Labels))
	for k, v := range ctx.Labels {
		labels = append(labels, &v1.LabelPair{
			Name:  jfrLabels.Strings[k],
			Value: jfrLabels.Strings[v],
		})
	}
	return labels
}

func (b *jfrPprofBuilders) build(event string) *model.PushRequest {
	profiles := make([]*model.ProfileSeries, 0, len(b.builders))
	for _, builder := range b.builders {
		profiles = append(profiles, &model.ProfileSeries{
			Labels:  append(builder.Labels, &v1.LabelPair{Name: "jfr_event", Value: event}),
			Samples: []*model.ProfileSample{{Profile: pprof.RawFromProto(builder.Profile)}},
		})
	}
	return &model.PushRequest{Series: profiles}
}
