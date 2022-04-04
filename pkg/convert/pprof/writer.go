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
	ingester Ingester
	labels   map[string]string
	config   map[string]*tree.SampleTypeConfig

	r *ProfileReader
}

func NewProfileWriter(ingester Ingester, labels map[string]string, config map[string]*tree.SampleTypeConfig) *ProfileWriter {
	w := ProfileWriter{
		ingester: ingester,
		labels:   labels,
		config:   config,
	}
	w.r = NewProfileReader().SampleTypeFilter(w.filterSampleType)
	return &w
}

func (w *ProfileWriter) Reset() { w.r.Reset() }

func (w *ProfileWriter) WriteProfile(ctx context.Context, startTime, endTime time.Time, spyName string, p *tree.Profile) error {
	return w.r.Read(p, func(vt *tree.ValueType, l tree.Labels, t *tree.Tree) (keep bool, err error) {
		if vt.Type >= int64(len(p.StringTable)) {
			return false, fmt.Errorf("sample value type is invalid")
		}
		sampleType := p.StringTable[vt.Type]
		sampleTypeConfig, ok := w.config[sampleType]
		if !ok {
			return false, fmt.Errorf("sample value type is unknown")
		}
		pi := storage.PutInput{
			StartTime: startTime,
			EndTime:   endTime,
			SpyName:   spyName,
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
		if sampleTypeConfig.Sampled && p.Period > 0 {
			pi.SampleRate = uint32(time.Second / time.Duration(p.Period))
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

func (w *ProfileWriter) filterSampleType(s string) bool {
	_, ok := w.config[s]
	return ok
}

func (w *ProfileWriter) buildName(sampleTypeName string, labels map[string]string) *segment.Key {
	for k, v := range w.labels {
		labels[k] = v
	}
	labels["__name__"] += "." + sampleTypeName
	return segment.NewKey(labels)
}
