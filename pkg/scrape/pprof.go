// Copyright 2021 The Pyroscope Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scrape

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/labels"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/model"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type pprofWriter struct {
	ingester Ingester
	labels   labels.Labels
	config   *config.Profile

	r *tree.ProfileReader
}

func newPprofWriter(ingester Ingester, target *Target) *pprofWriter {
	w := pprofWriter{
		ingester: ingester,
		labels:   target.Labels(),
		config:   target.profile,
	}
	w.r = tree.NewProfileReader().SampleTypeFilter(w.filterSampleType)
	return &w
}

func (w *pprofWriter) writeProfile(st, et time.Time, p *tree.Profile) error {
	return w.r.Read(p, func(vt *tree.ValueType, l tree.Labels, t *tree.Tree) (keep bool, err error) {
		sampleType := p.StringTable[vt.Type]
		sampleTypeConfig := w.config.SampleTypes[sampleType]
		pi := storage.PutInput{
			StartTime: st,
			EndTime:   et,
			Val:       t,
			SpyName:   "scrape",
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
		w.ingester.Enqueue(&pi)
		return sampleTypeConfig.Cumulative, nil
	})
}

func (w *pprofWriter) filterSampleType(s string) bool {
	_, ok := w.config.SampleTypes[s]
	return ok
}

func (w *pprofWriter) buildName(sampleTypeName string, nameLabels map[string]string) *segment.Key {
	for _, label := range w.labels {
		nameLabels[label.Name] = label.Value
	}
	nameLabels[model.AppNameLabel] += "." + sampleTypeName
	return segment.NewKey(nameLabels)
}

func (w *pprofWriter) reset() {
	w.r.Reset()
}
