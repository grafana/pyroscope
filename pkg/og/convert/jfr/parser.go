package jfr

import (
	"fmt"
	"io"

	"github.com/grafana/jfr-parser/parser"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/storage"
)

type stats struct {
}

func ParseJFR(body []byte, pi *storage.PutInput, jfrLabels *LabelsSnapshot) (req *phlaremodel.PushRequest, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("jfr parser panic: %v", r)
		}
	}()
	p := parser.NewParser(body, parser.Options{
		SymbolProcessor: processSymbols,
	})
	return parse(p, pi, jfrLabels)
}

func parse(parser *parser.Parser, piOriginal *storage.PutInput, jfrLabels *LabelsSnapshot) (req *phlaremodel.PushRequest, err error) {
	var event string

	builders := newJfrPprofBuilders(parser, jfrLabels, piOriginal)

	var values = [2]int64{1, 0}

	for {
		typ, err := parser.ParseEvent()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("jfr parser ParseEvent error: %w", err)
		}

		switch typ {
		case parser.TypeMap.T_EXECUTION_SAMPLE:
			ts := parser.GetThreadState(parser.ExecutionSample.State)
			if ts != nil && ts.Name == "STATE_RUNNABLE" {
				builders.addStacktrace(sampleTypeCPU, parser.ExecutionSample.ContextId, parser.ExecutionSample.StackTrace, values[:1])
			}
			if event == "wall" {
				builders.addStacktrace(sampleTypeWall, parser.ExecutionSample.ContextId, parser.ExecutionSample.StackTrace, values[:1])
			}
		case parser.TypeMap.T_ALLOC_IN_NEW_TLAB:
			values[1] = int64(parser.ObjectAllocationInNewTLAB.TlabSize)
			builders.addStacktrace(sampleTypeInTLAB, parser.ObjectAllocationInNewTLAB.ContextId, parser.ObjectAllocationInNewTLAB.StackTrace, values[:2])
		case parser.TypeMap.T_ALLOC_OUTSIDE_TLAB:
			values[1] = int64(parser.ObjectAllocationOutsideTLAB.AllocationSize)
			builders.addStacktrace(sampleTypeOutTLAB, parser.ObjectAllocationOutsideTLAB.ContextId, parser.ObjectAllocationOutsideTLAB.StackTrace, values[:2])
		case parser.TypeMap.T_MONITOR_ENTER:
			values[1] = int64(parser.JavaMonitorEnter.Duration)
			builders.addStacktrace(sampleTypeLock, parser.JavaMonitorEnter.ContextId, parser.JavaMonitorEnter.StackTrace, values[:2])
		case parser.TypeMap.T_THREAD_PARK:
			values[1] = int64(parser.ThreadPark.Duration)
			builders.addStacktrace(sampleTypeThreadPark, parser.ThreadPark.ContextId, parser.ThreadPark.StackTrace, values[:2])
		case parser.TypeMap.T_LIVE_OBJECT:
			builders.addStacktrace(sampleTypeLiveObject, 0, parser.LiveObject.StackTrace, values[:1])
		case parser.TypeMap.T_ACTIVE_SETTING:
			if parser.ActiveSetting.Name == "event" {
				event = parser.ActiveSetting.Value
			}

		}
	}

	req = builders.build(event)

	return req, err
}
