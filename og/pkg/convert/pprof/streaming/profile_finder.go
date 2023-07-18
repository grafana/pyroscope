package streaming

import (
	"sort"
)

func newFinder(functions []function, locations []location) finder {
	res := finder{functions: functions, locations: locations}
	if !locationSlice(locations) {
		res.locationsMap = locationMap(locations)
	}
	if !functionSlice(functions) {
		res.functionsMap = functionMap(functions)
	}
	return res
}

type finder struct {
	functions    []function
	locations    []location
	functionsMap map[uint64]*function
	locationsMap map[uint64]*location
}

func (f *finder) FindLocation(id uint64) (*location, bool) {
	if f.locationsMap == nil {
		idx := id - 1
		return &f.locations[idx], true
	}
	l, ok := f.locationsMap[id]
	return l, ok
}

func (f *finder) FindFunction(id uint64) (*function, bool) {
	if f.functionsMap == nil {
		idx := id - 1
		return &f.functions[idx], true
	}
	ff, ok := f.functionsMap[id]
	return ff, ok
}

func locationSlice(locations []location) (ok bool) {
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
		return false
	}
	if !sorted {
		sort.Slice(locations, func(i, j int) bool {
			return locations[i].id < locations[j].id
		})
	}
	return true
}

func locationMap(locations []location) map[uint64]*location {
	m := make(map[uint64]*location, len(locations))
	for i := range locations {
		m[locations[i].id] = &locations[i]
	}
	return m
}

func functionSlice(functions []function) (ok bool) {
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
		return false
	}
	if !sorted {
		sort.Slice(functions, func(i, j int) bool {
			return functions[i].id < functions[j].id
		})
	}
	return true
}

func functionMap(functions []function) map[uint64]*function {
	m := make(map[uint64]*function, len(functions))
	for i := range functions {
		m[functions[i].id] = &functions[i]
	}
	return m
}
