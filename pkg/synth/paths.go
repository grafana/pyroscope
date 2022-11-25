package synth

import "strings"

type pathSanitizer struct {
	mapping map[string]string
}

func newPathSanitizer() *pathSanitizer {
	return &pathSanitizer{
		mapping: make(map[string]string),
	}
}

func (f *pathSanitizer) sanitize(name string) string {
	if strings.HasPrefix(name, "/") {
		name = name[1:]
	}
	if name == "(unknown)" {
		return "main.rb"
	}

	if !strings.HasSuffix(name, ".rb") {
		name += ".rb"
	}
	return name
}
