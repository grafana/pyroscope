package jfr

import (
	"github.com/grafana/pyroscope/pkg/distributor/model"

	"github.com/grafana/jfr-parser/parser"
	"github.com/grafana/jfr-parser/parser/types"
	"github.com/prometheus/prometheus/model/labels"

	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

const (
	sampleTypeCPU  = 0
	sampleTypeWall = 1

	sampleTypeInTLAB = 2

	sampleTypeOutTLAB = 3

	sampleTypeLock = 4

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
		timeNanos:     st,
		durationNanos: et - st,
		labels:        make([]*v1.LabelPair, 0, len(piOriginal.Key.Labels())+5),
		period:        period,
		appName:       piOriginal.Key.AppName(),
		spyName:       piOriginal.SpyName,

		parser: p,
		cache: tree.NewLabelsCache[testhelper.ProfileBuilder](func() *testhelper.ProfileBuilder {
			return testhelper.NewProfileBuilderWithLabels(0, nil)
		}),
		contexts:  make(map[uint64]labelsWithHash),
		baseline:  make(map[uint64]labelsWithHash),
		jfrLabels: jfrLabels,
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

	return res
}

type labelsWithHash struct {
	Labels tree.Labels
	Hash   uint64
}
type jfrPprofBuilders struct {
	timeNanos     int64
	durationNanos int64
	labels        []*v1.LabelPair
	appName       string
	spyName       string

	parser *parser.Parser
	cache  tree.LabelsCache[testhelper.ProfileBuilder]

	contexts map[uint64]labelsWithHash
	baseline map[uint64]labelsWithHash // without profile_id

	jfrLabels *LabelsSnapshot
	period    int64
}

func (b *jfrPprofBuilders) getLabels(contextID uint64) labelsWithHash {
	res, ok := b.contexts[contextID]
	if ok {
		return res
	}
	ls := getContextLabels(int64(contextID), b.jfrLabels)
	res = labelsWithHash{
		Labels: ls,
		Hash:   ls.Hash(),
	}

	b.contexts[contextID] = res

	if i := labelIndex(b.jfrLabels, ls, segment.ProfileIDLabelName); i != -1 {
		cutLabels := tree.CutLabel(ls, i)
		b.baseline[contextID] = labelsWithHash{
			Labels: cutLabels,
			Hash:   cutLabels.Hash(),
		}
	}

	return res
}

func (b *jfrPprofBuilders) addStacktrace(sampleType int64, contextID uint64, ref types.StackTraceRef, values []int64) {
	lwh := b.getLabels(contextID)
	b.addStacktraceImpl(sampleType, lwh, ref, values)

	lwhBaseline, ok := b.baseline[contextID]
	if ok {
		b.addStacktraceImpl(sampleType, lwhBaseline, ref, values)
	}
}

func (b *jfrPprofBuilders) addStacktraceImpl(sampleType int64, lwh labelsWithHash, ref types.StackTraceRef, values []int64) {
	e := b.cache.GetOrCreateTreeByHash(sampleType, lwh.Labels, lwh.Hash)
	st := b.parser.GetStacktrace(ref)
	if st == nil {
		return
	}

	locations := make([]uint64, 0, len(st.Frames))
	for i := 0; i < len(st.Frames); i++ {
		f := st.Frames[i]
		loc, found := e.Value.FindLocationByExternalID(uint32(f.Method))
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
				loc = e.Value.AddExternalFunction(frame, uint32(f.Method))
				locations = append(locations, loc)
			}
			//todo remove Scratch field from the Method

		}
	}
	vs := make([]int64, len(values))
	copy(vs, values)
	if sampleType == sampleTypeCPU || sampleType == sampleTypeWall {
		vs[0] *= b.period
	}
	e.Value.AddSample(locations, vs)

}

func (b *jfrPprofBuilders) build(event string) *model.PushRequest {
	profiles := make([]*model.ProfileSeries, 0, len(b.cache.Map))

	for sampleType, entries := range b.cache.Map {
		for _, e := range entries {
			e.Value.TimeNanos = b.timeNanos
			e.Value.DurationNanos = b.durationNanos
			metric := ""
			switch sampleType {
			case sampleTypeCPU:
				e.Value.AddSampleType("cpu", "nanoseconds")
				e.Value.PeriodType("cpu", "nanoseconds")
				metric = "process_cpu"
			case sampleTypeWall:
				e.Value.AddSampleType("wall", "nanoseconds")
				e.Value.PeriodType("wall", "nanoseconds")
				metric = "wall"
			case sampleTypeInTLAB:
				e.Value.AddSampleType("alloc_in_new_tlab_objects", "count")
				e.Value.AddSampleType("alloc_in_new_tlab_bytes", "bytes")
				metric = "memory"
			case sampleTypeOutTLAB:
				e.Value.AddSampleType("alloc_outside_tlab_objects", "count")
				e.Value.AddSampleType("alloc_outside_tlab_bytes", "bytes")
				metric = "memory"
			case sampleTypeLock:
				e.Value.AddSampleType("contentions", "count")
				e.Value.AddSampleType("delay", "nanoseconds")
				metric = "mutex"
			case sampleTypeThreadPark:
				e.Value.AddSampleType("contentions", "count")
				e.Value.AddSampleType("delay", "nanoseconds")
				metric = "block"
			case sampleTypeLiveObject:
				e.Value.AddSampleType("live", "count")
				metric = "memory"
			}
			ls := make([]*v1.LabelPair, 0, len(e.Labels)+len(b.labels)+5)
			ls = append(ls, &v1.LabelPair{
				Name:  labels.MetricName,
				Value: metric,
			}, &v1.LabelPair{
				Name:  phlaremodel.LabelNameDelta,
				Value: "false",
			}, &v1.LabelPair{
				Name:  "jfr_event",
				Value: event,
			}, &v1.LabelPair{
				Name:  "pyroscope_spy",
				Value: b.spyName,
			})
			for _, v := range b.labels {
				ls = append(ls, v)
			}
			for _, label := range e.Labels {
				ks, ok := b.jfrLabels.Strings[label.Key]
				if !ok {
					continue
				}
				vs, ok := b.jfrLabels.Strings[label.Str]
				ls = append(ls, &v1.LabelPair{
					Name:  ks,
					Value: vs,
				})
			}
			serviceNameLabelName := "service_name"
			for _, label := range ls {
				if label.Name == serviceNameLabelName {
					serviceNameLabelName = "app_name"
					break
				}
			}
			ls = append(ls, &v1.LabelPair{
				Name:  serviceNameLabelName,
				Value: b.appName,
			})
			profiles = append(profiles, &model.ProfileSeries{
				Labels: ls,
				Samples: []*model.ProfileSample{
					{
						Profile:    pprof.RawFromProto(e.Value.Profile),
						RawProfile: nil,
						ID:         "",
					},
				},
			})
		}
	}
	return &model.PushRequest{
		Series: profiles,
	}
}

func getContextLabels(contextID int64, labels *LabelsSnapshot) tree.Labels {
	if contextID == 0 {
		return nil
	}
	var ctx *Context
	var ok bool
	if ctx, ok = labels.Contexts[contextID]; !ok {
		return nil
	}
	res := make(tree.Labels, 0, len(ctx.Labels))
	for k, v := range ctx.Labels {
		res = append(res, &tree.Label{Key: k, Str: v})
	}
	return res
}
func labelIndex(s *LabelsSnapshot, labels tree.Labels, key string) int {
	for i, label := range labels {
		if n, ok := s.Strings[label.Key]; ok {
			if n == key {
				return i
			}
		}
	}
	return -1
}
