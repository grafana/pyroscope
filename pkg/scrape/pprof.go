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
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/valyala/bytebufferpool"
	"google.golang.org/protobuf/proto"

	"github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/labels"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/model"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type pprofWriter struct {
	storage *storage.Storage
	labels  labels.Labels
	config  *config.Profile
	time    time.Time
	cache   cache
}

func newPprofWriter(s *storage.Storage, target *Target) *pprofWriter {
	return &pprofWriter{
		storage: s,
		labels:  target.Labels(),
		config:  target.profile,
		cache:   make(cache),
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
	finder := tree.NewFinder(&p)

	for _, s := range p.GetSampleType() {
		sampleTypeName := p.StringTable[s.Type]
		sampleTypeConfig, ok := w.config.SampleTypes[sampleTypeName]
		if !ok && !w.config.AllSampleTypes {
			continue
		}

		c.writeProfiles(&p, s.Type, finder)
		for hash, entry := range c[s.Type] {
			pi := &storage.PutInput{SpyName: "scrape", Val: entry.Tree}
			// Cumulative profiles require two consecutive samples,
			// therefore we have to cache this trie.
			if sampleTypeConfig.Cumulative {
				prev, found := w.cache.get(s.Type, hash)
				if !found {
					continue
				}
				pi.Val = entry.Tree.Diff(prev.Tree)
			}

			switch {
			case p.DurationNanos > 0:
				pi.StartTime = profileTime
				pi.EndTime = profileTime.Add(time.Duration(p.DurationNanos))
			case !w.time.IsZero():
				// Without DurationNanos we can not deduce the time range
				// of the profile and therefore need the previous profile time.
				pi.StartTime = w.time
				pi.EndTime = profileTime
			default:
				continue
			}

			pi.AggregationType = sampleTypeConfig.Aggregation
			if sampleTypeConfig.Sampled && p.Period > 0 {
				pi.SampleRate = uint32(time.Second / time.Duration(p.Period))
			}
			if sampleTypeConfig.DisplayName != "" {
				sampleTypeName = sampleTypeConfig.DisplayName
			}
			pi.Key = w.buildName(sampleTypeName, resolveLabels(&p, entry))
			if sampleTypeConfig.Units != "" {
				pi.Units = sampleTypeConfig.Units
			} else {
				pi.Units = p.StringTable[s.Unit]
			}

			w.storage.Put(pi)
		}
	}

	// TODO(kolesnikovae):
	//  Not all profiles need to be kept in cache (e.g. "samples").
	w.cache = c
	w.time = profileTime
	return nil
}

func (w *pprofWriter) buildName(sampleTypeName string, nameLabels map[string]string) *segment.Key {
	for _, label := range w.labels {
		nameLabels[label.Name] = label.Value
	}
	nameLabels[model.AppNameLabel] += "." + sampleTypeName
	return segment.NewKey(nameLabels)
}

var samplesPool = bytebufferpool.Pool{}

// sample type -> labels hash -> entry
type cache map[int64]map[uint64]*cacheEntry

type cacheEntry struct {
	labels []*tree.Label
	*tree.Tree
}

func newCacheEntry(l []*tree.Label) *cacheEntry {
	return &cacheEntry{Tree: tree.New(), labels: l}
}

func (t *cache) writeProfiles(x *tree.Profile, sampleType int64, finder tree.Finder) {
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
			loc, ok := finder.FindLocation(s.LocationId[i])
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
				fn, ok := finder.FindFunction(loc.Line[j].FunctionId)
				if !ok {
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
