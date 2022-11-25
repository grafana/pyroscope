package synth

import (
	"strconv"
)

type keyNamesGenerator struct {
	mapping map[string]string
}

func newKeyNamesGenerator() *keyNamesGenerator {
	return &keyNamesGenerator{
		mapping: make(map[string]string),
	}
}

func (f *keyNamesGenerator) key(name string) string {
	if _, ok := f.mapping[name]; !ok {
		f.mapping[name] = strconv.Itoa(len(f.mapping))
	}
	return f.mapping[name]
}
