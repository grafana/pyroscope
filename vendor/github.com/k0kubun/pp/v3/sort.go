// sort.go: Implementation for sorting map keys
package pp

import (
	"reflect"
	"sort"
)

func sortMap(value reflect.Value) *sortedMap {
	if value.Type().Kind() != reflect.Map {
		panic("sortMap is used for a non-Map value")
	}

	keys := make([]reflect.Value, 0, value.Len())
	values := make([]reflect.Value, 0, value.Len())
	mapKeys := value.MapKeys()
	for i := 0; i < len(mapKeys); i++ {
		keys = append(keys, mapKeys[i])
		values = append(values, value.MapIndex(mapKeys[i]))
	}

	sorted := &sortedMap{
		keys:   keys,
		values: values,
	}
	sort.Stable(sorted)
	return sorted
}

type sortedMap struct {
	keys   []reflect.Value
	values []reflect.Value
}

// Functions for sort.Interface

func (s *sortedMap) Len() int {
	return len(s.keys)
}

func (s *sortedMap) Swap(i, j int) {
	s.keys[i], s.keys[j] = s.keys[j], s.keys[i]
	s.values[i], s.values[j] = s.values[j], s.values[i]
}

func (s *sortedMap) Less(i, j int) bool {
	a, b := s.keys[i], s.keys[j]
	if a.Type() != b.Type() {
		return false // give up
	}

	// Return true if b is bigger
	switch a.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() < b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return a.Uint() < b.Uint()
	case reflect.String:
		return a.String() < b.String()
	case reflect.Float32, reflect.Float64:
		if a.Float() != a.Float() || b.Float() != b.Float() {
			return false // NaN
		}
		return a.Float() < b.Float()
	case reflect.Bool:
		return !a.Bool() && b.Bool()
	case reflect.Ptr:
		return a.Pointer() < b.Pointer()
	case reflect.Struct:
		return a.NumField() < b.NumField()
	case reflect.Array:
		return a.Len() < b.Len()
	default:
		return false // not supported yet
	}
}
