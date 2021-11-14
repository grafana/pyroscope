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

	"github.com/prometheus/prometheus/pkg/labels"
	"google.golang.org/protobuf/proto"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
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
	w.tries = nil
	w.time = time.Time{}
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
			// TODO(kolesnikovae): get spy name from profile?
			j := &upstream.UploadJob{SpyName: "scrape", Trie: entry.Trie}
			// CPU ("samples") sample type. This is the only type
			// that requires SampleRate.
			if pt == "samples" && p.Period > 0 {
				j.SampleRate = uint32(time.Second / time.Duration(p.Period))
			}
			// Without DurationNanos we can not deduce the time range
			// of the profile and need the previous profile time.
			if p.DurationNanos > 0 {
				j.StartTime = profileTime
				j.EndTime = profileTime.Add(time.Duration(p.DurationNanos))
			} else if !w.time.IsZero() {
				j.StartTime = w.time
				j.EndTime = profileTime
			}
			// If we don't hold info on the previous profile, we need to cache
			// the data if the profile is cumulative (can be only generated
			// from two consecutive samples.)
			if pt.IsCumulative() {
				prev, found := w.tries.get(pt, id)
				w.tries.put(pt, entry)
				if !found {
					continue
				}
				j.Trie = entry.Trie.Diff(prev.Trie)
			}
			w.write(pt, entry.labels, j)
		}
	}

	w.time = profileTime
	return nil
}

func (w *pprofWriter) write(pt spy.ProfileType, l *spy.Labels, j *upstream.UploadJob) {
	j.Name = w.buildAppName(pt, l)
	j.Units = pt.Units()
	j.AggregationType = pt.AggregationType()
	w.upstream.Upload(j)
}

func (w *pprofWriter) buildAppName(pt spy.ProfileType, l *spy.Labels) string {
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
	return segment.NewKey(nameLabels).Normalized()
}

type triesCache map[spy.ProfileType]map[string]*cacheEntry

type cacheEntry struct {
	*transporttrie.Trie
	labels *spy.Labels
}

func (t triesCache) get(pt spy.ProfileType, name string) (*cacheEntry, bool) {
	p, ok := t[pt]
	if !ok {
		return nil, false
	}
	x, ok := p[name]
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
