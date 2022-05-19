package tree

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
)

type SampleTypeConfig struct {
	Units       metadata.Units           `json:"units,omitempty" yaml:"units,omitempty"`
	DisplayName string                   `json:"display-name,omitempty" yaml:"display-name,omitempty"`
	Aggregation metadata.AggregationType `json:"aggregation,omitempty" yaml:"aggregation,omitempty"`
	Cumulative  bool                     `json:"cumulative,omitempty" yaml:"cumulative,omitempty"`
	Sampled     bool                     `json:"sampled,omitempty" yaml:"sampled,omitempty"`
}

// DefaultSampleTypeMapping contains default settings for every
// supported pprof sample type. These settings are required to build
// a proper storage.PutInput payload.
//
// TODO(kolesnikovae): We should find a way to eliminate collisions.
//
//  For example, both Go 'block' and 'mutex' profiles have
//  'contentions' and 'delay' sample types - this means we can't
//  override display name of the profile types and they would
//  be indistinguishable for the server.
//
//  The keys should have the following structure:
//  	{origin}.{profile_type}.{sample_type}
//
//  Example names (can be a reserved label, e.g __type__):
//    * go.cpu.samples
//    * go.block.delay
//    * go.mutex.delay
//    * nodejs.heap.objects
//
// Another problem is that in pull mode we don't have spy-name,
// therefore we should solve this problem first.
var DefaultSampleTypeMapping = map[string]*SampleTypeConfig{
	// Sample types specific to Go.
	"samples": {
		DisplayName: "cpu",
		Units:       "samples",
		Sampled:     true,
	},
	"inuse_objects": {
		Units:       "objects",
		Aggregation: "average",
	},
	"alloc_objects": {
		Units:      "objects",
		Cumulative: true,
	},
	"inuse_space": {
		Units:       "bytes",
		Aggregation: "average",
	},
	"alloc_space": {
		Units:      "bytes",
		Cumulative: true,
	},
}

type pprof struct {
	locations map[string]uint64
	functions map[string]uint64
	strings   map[string]int64
	profile   *Profile
}

type PprofMetadata struct {
	Type      string
	Unit      string
	StartTime time.Time
	Duration  time.Duration
}

func (t *Tree) Pprof(mdata *PprofMetadata) *Profile {
	t.RLock()
	defer t.RUnlock()

	p := &pprof{
		locations: make(map[string]uint64),
		functions: make(map[string]uint64),
		strings:   make(map[string]int64),
		profile: &Profile{
			StringTable: []string{""},
		},
	}

	p.profile.SampleType = []*ValueType{{Type: p.newString(mdata.Type), Unit: p.newString(mdata.Unit)}}
	p.profile.TimeNanos = mdata.StartTime.UnixNano()
	p.profile.DurationNanos = mdata.Duration.Nanoseconds()
	t.IterateStacks(func(name string, self uint64, stack []string) {
		value := []int64{int64(self)}
		loc := []uint64{}
		for _, l := range stack {
			loc = append(loc, p.newLocation(l))
		}
		sample := &Sample{LocationId: loc, Value: value}
		p.profile.Sample = append(p.profile.Sample, sample)
	})

	return p.profile
}

func (p *pprof) newString(value string) int64 {
	id, ok := p.strings[value]
	if !ok {
		id = int64(len(p.profile.StringTable))
		p.profile.StringTable = append(p.profile.StringTable, value)
		p.strings[value] = id
	}
	return id
}

func (p *pprof) newLocation(location string) uint64 {
	id, ok := p.locations[location]
	if !ok {
		id = uint64(len(p.profile.Location) + 1)
		newLoc := &Location{
			Id:   id,
			Line: []*Line{{FunctionId: p.newFunction(location)}},
		}
		p.profile.Location = append(p.profile.Location, newLoc)
		p.locations[location] = newLoc.Id
	}
	return id
}

func (p *pprof) newFunction(function string) uint64 {
	id, ok := p.functions[function]
	if !ok {
		id = uint64(len(p.profile.Function) + 1)
		name := p.newString(function)
		newFn := &Function{
			Id:         id,
			Name:       name,
			SystemName: name,
		}
		p.functions[function] = id
		p.profile.Function = append(p.profile.Function, newFn)
	}
	return id
}
