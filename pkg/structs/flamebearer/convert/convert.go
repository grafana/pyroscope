package convert

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/convert/perf"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

// ProfileFile represents content to be converted to flamebearer.
type ProfileFile struct {
	// Name of the file in which the profile was saved. Optional.
	// example: pyroscope.server.cpu-2022-01-23T14:31:43Z.json
	Name string
	// Type of profile. Optional.
	Type     ProfileFileType
	TypeData ProfileFileTypeData
	// Raw profile bytes. Required, min length 2.
	Data []byte
}

type ProfileFileType string

type ProfileFileTypeData struct {
	SpyName string
	Units   metadata.Units
}

const (
	ProfileFileTypeJSON       ProfileFileType = "json"
	ProfileFileTypePprof      ProfileFileType = "pprof"
	ProfileFileTypeCollapsed  ProfileFileType = "collapsed"
	ProfileFileTypePerfScript ProfileFileType = "perf_script"
)

type ConverterFn func(b []byte, name string, maxNodes int) (*flamebearer.FlamebearerProfile, error)

var formatConverters = map[ProfileFileType]ConverterFn{
	ProfileFileTypeJSON:       JSONToProfile,
	ProfileFileTypePprof:      PprofToProfile,
	ProfileFileTypeCollapsed:  CollapsedToProfile,
	ProfileFileTypePerfScript: PerfScriptToProfile,
}

func FlamebearerFromFile(f ProfileFile, maxNodes int) (*flamebearer.FlamebearerProfile, error) {
	convertFn, _, err := Converter(f)
	if err != nil {
		return nil, err
	}
	return convertFn(f.Data, f.Name, maxNodes)
}

// Converter returns a ConverterFn that converts to
// FlamebearerProfile and overrides any specified fields.
func Converter(p ProfileFile) (ConverterFn, ProfileFileType, error) {
	convertFn, err := converter(p)
	if err != nil {
		return nil, "", err
	}
	return func(b []byte, name string, maxNodes int) (*flamebearer.FlamebearerProfile, error) {
		fb, err := convertFn(b, name, maxNodes)
		if err != nil {
			return nil, fmt.Errorf("unable to process the profile. The profile was detected as %q: %w",
				converterToFormat(convertFn), err)
		}
		// Overwrite fields if available
		if p.TypeData.SpyName != "" {
			fb.Metadata.SpyName = p.TypeData.SpyName
		}
		// Replace the units if provided
		if p.TypeData.Units != "" {
			fb.Metadata.Units = p.TypeData.Units
		}
		return fb, nil
	}, converterToFormat(convertFn), nil
}

// Note that converterToFormat works only for converter output,
// Converter wraps the returned function into anonymous one.
func converterToFormat(f ConverterFn) ProfileFileType {
	switch reflect.ValueOf(f).Pointer() {
	case reflect.ValueOf(JSONToProfile).Pointer():
		return ProfileFileTypeJSON
	case reflect.ValueOf(PprofToProfile).Pointer():
		return ProfileFileTypePprof
	case reflect.ValueOf(CollapsedToProfile).Pointer():
		return ProfileFileTypeCollapsed
	case reflect.ValueOf(PerfScriptToProfile).Pointer():
		return ProfileFileTypePerfScript
	}
	return "unknown"
}

