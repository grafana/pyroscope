package jfr

import (
	"context"
	"fmt"
	"io"

	"github.com/hashicorp/go-multierror"
	"github.com/pyroscope-io/jfr-parser/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func ParseJFR(ctx context.Context, s storage.Putter, body io.Reader, pi *storage.PutInput, jfrLabels *LabelsSnapshot) (err error) {
	chunks, err := parser.Parse(body)
	if err != nil {
		return fmt.Errorf("unable to parse JFR format: %w", err)
	}
	for _, c := range chunks {
		if pErr := parse(ctx, c, s, pi, jfrLabels); pErr != nil {
			err = multierror.Append(err, pErr)
		}
	}
	return err
}

// revive:disable-next-line:cognitive-complexity necessary complexity
func parse(ctx context.Context, c parser.Chunk, s storage.Putter, piOriginal *storage.PutInput, jfrLabels *LabelsSnapshot) (err error) {
	var event, alloc, lock string
	for _, e := range c.Events {
		if as, ok := e.(*parser.ActiveSetting); ok {
			switch as.Name {
			case "event":
				event = as.Value
			case "alloc":
				alloc = as.Value
			case "lock":
				lock = as.Value
			}
		}
	}
	for contextID, events := range groupEventsByContextID(c.Events) {
		resolvedJfrLabels := resolveLabels(contextID, jfrLabels)
		cpu := tree.New()
		wall := tree.New()
		inTLABObjects := tree.New()
		inTLABBytes := tree.New()
		outTLABObjects := tree.New()
		outTLABBytes := tree.New()
		lockSamples := tree.New()
		lockDuration := tree.New()
		for _, e := range events {
			switch e.(type) {
			case *parser.ExecutionSample:
				es := e.(*parser.ExecutionSample)
				if fs := frames(es.StackTrace); fs != nil {
					if es.State.Name == "STATE_RUNNABLE" {
						cpu.InsertStackString(fs, 1)
					}
					wall.InsertStackString(fs, 1)
				}
			case *parser.ObjectAllocationInNewTLAB:
				oa := e.(*parser.ObjectAllocationInNewTLAB)
				if fs := frames(oa.StackTrace); fs != nil {
					inTLABObjects.InsertStackString(fs, 1)
					inTLABBytes.InsertStackString(fs, uint64(oa.TLABSize))
				}
			case *parser.ObjectAllocationOutsideTLAB:
				oa := e.(*parser.ObjectAllocationOutsideTLAB)
				if fs := frames(oa.StackTrace); fs != nil {
					outTLABObjects.InsertStackString(fs, 1)
					outTLABBytes.InsertStackString(fs, uint64(oa.AllocationSize))
				}
			case *parser.JavaMonitorEnter:
				jme := e.(*parser.JavaMonitorEnter)
				if fs := frames(jme.StackTrace); fs != nil {
					lockSamples.InsertStackString(fs, 1)
					lockDuration.InsertStackString(fs, uint64(jme.Duration))
				}
			case *parser.ThreadPark:
				tp := e.(*parser.ThreadPark)
				if fs := frames(tp.StackTrace); fs != nil {
					lockSamples.InsertStackString(fs, 1)
					lockDuration.InsertStackString(fs, uint64(tp.Duration))
				}
			}
		}

		labelsOriginal := piOriginal.Key.Labels()
		prefix := labelsOriginal["__name__"]

		cb := func(n string, t *tree.Tree, u metadata.Units) {
			labels := map[string]string{}
			for k, v := range labelsOriginal {
				labels[k] = v
			}
			for k, v := range resolvedJfrLabels {
				labels[k] = v
			}

			labels["__name__"] = prefix + "." + n
			pi := &storage.PutInput{
				StartTime:       piOriginal.StartTime,
				EndTime:         piOriginal.EndTime,
				Key:             segment.NewKey(labels),
				Val:             t,
				SpyName:         piOriginal.SpyName,
				SampleRate:      piOriginal.SampleRate,
				Units:           u,
				AggregationType: metadata.SumAggregationType,
			}
			if putErr := s.Put(ctx, pi); putErr != nil {
				err = multierror.Append(err, putErr)
			}
		}

		if event == "cpu" || event == "itimer" || event == "wall" {
			profile := event
			if event == "wall" {
				profile = "cpu"
			}
			cb(profile, cpu, metadata.SamplesUnits)
		}
		if event == "wall" {
			cb(event, wall, metadata.SamplesUnits)
		}
		if alloc != "" {
			cb("alloc_in_new_tlab_objects", inTLABObjects, metadata.ObjectsUnits)
			cb("alloc_in_new_tlab_bytes", inTLABBytes, metadata.BytesUnits)
			cb("alloc_outside_tlab_objects", outTLABObjects, metadata.ObjectsUnits)
			cb("alloc_outside_tlab_bytes", outTLABBytes, metadata.BytesUnits)
		}
		if lock != "" {
			cb("lock_count", lockSamples, metadata.LockSamplesUnits)
			cb("lock_duration", lockDuration, metadata.LockNanosecondsUnits)
		}
	}
	return err
}

func resolveLabels(contextID int64, labels *LabelsSnapshot) map[string]string {
	res := make(map[string]string)
	if contextID == 0 {
		return res
	}
	var ctx *Context
	var ok bool
	if ctx, ok = labels.Contexts[contextID]; !ok {
		return res
	}
	for k, v := range ctx.Labels {
		var ks string
		var vs string
		if ks, ok = labels.Strings[k]; !ok {
			continue
		}
		if vs, ok = labels.Strings[v]; !ok {
			continue
		}
		res[ks] = vs
	}
	return res
}

func groupEventsByContextID(events []parser.Parseable) map[int64][]parser.Parseable {
	res := make(map[int64][]parser.Parseable)
	for _, e := range events {
		switch e.(type) {
		case *parser.ExecutionSample:
			es := e.(*parser.ExecutionSample)
			res[es.ContextId] = append(res[es.ContextId], e)
		case *parser.ObjectAllocationInNewTLAB:
			oa := e.(*parser.ObjectAllocationInNewTLAB)
			res[oa.ContextId] = append(res[oa.ContextId], e)
		case *parser.ObjectAllocationOutsideTLAB:
			oa := e.(*parser.ObjectAllocationOutsideTLAB)
			res[oa.ContextId] = append(res[oa.ContextId], e)
		case *parser.JavaMonitorEnter:
			jme := e.(*parser.JavaMonitorEnter)
			res[jme.ContextId] = append(res[jme.ContextId], e)
		case *parser.ThreadPark:
			tp := e.(*parser.ThreadPark)
			res[tp.ContextId] = append(res[tp.ContextId], e)
		}
	}
	return res
}

func frames(st *parser.StackTrace) []string {
	if st == nil {
		return nil
	}
	frames := make([]string, 0, len(st.Frames))
	for i := len(st.Frames) - 1; i >= 0; i-- {
		f := st.Frames[i]
		// TODO(abeaumont): Add support for line numbers.
		if f.Method != nil && f.Method.Type != nil && f.Method.Type.Name != nil && f.Method.Name != nil {
			frames = append(frames, f.Method.Type.Name.String+"."+f.Method.Name.String)
		}
	}
	return frames
}
