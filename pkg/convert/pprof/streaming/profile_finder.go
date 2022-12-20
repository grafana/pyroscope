package streaming

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
func NewFinder(functions []function, locations []location) Finder {
	return &finder{functions: functions, locations: locations, lf: nil, ff: nil}
}

type Finder interface {
	functionFinder
	locationFinder
}

// Find location in a profile based on its ID
type locationFinder interface {
	Findlocation(id uint64) (*location, bool)
}

// Find function in a profile based on its ID
type functionFinder interface {
	Findfunction(id uint64) (*function, bool)
}

type slicelocationFinder []location

func (f slicelocationFinder) Findlocation(id uint64) (*location, bool) {
	if id == 0 || id > uint64(len(f)) {
		return nil, false
	}
	return &f[id-1], true
}

type maplocationFinder map[uint64]*location

func (f maplocationFinder) Findlocation(id uint64) (*location, bool) {
	loc, ok := f[id]
	return loc, ok
}

type slicefunctionFinder []function

func (f slicefunctionFinder) Findfunction(id uint64) (*function, bool) {
	if id == 0 || id > uint64(len(f)) {
		return nil, false
	}
	return &f[id-1], true
}

type mapfunctionFinder map[uint64]*function

func (f mapfunctionFinder) Findfunction(id uint64) (*function, bool) {
	fun, ok := f[id]
	return fun, ok
}

// finder is a lazy implementation of Finder, that will be using the most efficient function and location finder.
type finder struct {
	functions []function
	locations []location
	lf locationFinder
	ff functionFinder
}

func (f *finder) Findlocation(id uint64) (*location, bool) {
	if f.lf == nil {
		var ok bool
		f.lf, ok = locationSlice(f.locations)
		if !ok {
			f.lf = locationMap(f.locations)
		}
	}
	return f.lf.Findlocation(id)
}

func (f *finder) Findfunction(id uint64) (*function, bool) {
	if f.ff == nil {
		var ok bool
		f.ff, ok = functionSlice(f.functions)
		if !ok {
			f.ff = functionMap(f.functions)
		}
	}
	return f.ff.Findfunction(id)
}

func locationSlice(locations[]location) (slicelocationFinder, bool) {
	// Check if it's already sorted first
	max := uint64(0)
	sorted := true
	for i, l := range locations {
		if l.id != uint64(i+1) {
			sorted = false
			if l.id > max {
				max = l.id
			}
		}
	}
	if max > uint64(len(locations)) {
		// IDs are not consecutive numbers starting at 1, a slice is not good enough
		return nil, false
	}
	if !sorted {
		sort.Slice(locations, func(i, j int) bool {
			return locations[i].id < locations[j].id
		})
	}
	return slicelocationFinder(locations), true
}

func locationMap(locations []location) maplocationFinder {
	m := make(map[uint64]*location, len(locations))
	for _, l := range locations {
		m[l.id] = &l
	}
	return maplocationFinder(m)
}

func functionSlice(functions [] function) (slicefunctionFinder, bool) {
	// Check if it's already sorted first
	max := uint64(0)
	sorted := true
	for i, f := range functions {
		if f.id != uint64(i+1) {
			sorted = false
			if f.id > max {
				max = f.id
			}
		}
	}
	if max > uint64(len(functions)) {
		// IDs are not consecutive numbers starting at one, this won't work
		return nil, false
	}
	if !sorted {
		sort.Slice(functions, func(i, j int) bool {
			return functions[i].id < functions[j].id
		})
	}
	return slicefunctionFinder(functions), true
}

func functionMap(functions[]function) mapfunctionFinder {
	m := make(map[uint64]*function, len(functions))
	for _, f := range functions {
		m[f.id] = &f
	}
	return m
}
