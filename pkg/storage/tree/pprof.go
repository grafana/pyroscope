package tree

import (
	"io/ioutil"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/proto"
)

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

func (t *Tree) Pprof(metadata *PprofMetadata) *Profile {
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

	p.profile.SampleType = []*ValueType{{Type: p.newString(metadata.Type), Unit: p.newString(metadata.Unit)}}
	p.profile.TimeNanos = metadata.StartTime.UnixNano()
	p.profile.DurationNanos = metadata.Duration.Nanoseconds()
	t.Iterate2(func(name string, self uint64, stack []string) {
		value := []int64{int64(self)}
		loc := []uint64{}
		for _, l := range stack {
			loc = append(loc, uint64(p.newLocation(l)))
		}
		sample := &Sample{LocationId: loc, Value: value}
		p.profile.Sample = append(p.profile.Sample, sample)
	})

	/* TODO: Remove */
	out, err := proto.Marshal(p.profile)
	if err == nil {
		ioutil.WriteFile("pprof.pb", out, 0600)
		m := jsonpb.Marshaler{}
		result, _ :=
			m.MarshalToString(p.profile)
		ioutil.WriteFile("./pprof.json", []byte(result), 0600)
		ioutil.WriteFile("collapsed.txt", []byte(t.Collapsed()), 0600)
		ioutil.WriteFile("collapsed2.txt", []byte(t.String()), 0600)
	}
	//
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
		name := int64(p.newString(function))
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
