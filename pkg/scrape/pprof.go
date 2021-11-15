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
	"strings"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"google.golang.org/protobuf/proto"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/structs/sortedmap"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type pprofWriter struct {
	labels   labels.Labels
	upstream upstream.Upstream

	time  time.Time
	tries triesCache
}

func newPprofWriter(u upstream.Upstream, l labels.Labels) *pprofWriter {
	return &pprofWriter{
		tries:    make(triesCache),
		labels:   l,
		upstream: u,
	}
}

func (w *pprofWriter) Reset() {
	w.time = time.Time{}
	w.tries = make(triesCache)
}

func (w *pprofWriter) WriteProfile(b []byte) error {
	var p convert.Profile
	if err := proto.Unmarshal(b, &p); err != nil {
		return err
	}
	// TimeNanos is the time of collection (UTC) represented
	// as nanoseconds past the epoch reported by profiler.
	profileTime := time.Unix(0, p.TimeNanos).UTC()
	c := make(triesCache)
	for _, st := range p.GetSampleType() {
		pt := spy.ProfileType(p.StringTable[st.Type])
		if pt == spy.ProfileCPU {
			// spy.ProfileType denotes sample type, and spy.ProfileCPU
			// refers to "cpu" sample type which measures in seconds.
			// In Pyroscope, CPU profiles are built from "samples".
			continue
		}
		_ = p.Get(string(pt), func(labels *spy.Labels, name []byte, val int) {
			c.getOrCreate(pt, labels).Insert(name, uint64(val))
		})
		// Remove cache entries for discontinued series.
		for id := range w.tries[pt] {
			if _, ok := c.get(pt, id); !ok {
				w.tries.reset(pt, id)
				continue
			}
		}
		for id, entry := range c[pt] {
			j := &upstream.UploadJob{SpyName: "scrape", Trie: entry.Trie}
			// Cumulative profiles require two consecutive samples,
			// therefore we have to cache this trie.
			if pt.IsCumulative() {
				prev, found := w.tries.get(pt, id)
				w.tries.put(pt, entry)
				if !found {
					continue
				}
				j.Trie = entry.Trie.Diff(prev.Trie)
			}

			// CPU ("samples") sample type. This is the only type
			// that requires SampleRate.
			if pt == "samples" && p.Period > 0 {
				j.SampleRate = uint32(time.Second / time.Duration(p.Period))
			}

			switch {
			case p.DurationNanos > 0:
				j.StartTime = profileTime
				j.EndTime = profileTime.Add(time.Duration(p.DurationNanos))
			case !w.time.IsZero():
				// Without DurationNanos we can not deduce the time range
				// of the profile and therefore need the previous profile time.
				j.StartTime = w.time
				j.EndTime = profileTime
			default:
				continue
			}

			j.Name = w.buildName(pt, entry.labels)
			j.Units = pt.Units()
			j.AggregationType = pt.AggregationType()
			w.upstream.Upload(j)
		}
	}

	w.time = profileTime
	return nil
}

func (w *pprofWriter) buildName(pt spy.ProfileType, l *spy.Labels) string {
	nameLabels := make(map[string]string)
	if l != nil {
		for k, v := range l.Tags() {
			nameLabels[k] = v
		}
	}
	for _, label := range w.labels {
		nameLabels[label.Name] = label.Value
	}
	appName := nameLabels[AppNameLabel]
	if pt == "samples" {
		// Substitute "samples" sample type with "cpu" so as to
		// preserve current UX (basically, "cpu" profile type suffix).
		appName += ".cpu"
	} else {
		appName += "." + string(pt)
	}
	nameLabels[AppNameLabel] = appName
	// TODO(kolesnikovae): a copy of segment.Key.Normalize().
	//   To be refactored once pyroscope model package emerges.
	var sb strings.Builder
	sortedMap := sortedmap.New()
	for k, v := range nameLabels {
		if k == AppNameLabel {
			sb.WriteString(v)
		} else {
			sortedMap.Put(k, v)
		}
	}
	sb.WriteString("{")
	for i, k := range sortedMap.Keys() {
		v := sortedMap.Get(k).(string)
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
	}
	sb.WriteString("}")
	return sb.String()
}

type triesCache map[spy.ProfileType]map[string]*cacheEntry

type cacheEntry struct {
	*transporttrie.Trie
	labels *spy.Labels
}

func (t triesCache) get(pt spy.ProfileType, id string) (*cacheEntry, bool) {
	p, ok := t[pt]
	if !ok {
		return nil, false
	}
	x, ok := p[id]
	return x, ok
}

func (t triesCache) put(pt spy.ProfileType, x *cacheEntry) {
	p, ok := t[pt]
	if !ok {
		p = make(map[string]*cacheEntry)
		t[pt] = p
	}
	p[x.labels.ID()] = x
}

func (t triesCache) getOrCreate(pt spy.ProfileType, l *spy.Labels) *cacheEntry {
	p, ok := t[pt]
	if !ok {
		x := newCacheEntry(l)
		t[pt] = map[string]*cacheEntry{l.ID(): x}
		return x
	}
	x, ok := p[l.ID()]
	if !ok {
		x = newCacheEntry(l)
		p[l.ID()] = x
	}
	return x
}

func newCacheEntry(l *spy.Labels) *cacheEntry {
	return &cacheEntry{Trie: transporttrie.New(), labels: l}
}

func (t triesCache) reset(pt spy.ProfileType, id string) {
	if p, ok := t[pt]; ok {
		delete(p, id)
	}
}
