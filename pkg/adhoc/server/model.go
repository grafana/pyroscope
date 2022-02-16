package server

import (
	"errors"
	"path"
	"reflect"
	"strings"
	"unicode"

	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

type ConverterFn func(b []byte, name string, maxNodes int) (*flamebearer.FlamebearerProfile, error)

var (
	formatConverters = map[string]ConverterFn{
		"json":      JSONToProfileV1,
		"pprof":     PprofToProfileV1,
		"collapsed": CollapsedToProfileV1,
	}
	errTooShort = errors.New("profile is too short")
)

//revive:disable:line-length-limit We want to keep the base64 example in a single line

// swagger:model
type Model struct {
	// Name of the file in which the profile was saved, if any.
	// example: pyroscope.server.cpu-2022-01-23T14:31:43Z.json
	Filename string `json:"filename"`
	// base64-encoded data of the profile, in any of the supported formats
	// (currently supported: pprof, Pyroscope JSON and collapsed).
	// example: eyJ2ZXJzaW9uIjoxLCJmbGFtZWJlYXJlciI6eyJuYW1lcyI6WyJ0b3RhbCIsImEiLCJjIiwiYiJdLCJsZXZlbHMiOltbMCwzLDAsMF0sWzAsMywwLDFdLFswLDEsMSwzLDAsMiwyLDJdXSwibnVtVGlja3MiOjMsIm1heFNlbGYiOjJ9LCJtZXRhZGF0YSI6eyJmb3JtYXQiOiJzaW5nbGUiLCJzcHlOYW1lIjoic3B5LW5hbWUiLCJzYW1wbGVSYXRlIjoxMCwidW5pdHMiOiJ1bml0cyIsIm5hbWUiOiJuYW1lIn19Cg
	// required: true
	// minLength: 2
	Profile []byte `json:"profile"`
	// Type of profile, if known (currently supported: pprof, json, collapsed")
	// example: json
	Type string `json:"type"`
}

func (m Model) Converter() (ConverterFn, error) {
	if f, ok := formatConverters[m.Type]; ok {
		return f, nil
	}
	ext := strings.TrimPrefix(path.Ext(m.Filename), ".")
	if f, ok := formatConverters[ext]; ok {
		return f, nil
	}
	if ext == "txt" {
		return formatConverters["collapsed"], nil
	}
	if len(m.Profile) < 2 {
		return nil, errTooShort
	}
	if m.Profile[0] == '{' {
		return formatConverters["json"], nil
	}
	if m.Profile[0] == '\x1f' && m.Profile[1] == '\x8b' {
		// gzip magic number, assume pprof
		return formatConverters["pprof"], nil
	}
	// Unclear whether it's uncompressed pprof or collapsed, let's check if all the bytes are printable
	// This will be slow for collapsed format, but should be fast enough for pprof, which is the most usual case,
	// but we have a reasonable upper bound just in case.
	// TODO(abeaumont): This won't work with collapsed format with non-ascii encodings.
	for i, b := range m.Profile {
		if i == 100 {
			break
		}
		if !unicode.IsPrint(rune(b)) && !unicode.IsSpace(rune(b)) {
			return formatConverters["pprof"], nil
		}
	}
	return formatConverters["collapsed"], nil
}

func ConverterToFormat(f ConverterFn) string {
	switch reflect.ValueOf(f).Pointer() {
	case reflect.ValueOf(JSONToProfileV1).Pointer():
		return "json"
	case reflect.ValueOf(PprofToProfileV1).Pointer():
		return "pprof"
	case reflect.ValueOf(CollapsedToProfileV1).Pointer():
		return "collapsed"
	}
	return "unknown"
}
