package jfr

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/grafana/jfr-parser/parser"
	"github.com/grafana/jfr-parser/parser/types"
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
	"github.com/prometheus/prometheus/model/labels"
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

func ParseJFR(ctx context.Context, body []byte, pi *storage.PutInput, jfrLabels *LabelsSnapshot) (profiles []phlaremodel.ParsedProfileSeries, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("jfr parser panic: %v", r)
		}
	}()
	p := parser.NewParser(body, parser.Options{
		SymbolProcessor: processSymbols,
	})
	return parse(ctx, p, pi, jfrLabels)
}

// revive:disable-next-line:cognitive-complexity necessary complexity
func parse(ctx context.Context, c *parser.Parser, piOriginal *storage.PutInput, jfrLabels *LabelsSnapshot) (profiles []phlaremodel.ParsedProfileSeries, err error) {
	var event string

	cache := make(tree.LabelsCache[testhelper.ProfileBuilder])
	type labelsWithHash struct {
		Labels tree.Labels
		Hash   uint64
	}
	contexts := make(map[uint64]labelsWithHash)

	getLabels := func(contextID uint64) labelsWithHash {
		res, ok := contexts[contextID]
		if ok {
			return res
		}
		ls := getContextLabels(int64(contextID), jfrLabels)
		res = labelsWithHash{
			Labels: ls,
			Hash:   ls.Hash(),
		}
		contexts[contextID] = res
		return res
	}

	addStacktrace := func(sampleType int64, contextID uint64, ref types.StackTraceRef, values []int64) {
		lwh := getLabels(contextID)
		e := cache.GetOrCreateTreeByHash(sampleType, lwh.Labels, lwh.Hash)
		if e.Value == nil {
			e.Value = testhelper.NewProfileBuilderWithLabels(0, nil)
		}
		st := c.GetStacktrace(ref)
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
			m := c.GetMethod(f.Method)
			if m != nil {

				cls := c.GetClass(m.Type)
				if cls != nil {
					clsName := c.GetSymbolString(cls.Name)
					methodName := c.GetSymbolString(m.Name)
					frame := clsName + "." + methodName
					loc = e.Value.AddExternalFunction(frame, uint32(f.Method))
					locations = append(locations, loc)
				}
				//todo remove Scratch field from the Method

			}
		}
		vs := make([]int64, len(values))
		copy(vs, values)
		e.Value.AddSample(locations, vs)

	}
	var values = [2]int64{1, 0}

	for {
		typ, err := c.ParseEvent()
		if err != nil {
			if err == io.EOF {
				break
			}
			return profiles, fmt.Errorf("jfr parser ParseEvent error: %w", err)
		}

		switch typ {
		case c.TypeMap.T_EXECUTION_SAMPLE:
			ts := c.GetThreadState(c.ExecutionSample.State) //todo this could be slice instead of hash
			if ts != nil && ts.Name == "STATE_RUNNABLE" {
				addStacktrace(sampleTypeCPU, c.ExecutionSample.ContextId, c.ExecutionSample.StackTrace, values[:1])
			}
			if event == "wall" {
				addStacktrace(sampleTypeWall, c.ExecutionSample.ContextId, c.ExecutionSample.StackTrace, values[:1])
			}
		case c.TypeMap.T_ALLOC_IN_NEW_TLAB:
			values[1] = int64(c.ObjectAllocationInNewTLAB.TlabSize)
			addStacktrace(sampleTypeInTLAB, c.ObjectAllocationInNewTLAB.ContextId, c.ObjectAllocationInNewTLAB.StackTrace, values[:2])
		case c.TypeMap.T_ALLOC_OUTSIDE_TLAB:
			values[1] = int64(c.ObjectAllocationOutsideTLAB.AllocationSize)
			addStacktrace(sampleTypeOutTLAB, c.ObjectAllocationOutsideTLAB.ContextId, c.ObjectAllocationOutsideTLAB.StackTrace, values[:2])
		case c.TypeMap.T_MONITOR_ENTER:
			values[1] = int64(c.JavaMonitorEnter.Duration)
			addStacktrace(sampleTypeLock, c.JavaMonitorEnter.ContextId, c.JavaMonitorEnter.StackTrace, values[:2])
		case c.TypeMap.T_THREAD_PARK:
			values[1] = int64(c.ThreadPark.Duration)
			addStacktrace(sampleTypeThreadPark, c.ThreadPark.ContextId, c.ThreadPark.StackTrace, values[:2])
		case c.TypeMap.T_LIVE_OBJECT:
			addStacktrace(sampleTypeLiveObject, 0, c.LiveObject.StackTrace, values[:1])
		case c.TypeMap.T_ACTIVE_SETTING:
			if c.ActiveSetting.Name == "event" {
				event = c.ActiveSetting.Value
			}

		}
	}

	//todo
	//for sampleType, entries := range cache {
	//	for _, e := range entries {
	//		if i := labelIndex(jfrLabels, e.Labels, segment.ProfileIDLabelName); i != -1 {
	//			cutLabels := tree.CutLabel(e.Labels, i)
	//			cache.GetOrCreateTree(sampleType, cutLabels).Merge(e.Tree)
	//		}
	//	}
	//}

	profiles = make([]phlaremodel.ParsedProfileSeries, 0, len(cache))

	for sampleType, entries := range cache {
		for _, e := range entries {
			e.Value.TimeNanos = piOriginal.StartTime.UnixNano()
			metric := ""
			switch sampleType {
			case sampleTypeCPU:
				e.Value.AddSampleType("cpu", "nanoseconds")
				e.Value.PeriodType("cpu", "nanoseconds")
				e.Value.Period = int64(piOriginal.SampleRate) //todo this should be 1 and values scaled
				metric = "process_cpu"
			case sampleTypeWall:
				e.Value.AddSampleType("cpu", "nanoseconds")
				e.Value.PeriodType("cpu", "nanoseconds")
				e.Value.Period = int64(piOriginal.SampleRate) //todo this should be 1 and values scaled
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
			ls := make([]*v1.LabelPair, 0, len(e.Labels)+len(piOriginal.Key.Labels())+4)
			ls = append(ls, &v1.LabelPair{
				Name:  labels.MetricName,
				Value: metric,
			}, &v1.LabelPair{
				Name:  phlaremodel.LabelNameDelta,
				Value: "false",
			}, &v1.LabelPair{
				Name:  "service_name",
				Value: piOriginal.Key.AppName(),
			}, &v1.LabelPair{
				Name:  "__javaspy_event__",
				Value: event,
			})
			for k, v := range piOriginal.Key.Labels() {
				if strings.HasPrefix(k, "__") {
					continue
				}
				ls = append(ls, &v1.LabelPair{
					Name:  k,
					Value: v,
				})
			}
			for _, label := range e.Labels {
				ks, ok := jfrLabels.Strings[label.Key]
				if !ok {
					continue
				}
				vs, ok := jfrLabels.Strings[label.Str]
				ls = append(ls, &v1.LabelPair{
					Name:  ks,
					Value: vs,
				})
			}
			profiles = append(profiles, phlaremodel.ParsedProfileSeries{
				Labels:  ls,
				Profile: e.Value.Profile,
			})
		}
	}
	return profiles, err
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
