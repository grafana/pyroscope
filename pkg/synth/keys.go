package synth

import (
	"strconv"
)

type keyNamesSanitizer struct {
	mapping map[string]string
}

func newKeyNamesSanitizer() *keyNamesSanitizer {
	return &keyNamesSanitizer{
		mapping: make(map[string]string),
	}
}

func (f *keyNamesSanitizer) sanitize(name string) string {
	if _, ok := f.mapping[name]; !ok {
		f.mapping[name] = strconv.Itoa(len(f.mapping))
	}
	return f.mapping[name]
}
