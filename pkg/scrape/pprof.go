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

	"github.com/cespare/xxhash/v2"
	"github.com/valyala/bytebufferpool"
	"google.golang.org/protobuf/proto"

	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/labels"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/model"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/sortedmap"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type pprofWriter struct {
	// Instead of the upstream (which requires transporttrie.Trie)
	// we could have a more generic samples consumer.
	upstream upstream.Upstream
	labels   labels.Labels
	config   *config.Profile
	time     time.Time
	cache    cache
}

func newPprofWriter(u upstream.Upstream, target *Target) *pprofWriter {
	return &pprofWriter{
		upstream: u,
		labels:   target.Labels(),
		config:   target.profile,
		cache:    make(cache),
	}
}

func (w *pprofWriter) reset() {
	w.time = time.Time{}
	w.cache = make(cache)
}

func (w *pprofWriter) writeProfile(b []byte) error {
	var p tree.Profile
	if err := proto.Unmarshal(b, &p); err != nil {
		return err
	}
	var profileTime time.Time
	c := make(cache)
	// TimeNanos is the time of collection (UTC) represented
	// as nanoseconds past the epoch reported by profiler.
	if p.TimeNanos > 0 {
		profileTime = time.Unix(0, p.TimeNanos).UTC()
	} else {
		// An extreme measure.
		profileTime = time.Now()
	}

	var locs []*tree.Location
	var fns []*tree.Function

	for _, s := range p.GetSampleType() {
		sampleTypeName := p.StringTable[s.Type]
		sampleTypeConfig, ok := w.config.SampleTypes[sampleTypeName]
		if !ok && !w.config.AllSampleTypes {
			continue
		}

		if locs == nil {
			locs = tree.Locations(&p)
		}
		if fns == nil {
			fns = tree.Functions(&p)
		}

		c.writeProfiles(&p, s.Type, locs, fns)
		for hash, entry := range c[s.Type] {
			j := &upstream.UploadJob{SpyName: "scrape", Trie: entry.Trie}
			// Cumulative profiles require two consecutive samples,
			// therefore we have to cache this trie.
			if sampleTypeConfig.Cumulative {
				prev, found := w.cache.get(s.Type, hash)
				if !found {
					continue
				}
				j.Trie = entry.Trie.Diff(prev.Trie)
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

			j.AggregationType = sampleTypeConfig.Aggregation
			if sampleTypeConfig.Sampled && p.Period > 0 {
				j.SampleRate = uint32(time.Second / time.Duration(p.Period))
			}
			if sampleTypeConfig.DisplayName != "" {
				sampleTypeName = sampleTypeConfig.DisplayName
			}
			j.Name = w.buildName(sampleTypeName, resolveLabels(&p, entry))
			if sampleTypeConfig.Units != "" {
				j.Units = sampleTypeConfig.Units
			} else {
				j.Units = p.StringTable[s.Unit]
			}

			w.upstream.Upload(j)
		}
	}

	// TODO(kolesnikovae):
	//  Not all profiles need to be kept in cache (e.g. "samples").
	w.cache = c
	w.time = profileTime
	return nil
}

func (w *pprofWriter) buildName(sampleTypeName string, nameLabels map[string]string) string {
	for _, label := range w.labels {
		nameLabels[label.Name] = label.Value
	}
	nameLabels[model.AppNameLabel] += "." + sampleTypeName
	// TODO(kolesnikovae): a copy of segment.Key.Normalize().
	//   To be refactored once pyroscope model package emerges.
	var sb strings.Builder
	sortedMap := sortedmap.New()
	for k, v := range nameLabels {
		if k == model.AppNameLabel {
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

// sample type -> labels hash -> entry
type cache map[int64]map[uint64]*cacheEntry

type cacheEntry struct {
	labels []*tree.Label
	*transporttrie.Trie
}

func newCacheEntry(l []*tree.Label) *cacheEntry {
	return &cacheEntry{Trie: transporttrie.New(), labels: l}
}

func (t *cache) writeProfiles(x *tree.Profile, sampleType int64, locs []*tree.Location, fns []*tree.Function) {
	valueIndex := 0
	if sampleType != 0 {
		for i, v := range x.SampleType {
			if v.Type == sampleType {
				valueIndex = i
				break
			}
		}
	}

	b := samplesPool.Get()
	defer samplesPool.Put(b)

	for _, s := range x.Sample {
		entry := t.getOrCreate(sampleType, s.Label)
		for i := len(s.LocationId) - 1; i >= 0; i-- {
			id := s.LocationId[i]
			if id >= uint64(len(locs)) {
				continue
			}
			loc := locs[id]
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
				id := loc.Line[j].FunctionId
				if id >= uint64(len(fns)) {
					continue
				}
				fn := fns[id]
				if b.Len() > 0 {
					_ = b.WriteByte(';')
				}
				_, _ = b.WriteString(x.StringTable[fn.Name])
			}
		}

		entry.Insert(b.Bytes(), uint64(s.Value[valueIndex]), true)
		b.Reset()
	}
}

func resolveLabels(x *tree.Profile, entry *cacheEntry) map[string]string {
	m := make(map[string]string)
	for _, label := range entry.labels {
		if label.Str != 0 {
			m[x.StringTable[label.Key]] = x.StringTable[label.Str]
		}
	}
	return m
}

func (t cache) getOrCreate(sampleType int64, l []*tree.Label) *cacheEntry {
	p, ok := t[sampleType]
	if !ok {
		e := newCacheEntry(l)
		t[sampleType] = map[uint64]*cacheEntry{labelsHash(l): e}
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

func (t cache) get(sampleType int64, h uint64) (*cacheEntry, bool) {
	p, ok := t[sampleType]
	if !ok {
		return nil, false
	}
	x, ok := p[h]
	return x, ok
}

func (t cache) put(sampleType int64, e *cacheEntry) {
	p, ok := t[sampleType]
	if !ok {
		p = make(map[uint64]*cacheEntry)
		t[sampleType] = p
	}
	p[labelsHash(e.labels)] = e
}

func labelsHash(l []*tree.Label) uint64 {
	h := xxhash.New()
	t := make([]byte, 16)
	for _, x := range l {
		if x.Str == 0 {
			continue
		}
		binary.LittleEndian.PutUint64(t[0:8], uint64(x.Key))
		binary.LittleEndian.PutUint64(t[8:16], uint64(x.Str))
		_, _ = h.Write(t)
	}
	return h.Sum64()
}
