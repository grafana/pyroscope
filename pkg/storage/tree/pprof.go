package tree

import (
	"io/ioutil"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/proto"
)

type Pprof struct {
	locations map[string]uint64
	functions map[string]uint64
	strings   map[string]int64
	profile   *Profile
	tree      *Tree
	metadata  *PprofMetadata
}

type PprofMetadata struct {
	Type      string
	Unit      string
	StartTime int64
	Duration  int64
}

func (t *Tree) PprofStruct(metadata *PprofMetadata) *Pprof {
	pprof := &Pprof{
		locations: make(map[string]uint64),
		functions: make(map[string]uint64),
		strings:   make(map[string]int64),
		profile: &Profile{
			StringTable: []string{""},
		},
		tree:     t,
		metadata: metadata,
	}
	return pprof
}

func (p *Pprof) Pprof() *Profile {
	p.profile.SampleType = []*ValueType{{Type: p.newString(p.metadata.Type), Unit: p.newString(p.metadata.Unit)}}
	p.profile.TimeNanos = p.metadata.StartTime
	p.profile.DurationNanos = p.metadata.Duration

	p.tree.Iterate2(func(name string, self uint64, stack []string) {
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
		ioutil.WriteFile("collapsed.txt", []byte(p.tree.Collapsed()), 0600)
		ioutil.WriteFile("collapsed2.txt", []byte(p.tree.String()), 0600)

	}
	//

	return p.profile

}

func (p *Pprof) newString(value string) int64 {
	id, ok := p.strings[value]
	if !ok {
		id = int64(len(p.profile.StringTable))
		p.profile.StringTable = append(p.profile.StringTable, value)
		p.strings[value] = id
	}
	return id
}

func (p *Pprof) newLocation(location string) uint64 {
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

func (p *Pprof) newFunction(function string) uint64 {
	id, ok := p.functions[function]
	if !ok {
		id = uint64(len(p.profile.Function) + 1)
		newFn := &Function{
			Id:         id,
			Name:       int64(p.newString(function)),
			SystemName: int64(p.newString(function)),
		}
		p.functions[function] = id
		p.profile.Function = append(p.profile.Function, newFn)
	}
	return id
}
