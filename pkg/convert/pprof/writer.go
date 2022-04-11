package pprof

import (
	"context"
	"fmt"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type Ingester interface {
	Enqueue(context.Context, *storage.PutInput)
}

type ProfileWriter struct {
	ingester    Ingester
	spyName     string
	labels      map[string]string
	sampleTypes map[string]*tree.SampleTypeConfig

	r *ProfileReader
}

type ProfileWriterConfig struct {
	SpyName     string
	Labels      map[string]string
	SampleTypes map[string]*tree.SampleTypeConfig
}

func NewProfileWriter(ingester Ingester, config ProfileWriterConfig) *ProfileWriter {
	w := ProfileWriter{
		ingester:    ingester,
		spyName:     config.SpyName,
		labels:      config.Labels,
		sampleTypes: config.SampleTypes,
	}
	w.r = NewProfileReader().SampleTypeFilter(w.filterSampleType)
	return &w
}

func (w *ProfileWriter) Reset() { w.r.Reset() }

func (w *ProfileWriter) WriteProfile(ctx context.Context, startTime, endTime time.Time, p *tree.Profile) error {
	return w.r.Read(p, func(vt *tree.ValueType, l tree.Labels, t *tree.Tree) (keep bool, err error) {
		if vt.Type >= int64(len(p.StringTable)) {
			return false, fmt.Errorf("sample value type is invalid")
		}
		sampleType := p.StringTable[vt.Type]
		sampleTypeConfig, ok := w.sampleTypes[sampleType]
		if !ok {
			return false, fmt.Errorf("sample value type is unknown")
		}
		pi := storage.PutInput{
			StartTime: startTime,
			EndTime:   endTime,
			SpyName:   w.spyName,
			Val:       t,
		}
		// Cumulative profiles require two consecutive samples,
		// therefore we have to cache this trie.
		if sampleTypeConfig.Cumulative {
			prev, found := w.r.Load(vt.Type, l)
			if !found {
				// Keep the current entry in cache.
				return true, nil
			}
			// Take diff with the previous tree.
			// The result is written to prev, t is not changed.
			pi.Val = prev.Diff(t)
		}
		pi.AggregationType = sampleTypeConfig.Aggregation
		if sampleTypeConfig.Sampled {
			pi.SampleRate = sampleRate(p)
		}
		if sampleTypeConfig.DisplayName != "" {
			sampleType = sampleTypeConfig.DisplayName
		}
		if sampleTypeConfig.Units != "" {
			pi.Units = sampleTypeConfig.Units
		} else {
			pi.Units = p.StringTable[vt.Unit]
		}
		pi.Key = w.buildName(sampleType, p.ResolveLabels(l))
		w.ingester.Enqueue(ctx, &pi)
		return sampleTypeConfig.Cumulative, nil
	})
}

func sampleRate(p *tree.Profile) uint32 {
	if p.Period <= 0 || p.PeriodType == nil {
		return 0
	}
	sampleUnit := time.Nanosecond
	switch p.StringTable[p.PeriodType.Unit] {
	case "microseconds":
		sampleUnit = time.Microsecond
	case "milliseconds":
		sampleUnit = time.Millisecond
	case "seconds":
		sampleUnit = time.Second
	}
	return uint32(time.Second / (sampleUnit * time.Duration(p.Period)))
}

func (w *ProfileWriter) filterSampleType(s string) bool {
	_, ok := w.sampleTypes[s]
	return ok
}

func (w *ProfileWriter) buildName(sampleTypeName string, labels map[string]string) *segment.Key {
	for k, v := range w.labels {
		labels[k] = v
	}
	labels["__name__"] += "." + sampleTypeName
	return segment.NewKey(labels)
}
