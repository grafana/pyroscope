package jfr

import (
	"context"
	"fmt"
	"io"

	"github.com/grafana/jfr-parser/parser"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/storage"
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

func parse(ctx context.Context, c *parser.Parser, piOriginal *storage.PutInput, jfrLabels *LabelsSnapshot) (profiles []phlaremodel.ParsedProfileSeries, err error) {
	var event string

	builders := newJfrPprofBuilders(c, jfrLabels, piOriginal)

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
			ts := c.GetThreadState(c.ExecutionSample.State)
			if ts != nil && ts.Name == "STATE_RUNNABLE" {
				builders.addStacktrace(sampleTypeCPU, c.ExecutionSample.ContextId, c.ExecutionSample.StackTrace, values[:1])
			}
			if event == "wall" {
				builders.addStacktrace(sampleTypeWall, c.ExecutionSample.ContextId, c.ExecutionSample.StackTrace, values[:1])
			}
		case c.TypeMap.T_ALLOC_IN_NEW_TLAB:
			values[1] = int64(c.ObjectAllocationInNewTLAB.TlabSize)
			builders.addStacktrace(sampleTypeInTLAB, c.ObjectAllocationInNewTLAB.ContextId, c.ObjectAllocationInNewTLAB.StackTrace, values[:2])
		case c.TypeMap.T_ALLOC_OUTSIDE_TLAB:
			values[1] = int64(c.ObjectAllocationOutsideTLAB.AllocationSize)
			builders.addStacktrace(sampleTypeOutTLAB, c.ObjectAllocationOutsideTLAB.ContextId, c.ObjectAllocationOutsideTLAB.StackTrace, values[:2])
		case c.TypeMap.T_MONITOR_ENTER:
			values[1] = int64(c.JavaMonitorEnter.Duration)
			builders.addStacktrace(sampleTypeLock, c.JavaMonitorEnter.ContextId, c.JavaMonitorEnter.StackTrace, values[:2])
		case c.TypeMap.T_THREAD_PARK:
			values[1] = int64(c.ThreadPark.Duration)
			builders.addStacktrace(sampleTypeThreadPark, c.ThreadPark.ContextId, c.ThreadPark.StackTrace, values[:2])
		case c.TypeMap.T_LIVE_OBJECT:
			builders.addStacktrace(sampleTypeLiveObject, 0, c.LiveObject.StackTrace, values[:1])
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
	profiles = builders.build(event)

	return profiles, err
}
