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
	"encoding/binary"
	"strings"
	"time"

	"github.com/cespare/xxhash"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/valyala/bytebufferpool"
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
	cache cache
}

func newPprofWriter(u upstream.Upstream, l labels.Labels) *pprofWriter {
	return &pprofWriter{
		cache:    make(cache),
		labels:   l,
		upstream: u,
	}
}

func (w *pprofWriter) Reset() {
	w.time = time.Time{}
	w.cache = make(cache)
}

func (w *pprofWriter) WriteProfile(b []byte) error {
	var p convert.Profile
	if err := proto.Unmarshal(b, &p); err != nil {
		return err
	}
	// TimeNanos is the time of collection (UTC) represented
	// as nanoseconds past the epoch reported by profiler.
	profileTime := time.Unix(0, p.TimeNanos).UTC()
	c := make(cache)

	for _, st := range p.GetSampleType() {
		pt := spy.ProfileType(p.StringTable[st.Type])
		if pt == spy.ProfileCPU {
			// spy.ProfileType denotes sample type, and spy.ProfileCPU
			// refers to "cpu" sample type which measures in seconds.
			// In Pyroscope, CPU profiles are built from "samples".
			continue
		}

		c.writeProfiles(&p, string(pt))
		for hash, entry := range c[pt] {
			j := &upstream.UploadJob{SpyName: "scrape", Trie: entry.Trie}
			// Cumulative profiles require two consecutive samples,
			// therefore we have to cache this trie.
			if pt.IsCumulative() {
				prev, found := w.cache.get(pt, hash)
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

			j.Name = w.buildName(pt, resolveLabels(&p, entry))
			j.Units = pt.Units()
			j.AggregationType = pt.AggregationType()
			w.upstream.Upload(j)
		}
	}

	// We don't need to keep CPU profile cache anymore.
	delete(c, "samples")
	w.cache = c
	w.time = profileTime
	return nil
}

func (w *pprofWriter) buildName(pt spy.ProfileType, nameLabels map[string]string) string {
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

var samplesPool = bytebufferpool.Pool{}

type cache map[spy.ProfileType]map[uint64]*cacheEntry

type cacheEntry struct {
	labels []*convert.Label
	*transporttrie.Trie
}

func newCacheEntry(l []*convert.Label) *cacheEntry {
	return &cacheEntry{Trie: transporttrie.New(), labels: l}
}

func (t *cache) writeProfiles(x *convert.Profile, sampleType string) {
	valueIndex := 0
	if sampleType != "" {
		for i, v := range x.SampleType {
			if x.StringTable[v.Type] == sampleType {
				valueIndex = i
				break
			}
		}
	}

	b := samplesPool.Get()
	defer samplesPool.Put(b)

	for _, s := range x.Sample {
		entry := t.getOrCreate(spy.ProfileType(sampleType), s.Label)
		for i := len(s.LocationId) - 1; i >= 0; i-- {
			loc, ok := convert.FindLocation(x, s.LocationId[i])
			if !ok {
				continue
			}
			// Multiple line indicates this location has inlined functions,
			// where the last entry represents the caller into which the
			// preceding entries were inlined.
			//
			// E.g., if memcpy() is inlined into printf:
			//    line[0].function_name == "memcpy"
			//    line[1].function_name == "printf"
			//
			// Therefore iteration goes in reverse order.
			for j := len(loc.Line) - 1; j >= 0; j-- {
				fn, found := convert.FindFunction(x, loc.Line[j].FunctionId)
				if !found {
					continue
				}
				if b.Len() > 0 {
					_ = b.WriteByte(';')
				}
				_, _ = b.WriteString(x.StringTable[fn.Name])
			}
		}

		entry.Insert(b.Bytes(), uint64(s.Value[valueIndex]))
		b.Reset()
	}
}

func resolveLabels(x *convert.Profile, entry *cacheEntry) map[string]string {
	m := make(map[string]string)
	for _, label := range entry.labels {
		m[x.StringTable[label.Key]] = x.StringTable[label.Str]
	}
	return m
}

func (t cache) getOrCreate(pt spy.ProfileType, l []*convert.Label) *cacheEntry {
	p, ok := t[pt]
	if !ok {
		e := newCacheEntry(l)
		t[pt] = map[uint64]*cacheEntry{labelsHash(l): e}
		return e
	}
	h := labelsHash(l)
	e, found := p[h]
	if !found {
		e = newCacheEntry(l)
		p[h] = e
	}
	return e
}

func (t cache) get(pt spy.ProfileType, h uint64) (*cacheEntry, bool) {
	p, ok := t[pt]
	if !ok {
		return nil, false
	}
	x, ok := p[h]
	return x, ok
}

func (t cache) put(pt spy.ProfileType, e *cacheEntry) {
	p, ok := t[pt]
	if !ok {
		p = make(map[uint64]*cacheEntry)
		t[pt] = p
	}
	p[labelsHash(e.labels)] = e
}

func (t cache) reset(pt spy.ProfileType, h uint64) {
	if p, ok := t[pt]; ok {
		delete(p, h)
	}
}

func labelsHash(l []*convert.Label) uint64 {
	const es = 16
	b := make([]byte, len(l)*es)
	t := make([]byte, es)
	for _, x := range l {
		if x.Str != 0 {
			binary.LittleEndian.PutUint64(t[0:8], uint64(x.Key))
			binary.LittleEndian.PutUint64(t[8:16], uint64(x.Str))
		}
		b = append(b, t...)
	}
	return xxhash.Sum64(b)
}
