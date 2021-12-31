package tree

import (
	"sort"
)

// NewFinder creates an efficient finder for functions or locations in a profile.
//
// It exists to abstract the details of how functions and locations exist in a pprof profile,
// and make it easy to provide an efficient implementation depending on the actual profile format,
// as location and function finding is a recurrent operation while processing pprof profiles.
//
// The [pprof format description](https://github.com/google/pprof/tree/master/proto#general-structure-of-a-profile)
// describes that both locations and functions have unique nonzero ids.
// A [comment in the proto file](https://github.com/google/pprof/blob/master/proto/profile.proto#L164-L166)
// goes further: _A profile could use instruction addresses or any integer sequence as ids_.
//
// Based on this, any uint64 value (except 0) can appear as ids, and a map based cache can be used in that case.
// In practice, [go runtime](https://github.com/golang/go/blob/master/src/runtime/pprof/proto.go#L537)
// generates profiles where locations and functions use consecutive IDs starting from 1,
// making optimized access possible.
//
// Taking advantage of this, the finder will try to:
// - Use direct access to functions and locations indexed by IDs when possible
//   (sorting location and function sequences if needed).
// - Use a map based cache otherwise.
func NewFinder(p *Profile) Finder {
	return &finder{p: p, lf: nil, ff: nil}
}

type Finder interface {
	FunctionFinder
	LocationFinder
}

// Find location in a profile based on its ID
type LocationFinder interface {
	FindLocation(id uint64) (*Location, bool)
}

// Find function in a profile based on its ID
type FunctionFinder interface {
	FindFunction(id uint64) (*Function, bool)
}

type sliceLocationFinder []*Location

func (f sliceLocationFinder) FindLocation(id uint64) (*Location, bool) {
	if id == 0 || id > uint64(len(f)) {
		return nil, false
	}
	return f[id-1], true
}

type mapLocationFinder map[uint64]*Location

func (f mapLocationFinder) FindLocation(id uint64) (*Location, bool) {
	loc, ok := f[id]
	return loc, ok
}

type sliceFunctionFinder []*Function

func (f sliceFunctionFinder) FindFunction(id uint64) (*Function, bool) {
	if id == 0 || id > uint64(len(f)) {
		return nil, false
	}
	return f[id-1], true
}

type mapFunctionFinder map[uint64]*Function

func (f mapFunctionFinder) FindFunction(id uint64) (*Function, bool) {
	fun, ok := f[id]
	return fun, ok
}

// finder is a lazy implementation of Finder, that will be using the most efficient function and location finder.
type finder struct {
	p  *Profile
	lf LocationFinder
	ff FunctionFinder
}

func (f *finder) FindLocation(id uint64) (*Location, bool) {
	if f.lf == nil {
		var ok bool
		f.lf, ok = locationSlice(f.p)
		if !ok {
			f.lf = locationMap(f.p)
		}
	}
	return f.lf.FindLocation(id)
}

func (f *finder) FindFunction(id uint64) (*Function, bool) {
	if f.ff == nil {
		var ok bool
		f.ff, ok = functionSlice(f.p)
		if !ok {
			f.ff = functionMap(f.p)
		}
	}
	return f.ff.FindFunction(id)
}

func locationSlice(p *Profile) (sliceLocationFinder, bool) {
	// Check if it's already sorted first
	max := uint64(0)
	sorted := true
	for i, l := range p.Location {
		if l.Id != uint64(i+1) {
			sorted = false
			if l.Id > max {
				max = l.Id
			}
		}
	}
	if max > uint64(len(p.Location)) {
		// IDs are not consecutive numbers starting at 1, a slice is not good enough
		return nil, false
	}
	if !sorted {
		sort.Slice(p.Location, func(i, j int) bool {
			return p.Location[i].Id < p.Location[j].Id
		})
	}
	return sliceLocationFinder(p.Location), true
}

func locationMap(p *Profile) mapLocationFinder {
	m := make(map[uint64]*Location, len(p.Location))
	for _, l := range p.Location {
		m[l.Id] = l
	}
	return mapLocationFinder(m)
}

func functionSlice(p *Profile) (sliceFunctionFinder, bool) {
	// Check if it's already sorted first
	max := uint64(0)
	sorted := true
	for i, f := range p.Function {
		if f.Id != uint64(i+1) {
			sorted = false
			if f.Id > max {
				max = f.Id
			}
		}
	}
	if max > uint64(len(p.Function)) {
		// IDs are not consecutive numbers starting at one, this won't work
		return nil, false
	}
	if !sorted {
		sort.Slice(p.Function, func(i, j int) bool {
			return p.Function[i].Id < p.Function[j].Id
		})
	}
	return sliceFunctionFinder(p.Function), true
}

func functionMap(p *Profile) mapFunctionFinder {
	m := make(map[uint64]*Function, len(p.Function))
	for _, f := range p.Function {
		m[f.Id] = f
	}
	return m
}