// TODO(kolesnikovae):
//  Consider simpler (but more reliable) logic for format identification
//  with fallbacks: from the most strict format to the loosest one, e.g:
//    pprof, json, collapsed, perf.
func converter(p ProfileFile) (ConverterFn, error) {
	if f, ok := formatConverters[p.Type]; ok {
		return f, nil
	}
	ext := strings.TrimPrefix(path.Ext(p.Name), ".")
	if f, ok := formatConverters[ProfileFileType(ext)]; ok {
		return f, nil
	}
	if ext == "txt" {
		if perf.IsPerfScript(p.Data) {
			return PerfScriptToProfile, nil
		}
		return CollapsedToProfile, nil
	}
	if len(p.Data) < 2 {
		return nil, errors.New("profile is too short")
	}
	if p.Data[0] == '{' {
		return JSONToProfile, nil
	}
	if p.Data[0] == '\x1f' && p.Data[1] == '\x8b' {
		// gzip magic number, assume pprof
		return PprofToProfile, nil
	}
	// Unclear whether it's uncompressed pprof or collapsed, let's check if all the bytes are printable
	// This will be slow for collapsed format, but should be fast enough for pprof, which is the most usual case,
	// but we have a reasonable upper bound just in case.
	// TODO(abeaumont): This won't work with collapsed format with non-ascii encodings.
	for i, b := range p.Data {
		if i == 100 {
			break
		}
		if !unicode.IsPrint(rune(b)) && !unicode.IsSpace(rune(b)) {
			return PprofToProfile, nil
		}
	}
	if perf.IsPerfScript(p.Data) {
		return PerfScriptToProfile, nil
	}
	return CollapsedToProfile, nil
}

func JSONToProfile(b []byte, name string, _ int) (*flamebearer.FlamebearerProfile, error) {
	var profile flamebearer.FlamebearerProfile
	if err := json.Unmarshal(b, &profile); err != nil {
		return nil, fmt.Errorf("unable to unmarshall JSON: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return nil, fmt.Errorf("invalid profile: %w", err)
	}
	if name != "" {
		profile.Metadata.Name = name
	}
	return &profile, nil
}

func PprofToProfile(b []byte, name string, maxNodes int) (*flamebearer.FlamebearerProfile, error) {
	var p tree.Profile
	if err := pprof.Decode(bytes.NewReader(b), &p); err != nil {
		return nil, fmt.Errorf("parsing pprof: %w", err)
	}
	// TODO(abeaumont): Support multiple sample types
	for _, stype := range p.SampleTypes() {
		sampleRate := uint32(100)
		units := metadata.SamplesUnits
		if c, ok := tree.DefaultSampleTypeMapping[stype]; ok {
			units = c.Units
			if c.Sampled && p.Period > 0 {
				sampleRate = uint32(time.Second / time.Duration(p.Period))
			}
		}
		t := tree.New()
		p.Get(stype, func(_labels *spy.Labels, name []byte, val int) error {
			t.Insert(name, uint64(val))
			return nil
		})
		fb := flamebearer.NewProfile(flamebearer.ProfileConfig{
			Tree:     t,
			Name:     name,
			MaxNodes: maxNodes,
			Metadata: metadata.Metadata{
				SpyName:    "unknown",
				SampleRate: sampleRate,
				Units:      units,
			},
		})
		return &fb, nil
	}
	return nil, errors.New("no supported sample type found")
}

func CollapsedToProfile(b []byte, name string, maxNodes int) (*flamebearer.FlamebearerProfile, error) {
	t := tree.New()
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		i := bytes.LastIndexByte(line, ' ')
		if i < 0 {
			return nil, errors.New("unable to find stacktrace and value separator")
		}
		value, err := strconv.ParseUint(string(line[i+1:]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unable to parse sample value: %w", err)
		}
		t.Insert(line[:i], value)
	}
	fb := flamebearer.NewProfile(flamebearer.ProfileConfig{
		Name:     name,
		Tree:     t,
		MaxNodes: maxNodes,
		Metadata: metadata.Metadata{
			SpyName:    "unknown",
			SampleRate: 100, // We don't have this information, use the default
		},
	})
	return &fb, nil
}

func PerfScriptToProfile(b []byte, name string, maxNodes int) (*flamebearer.FlamebearerProfile, error) {
	t := tree.New()
	p := perf.NewScriptParser(b)
	events, err := p.ParseEvents()
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		t.InsertStack(e, 1)
	}
	fb := flamebearer.NewProfile(flamebearer.ProfileConfig{
		Name:     name,
		Tree:     t,
		MaxNodes: maxNodes,
		Metadata: metadata.Metadata{
			SpyName:    "unknown",
			SampleRate: 100, // We don't have this information, use the default
		},
	})
	return &fb, nil
}
