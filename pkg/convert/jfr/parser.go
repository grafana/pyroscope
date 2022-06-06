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

func ParseJFR(ctx context.Context, s storage.Putter, body io.Reader, pi *storage.PutInput) (err error) {
	chunks, err := parser.Parse(body)
	if err != nil {
		return fmt.Errorf("unable to parse JFR format: %w", err)
	}
	for _, c := range chunks {
		if pErr := parse(ctx, c, s, pi); pErr != nil {
			err = multierror.Append(err, pErr)
		}
	}
	return err
}

func parse(ctx context.Context, c parser.Chunk, s storage.Putter, piOriginal *storage.PutInput) (err error) {
	var event, alloc, lock string
	cpu := tree.New()
	wall := tree.New()
	inTLABObjects := tree.New()
	inTLABBytes := tree.New()
	outTLABObjects := tree.New()
	outTLABBytes := tree.New()
	lockSamples := tree.New()
	lockDuration := tree.New()
	for _, e := range c.Events {
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
		case *parser.ActiveSetting:
			as := e.(*parser.ActiveSetting)
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

	labelsOriginal := piOriginal.Key.Labels()
	prefix := labelsOriginal["__name__"]

	cb := func(n string, t *tree.Tree, u metadata.Units) {
		labels := map[string]string{}
		for k, v := range labelsOriginal {
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
	return err
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
