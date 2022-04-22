package jfr

import (
	"context"
	"fmt"
	"io"

	"github.com/hashicorp/go-multierror"
	"github.com/pyroscope-io/jfr-parser/parser"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

func ParseJFR(ctx context.Context, r io.Reader, s storage.Putter, pi *storage.PutInput) (err error) {
	chunks, err := parser.Parse(r)
	if err != nil {
		return fmt.Errorf("unable to parse JFR format: %w", err)
	}
	for _, c := range chunks {
		if pErr := parse(ctx, c, s, pi); pErr != nil {
			err = multierror.Append(err, pErr)
		}
	}
	pi.Val = nil
	return err
}

func parse(ctx context.Context, c parser.Chunk, s storage.Putter, pi *storage.PutInput) (err error) {
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
	labels := pi.Key.Labels()
	prefix := labels["__name__"]
	if event == "cpu" || event == "itimer" || event == "wall" {
		profile := event
		if event == "wall" {
			profile = "cpu"
		}
		labels["__name__"] = prefix + "." + profile
		pi.Key = segment.NewKey(labels)
		pi.Val = cpu
		pi.Units = metadata.SamplesUnits
		pi.AggregationType = metadata.SumAggregationType
		if putErr := s.Put(ctx, pi); err != nil {
			err = multierror.Append(err, putErr)
		}
	}
	if event == "wall" {
		labels["__name__"] = prefix + "." + event
		pi.Key = segment.NewKey(labels)
		pi.Val = wall
		pi.Units = metadata.SamplesUnits
		pi.AggregationType = metadata.SumAggregationType
		if putErr := s.Put(ctx, pi); err != nil {
			err = multierror.Append(err, putErr)
		}
	}
	if alloc != "" {
		labels["__name__"] = prefix + ".alloc_in_new_tlab_objects"
		pi.Key = segment.NewKey(labels)
		pi.Val = inTLABObjects
		pi.Units = metadata.ObjectsUnits
		pi.AggregationType = metadata.SumAggregationType
		if putErr := s.Put(ctx, pi); err != nil {
			err = multierror.Append(err, putErr)
		}
		labels["__name__"] = prefix + ".alloc_in_new_tlab_bytes"
		pi.Key = segment.NewKey(labels)
		pi.Val = inTLABObjects
		pi.Units = metadata.BytesUnits
		pi.AggregationType = metadata.SumAggregationType
		if putErr := s.Put(ctx, pi); err != nil {
			err = multierror.Append(err, putErr)
		}
		labels["__name__"] = prefix + ".alloc_outside_tlab_objects"
		pi.Key = segment.NewKey(labels)
		pi.Val = inTLABObjects
		pi.Units = metadata.ObjectsUnits
		pi.AggregationType = metadata.SumAggregationType
		if putErr := s.Put(ctx, pi); err != nil {
			err = multierror.Append(err, putErr)
		}
		labels["__name__"] = prefix + ".alloc_outside_tlab_bytes"
		pi.Key = segment.NewKey(labels)
		pi.Val = inTLABObjects
		pi.Units = metadata.BytesUnits
		pi.AggregationType = metadata.SumAggregationType
		if putErr := s.Put(ctx, pi); err != nil {
			err = multierror.Append(err, putErr)
		}
	}
	if lock != "" {
		labels["__name__"] = prefix + ".lock_samples"
		pi.Key = segment.NewKey(labels)
		pi.Val = lockSamples
		pi.Units = metadata.LockSamplesUnits
		pi.AggregationType = metadata.SumAggregationType
		if putErr := s.Put(ctx, pi); err != nil {
			err = multierror.Append(err, putErr)
		}
		labels["__name__"] = prefix + ".lock_nanoseconds"
		pi.Key = segment.NewKey(labels)
		pi.Val = lockDuration
		pi.Units = metadata.LockNanosecondsUnits
		pi.AggregationType = metadata.SumAggregationType
		if putErr := s.Put(ctx, pi); err != nil {
			err = multierror.Append(err, putErr)
		}
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
