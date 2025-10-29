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
	"unicode"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/convert/perf"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof"
	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
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

type ConverterFn func(b []byte, name string, maxNodes int) ([]*flamebearer.FlamebearerProfile, error)

var formatConverters = map[ProfileFileType]ConverterFn{
	ProfileFileTypeJSON:       JSONToProfile,
	ProfileFileTypePprof:      PprofToProfile,
	ProfileFileTypeCollapsed:  CollapsedToProfile,
	ProfileFileTypePerfScript: PerfScriptToProfile,
}

func FlamebearerFromFile(f ProfileFile, maxNodes int) ([]*flamebearer.FlamebearerProfile, error) {
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
	return func(b []byte, name string, maxNodes int) ([]*flamebearer.FlamebearerProfile, error) {
		fbs, err := convertFn(b, name, maxNodes)
		if err != nil {
			return nil, fmt.Errorf("unable to process the profile. The profile was detected as %q: %w",
				converterToFormat(convertFn), err)
		}
		// Overwrite fields if available
		if p.TypeData.SpyName != "" {
			for _, fb := range fbs {
				fb.Metadata.SpyName = p.TypeData.SpyName
			}
		}
		// Replace the units if provided
		if p.TypeData.Units != "" {
			for _, fb := range fbs {
				if fb.Metadata.Units == "" {
					fb.Metadata.Units = p.TypeData.Units
				}
			}
		}
		return fbs, nil
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
//
//	Consider simpler (but more reliable) logic for format identification
//	with fallbacks: from the most strict format to the loosest one, e.g:
//	  pprof, json, collapsed, perf.
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

func JSONToProfile(b []byte, name string, maxNodes int) ([]*flamebearer.FlamebearerProfile, error) {
	var profile flamebearer.FlamebearerProfile
	if err := json.Unmarshal(b, &profile); err != nil {
		return nil, fmt.Errorf("unable to unmarshall JSON: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return nil, fmt.Errorf("invalid profile: %w", err)
	}

	t, err := flamebearer.ProfileToTree(profile)
	if err != nil {
		return nil, fmt.Errorf("could not convert flameabearer to tree: %w", err)
	}

	pc := flamebearer.ProfileConfig{
		Tree:     t,
		Name:     profile.Metadata.Name,
		MaxNodes: maxNodes,
		Metadata: metadata.Metadata{
			SpyName:    profile.Metadata.SpyName,
			SampleRate: profile.Metadata.SampleRate,
			Units:      profile.Metadata.Units,
		},
	}

	p := flamebearer.NewProfile(pc)
	return []*flamebearer.FlamebearerProfile{&p}, nil
}

func getProfileType(name string, sampleType int, p *profilev1.Profile) (*typesv1.ProfileType, error) {
	tp := &typesv1.ProfileType{
		Name: name,
	}

	// check if the sampleID is valid
	if sampleType < 0 || sampleType >= len(p.SampleType) {
		return nil, fmt.Errorf("invalid sampleID: %d", sampleType)
	}

	if p.PeriodType == nil {
		return nil, fmt.Errorf("PeriodType is nil")
	}

	invalidStr := func(i int) bool {
		return i < 0 || i > len(p.StringTable)
	}
	if v := int(p.PeriodType.Type); invalidStr(v) {
		return nil, fmt.Errorf("invalid PeriodType: %d", v)
	} else {
		tp.PeriodType = p.StringTable[v]
	}
	if v := int(p.PeriodType.Unit); invalidStr(v) {
		return nil, fmt.Errorf("invalid PeriodUnit: %d", v)
	} else {
		tp.PeriodUnit = p.StringTable[v]
	}
	if v := int(p.SampleType[sampleType].Type); invalidStr(v) {
		return nil, fmt.Errorf("invalid SampleType[%d]: %d", sampleType, v)
	} else {
		tp.SampleType = p.StringTable[v]
	}
	if v := int(p.SampleType[sampleType].Unit); invalidStr(v) {
		return nil, fmt.Errorf("invalid SampleUnit[%d]: %d", sampleType, v)
	} else {
		tp.SampleUnit = p.StringTable[v]
	}

	tp.ID = fmt.Sprintf("%s:%s:%s:%s:%s", name, tp.SampleType, tp.SampleUnit, tp.PeriodType, tp.PeriodUnit)
	return tp, nil
}

func PprofToProfile(b []byte, name string, maxNodes int) ([]*flamebearer.FlamebearerProfile, error) {
	p := new(profilev1.Profile)
	if err := pprof.Decode(bytes.NewReader(b), p); err != nil {
		return nil, fmt.Errorf("parsing pprof: %w", err)
	}

	t := model.NewStacktraceTree(int(maxNodes * 2))
	stack := make([]int32, 0, 64)
	m := make(map[uint64]int32)

	fbs := make([]*flamebearer.FlamebearerProfile, 0)
	for sampleType := range p.SampleType {
		t.Reset()

		for i := range p.Sample {
			stack = stack[:0]
			for j := range p.Sample[i].LocationId {
				locIdx := int(p.Sample[i].LocationId[j]) - 1
				if locIdx < 0 || len(p.Location) <= locIdx {
					return nil, fmt.Errorf("invalid location ID %d in sample %d", p.Sample[i].LocationId[j], i)
				}

				loc := p.Location[locIdx]
				if len(loc.Line) > 0 {
					for l := range loc.Line {
						stack = append(stack, int32(p.Function[loc.Line[l].FunctionId-1].Name))
					}
					continue
				}
				addr, ok := m[loc.Address]
				if !ok {
					addr = int32(len(p.StringTable))
					p.StringTable = append(p.StringTable, strconv.FormatInt(int64(loc.Address), 16))
					m[loc.Address] = addr
				}
				stack = append(stack, addr)
			}

			if sampleType < 0 || sampleType >= len(p.Sample[i].Value) {
				return nil, fmt.Errorf("invalid sampleType index %d for sample %d (len=%d)", sampleType, i, len(p.Sample[i].Value))
			}

			t.Insert(stack, p.Sample[i].Value[sampleType])
		}

		tp, err := getProfileType(name, sampleType, p)
		if err != nil {
			return nil, err
		}

		fg := model.NewFlameGraph(t.Tree(int64(maxNodes), p.StringTable), int64(maxNodes))
		fbs = append(fbs, model.ExportToFlamebearer(fg, tp))
	}
	if len(fbs) == 0 {
		return nil, errors.New("no supported sample type found")
	}
	return fbs, nil
}

func CollapsedToProfile(b []byte, name string, maxNodes int) ([]*flamebearer.FlamebearerProfile, error) {
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
	return []*flamebearer.FlamebearerProfile{&fb}, nil
}

func PerfScriptToProfile(b []byte, name string, maxNodes int) ([]*flamebearer.FlamebearerProfile, error) {
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
	return []*flamebearer.FlamebearerProfile{&fb}, nil
}
