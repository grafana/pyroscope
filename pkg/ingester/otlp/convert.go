package otlp

import (
	"encoding/hex"
	"fmt"
	"time"

	otelProfile "go.opentelemetry.io/proto/otlp/profiles/v1development"

	googleProfile "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	pyromodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
)

const serviceNameKey = "service.name"

type convertedProfile struct {
	profile *googleProfile.Profile
	name    *typesv1.LabelPair
}

func at[T any](arr []T, i int32) (T, error) {
	if i >= 0 && int(i) < len(arr) {
		return arr[i], nil
	}
	var zero T
	return zero, fmt.Errorf("index %d out of bounds", i)
}

// ConvertOtelToGoogle converts an OpenTelemetry profile to a Google profile.
func ConvertOtelToGoogle(src *otelProfile.Profile, dictionary *otelProfile.ProfilesDictionary) (map[string]convertedProfile, error) {
	svc2Profile := make(map[string]*profileBuilder)
	for _, sample := range src.Sample {
		svc, err := serviceNameFromSample(sample, dictionary)
		if err != nil {
			return make(map[string]convertedProfile), nil
		}

		p, ok := svc2Profile[svc]
		if !ok {
			p, err = newProfileBuilder(src, dictionary)
			if err != nil {
				return nil, err
			}
			svc2Profile[svc] = p
		}
		if _, err := p.convertSampleBack(sample, dictionary); err != nil {
			return nil, err
		}
	}

	result := make(map[string]convertedProfile)
	for svc, p := range svc2Profile {
		result[svc] = convertedProfile{p.dst, p.name}
	}

	return result, nil
}

type sampleConversionType int

const (
	sampleConversionTypeNone           sampleConversionType = 0
	sampleConversionTypeSamplesToNanos sampleConversionType = 1
	sampleConversionTypeSumEvents      sampleConversionType = 2
)

type profileBuilder struct {
	src                     *otelProfile.Profile
	dst                     *googleProfile.Profile
	stringMap               map[string]int64
	functionMap             map[*otelProfile.Function]uint64
	unsymbolziedFuncNameMap map[string]uint64
	locationMap             map[*otelProfile.Location]uint64
	mappingMap              map[*otelProfile.Mapping]uint64

	sampleProcessingTypes []sampleConversionType
	name                  *typesv1.LabelPair
}

func newProfileBuilder(src *otelProfile.Profile, dictionary *otelProfile.ProfilesDictionary) (*profileBuilder, error) {
	res := &profileBuilder{
		src:                     src,
		stringMap:               make(map[string]int64),
		functionMap:             make(map[*otelProfile.Function]uint64),
		locationMap:             make(map[*otelProfile.Location]uint64),
		mappingMap:              make(map[*otelProfile.Mapping]uint64),
		unsymbolziedFuncNameMap: make(map[string]uint64),
		dst: &googleProfile.Profile{
			TimeNanos:     int64(src.TimeUnixNano),
			DurationNanos: int64(src.DurationNano),
			Period:        src.Period,
		},
	}
	res.addstr("")

	if src.SampleType == nil {
		return nil, fmt.Errorf("sample type is missing")
	}
	sampleType, err := res.convertSampleTypeBack(src.SampleType, dictionary)
	if err != nil {
		return nil, err
	}
	res.dst.SampleType = []*googleProfile.ValueType{sampleType}

	periodType, err := res.convertValueTypeBack(src.PeriodType, dictionary)
	if err != nil {
		return nil, err
	}
	res.dst.PeriodType = periodType

	var defaultSampleTypeLabel string
	if src.SampleType != nil {
		defaultSampleType := src.SampleType
		defaultSampleTypeLabel, err = at(dictionary.StringTable, defaultSampleType.TypeStrindex)
		if err != nil {
			return nil, fmt.Errorf("could not access default sample type label: %w", err)
		}
	} else {
		defaultSampleTypeLabel = "samples"
	}
	res.dst.DefaultSampleType = res.addstr(defaultSampleTypeLabel)

	if len(res.dst.SampleType) == 0 {
		res.dst.SampleType = []*googleProfile.ValueType{{
			Type: res.addstr(defaultSampleTypeLabel),
			Unit: res.addstr("ms"),
		}}
		res.dst.DefaultSampleType = res.addstr(defaultSampleTypeLabel)
	}
	res.sampleProcessingTypes = make([]sampleConversionType, len(res.dst.SampleType))
	for i := 0; i < len(res.dst.SampleType); i++ {
		profileType := res.profileType(i)
		if profileType == "samples:count:cpu:nanoseconds" {
			res.dst.SampleType[i] = &googleProfile.ValueType{
				Type: res.addstr("cpu"),
				Unit: res.addstr("nanoseconds"),
			}
			if len(res.dst.SampleType) == 1 {
				res.name = &typesv1.LabelPair{
					Name:  pyromodel.LabelNameProfileName,
					Value: "process_cpu",
				}
			}
			res.sampleProcessingTypes[i] = sampleConversionTypeSamplesToNanos
		}
		// Identify off cpu profiles
		if profileType == "events:nanoseconds::" && len(res.dst.SampleType) == 1 {
			res.sampleProcessingTypes[i] = sampleConversionTypeSumEvents
			res.name = &typesv1.LabelPair{
				Name:  pyromodel.LabelNameProfileName,
				Value: pyromodel.ProfileNameOffCpu,
			}
		}
	}
	if res.name == nil {
		res.name = &typesv1.LabelPair{
			Name:  pyromodel.LabelNameProfileName,
			Value: "process_cpu", // guess
		}
	}

	if res.dst.TimeNanos == 0 {
		res.dst.TimeNanos = time.Now().UnixNano()
	}
	if res.dst.DurationNanos == 0 {
		res.dst.DurationNanos = (time.Second * 10).Nanoseconds()
	}
	return res, nil
}

func (p *profileBuilder) profileType(idx int) string {
	var (
		periodType, periodUnit string
	)
	if p.dst.PeriodType != nil && p.dst.Period != 0 {
		periodType = p.dst.StringTable[p.dst.PeriodType.Type]
		periodUnit = p.dst.StringTable[p.dst.PeriodType.Unit]
	}
	return fmt.Sprintf("%s:%s:%s:%s",
		p.dst.StringTable[p.dst.SampleType[idx].Type],
		p.dst.StringTable[p.dst.SampleType[idx].Unit],
		periodType,
		periodUnit,
	)
}

func (p *profileBuilder) addstr(s string) int64 {
	if i, ok := p.stringMap[s]; ok {
		return i
	}
	idx := int64(len(p.dst.StringTable))
	p.stringMap[s] = idx
	p.dst.StringTable = append(p.dst.StringTable, s)
	return idx
}

func serviceNameFromSample(sample *otelProfile.Sample, dictionary *otelProfile.ProfilesDictionary) (string, error) {
	return getAttributeValueByKeyOrEmpty(sample.AttributeIndices, dictionary, serviceNameKey)
}

func (p *profileBuilder) convertSampleTypeBack(ost *otelProfile.ValueType, dictionary *otelProfile.ProfilesDictionary) (*googleProfile.ValueType, error) {
	gst, err := p.convertValueTypeBack(ost, dictionary)
	if err != nil {
		return nil, fmt.Errorf("could not process sample type: %w", err)
	}
	return gst, nil
}

func (p *profileBuilder) convertValueTypeBack(ovt *otelProfile.ValueType, dictionary *otelProfile.ProfilesDictionary) (*googleProfile.ValueType, error) {
	if ovt == nil {
		return nil, nil
	}
	typeLabel, err := at(dictionary.StringTable, ovt.TypeStrindex)
	if err != nil {
		return nil, fmt.Errorf("could not access type string: %w", err)
	}
	unitLabel, err := at(dictionary.StringTable, ovt.UnitStrindex)
	if err != nil {
		return nil, fmt.Errorf("could not access unit string: %w", err)
	}
	return &googleProfile.ValueType{Type: p.addstr(typeLabel), Unit: p.addstr(unitLabel)}, nil
}

func (p *profileBuilder) convertLocationBack(ol *otelProfile.Location, dictionary *otelProfile.ProfilesDictionary) (uint64, error) {
	if i, ok := p.locationMap[ol]; ok {
		return i, nil
	}
	lmi := ol.GetMappingIndex()
	om, err := at(dictionary.MappingTable, lmi)
	if err != nil {
		return 0, fmt.Errorf("could not access mapping: %w", err)
	}

	mappingId, err := p.convertMappingBack(om, dictionary)
	if err != nil {
		return 0, err
	}
	gl := &googleProfile.Location{
		MappingId: mappingId,
		Address:   ol.Address,
		Line:      make([]*googleProfile.Line, len(ol.Line)),
	}

	for i, line := range ol.Line {
		gl.Line[i], err = p.convertLineBack(line, dictionary)
		if err != nil {
			return 0, fmt.Errorf("could not process line at index %d: %w", i, err)
		}
	}

	p.dst.Location = append(p.dst.Location, gl)
	gl.Id = uint64(len(p.dst.Location))
	p.locationMap[ol] = gl.Id
	return gl.Id, nil
}

// convertLineBack converts an OpenTelemetry Line to a Google Line.
func (p *profileBuilder) convertLineBack(ol *otelProfile.Line, dictionary *otelProfile.ProfilesDictionary) (*googleProfile.Line, error) {
	function, err := at(dictionary.FunctionTable, ol.FunctionIndex)
	if err != nil {
		return nil, fmt.Errorf("could not access function: %w", err)
	}
	functionId, err := p.convertFunctionBack(function, dictionary)
	if err != nil {
		return nil, err
	}
	return &googleProfile.Line{FunctionId: functionId, Line: ol.Line}, nil
}

func (p *profileBuilder) convertFunctionBack(of *otelProfile.Function, dictionary *otelProfile.ProfilesDictionary) (uint64, error) {
	if i, ok := p.functionMap[of]; ok {
		return i, nil
	}
	nameLabel, err := at(dictionary.StringTable, of.NameStrindex)
	if err != nil {
		return 0, fmt.Errorf("could not access function name string: %w", err)
	}
	systemNameLabel, err := at(dictionary.StringTable, of.SystemNameStrindex)
	if err != nil {
		return 0, fmt.Errorf("could not access function system name string: %w", err)
	}
	filenameLabel, err := at(dictionary.StringTable, of.FilenameStrindex)
	if err != nil {
		return 0, fmt.Errorf("could not access function file name string: %w", err)
	}
	gf := &googleProfile.Function{
		Name:       p.addstr(nameLabel),
		SystemName: p.addstr(systemNameLabel),
		Filename:   p.addstr(filenameLabel),
		StartLine:  of.StartLine,
	}
	p.dst.Function = append(p.dst.Function, gf)
	gf.Id = uint64(len(p.dst.Function))
	p.functionMap[of] = gf.Id
	return gf.Id, nil
}

func (p *profileBuilder) convertSampleBack(os *otelProfile.Sample, dictionary *otelProfile.ProfilesDictionary) (*googleProfile.Sample, error) {
	gs := &googleProfile.Sample{
		Value: os.Values,
	}

	// According to spec, samples can come without values, in which case we assume that each timestamp occurrence has value of 1.
	// See: https://github.com/open-telemetry/opentelemetry-proto/blob/81d6676cdc30dddb0ec1f87d080e6dac07ab214f/opentelemetry/proto/profiles/v1development/profiles.proto#L351-L353
	if len(gs.Value) == 0 && len(os.TimestampsUnixNano) > 0 {
		gs.Value = []int64{int64(len(os.TimestampsUnixNano))}
	}

	if len(gs.Value) == 0 {
		return nil, fmt.Errorf("sample value is required")
	}

	for i, typ := range p.sampleProcessingTypes {
		switch typ {
		case sampleConversionTypeSamplesToNanos:
			gs.Value[i] *= p.src.Period
		case sampleConversionTypeSumEvents:
			// For off-CPU profiles, aggregate all sample values into a single sum
			// since pprof cannot represent variable-length sample values
			sum := int64(0)
			for _, v := range gs.Value {
				sum += v
			}
			gs.Value = []int64{sum}
		}
	}
	if p.dst.Period != 0 && p.dst.PeriodType != nil && len(gs.Value) != len(p.dst.SampleType) {
		return nil, fmt.Errorf("sample values length mismatch %d %d", len(gs.Value), len(p.dst.SampleType))
	}

	err := p.convertSampleAttributesToLabelsBack(os, dictionary, gs)
	if err != nil {
		return nil, err
	}

	stackIndex := os.GetStackIndex()
	if stackIndex < 0 || int(stackIndex) >= len(dictionary.StackTable) {
		return nil, fmt.Errorf("invalid stack index: %d", stackIndex)
	}
	stack := dictionary.StackTable[stackIndex]
	for _, olocIdx := range stack.LocationIndices {
		oloc, err := at(dictionary.LocationTable, olocIdx)
		if err != nil {
			return nil, fmt.Errorf("could not access location at index %d: %w", olocIdx, err)
		}
		loc, err := p.convertLocationBack(oloc, dictionary)
		if err != nil {
			return nil, err
		}
		gs.LocationId = append(gs.LocationId, loc)
	}

	p.dst.Sample = append(p.dst.Sample, gs)

	return gs, nil
}

func (p *profileBuilder) convertSampleAttributesToLabelsBack(os *otelProfile.Sample, dictionary *otelProfile.ProfilesDictionary, gs *googleProfile.Sample) error {
	gs.Label = make([]*googleProfile.Label, 0, len(os.AttributeIndices))
	for i, attributeIdx := range os.AttributeIndices {
		attribute, err := at(dictionary.AttributeTable, attributeIdx)
		if err != nil {
			return fmt.Errorf("could not access attribute at index %d: %w", i, err)
		}
		if keyStr, err := at(dictionary.StringTable, attribute.KeyStrindex); err == nil && keyStr == serviceNameKey {
			continue
		}
		if attribute.Value.GetStringValue() != "" {
			keyStr, err := at(dictionary.StringTable, attribute.KeyStrindex)
			if err != nil {
				return fmt.Errorf("could not access attribute key: %w", err)
			}
			gs.Label = append(gs.Label, &googleProfile.Label{
				Key: p.addstr(keyStr),
				Str: p.addstr(attribute.Value.GetStringValue()),
			})
		}
	}

	if os.GetLinkIndex() != 0 {
		link, err := at(dictionary.LinkTable, os.GetLinkIndex())
		if err != nil {
			return fmt.Errorf("could not access link at index %d: %w", os.GetLinkIndex(), err)
		}
		gs.Label = append(gs.Label, &googleProfile.Label{
			Key: p.addstr(pprof.SpanIDLabelName),
			Str: p.addstr(hex.EncodeToString(link.GetSpanId())),
		})
	}

	return nil
}

// convertMappingsBack converts a slice of OpenTelemetry Mapping entries to Google Mapping entries.
func (p *profileBuilder) convertMappingBack(om *otelProfile.Mapping, dictionary *otelProfile.ProfilesDictionary) (uint64, error) {
	if i, ok := p.mappingMap[om]; ok {
		return i, nil
	}

	buildID, _ := getAttributeValueByKeyOrEmpty(om.AttributeIndices, dictionary, "process.executable.build_id.gnu")
	filenameLabel, err := at(dictionary.StringTable, om.FilenameStrindex)
	if err != nil {
		return 0, fmt.Errorf("could not access mapping file name string: %w", err)
	}
	gm := &googleProfile.Mapping{
		MemoryStart:     om.MemoryStart,
		MemoryLimit:     om.MemoryLimit,
		FileOffset:      om.FileOffset,
		Filename:        p.addstr(filenameLabel),
		BuildId:         p.addstr(buildID),
		HasFunctions:    true,
		HasFilenames:    true,
		HasLineNumbers:  true,
		HasInlineFrames: true,
	}
	p.dst.Mapping = append(p.dst.Mapping, gm)
	gm.Id = uint64(len(p.dst.Mapping))
	p.mappingMap[om] = gm.Id
	return gm.Id, nil
}

// This function extracts the value of a specific attribute key from a list of attribute indices.
// TODO: build a map instead of iterating every time.
func getAttributeValueByKeyOrEmpty(attributeIndices []int32, dictionary *otelProfile.ProfilesDictionary, key string) (string, error) {
	for i, attributeIndex := range attributeIndices {
		attr, err := at(dictionary.AttributeTable, attributeIndex)
		if err != nil {
			return "", fmt.Errorf("attribute not found: %d: %w", i, err)
		}
		keyStr, err := at(dictionary.StringTable, attr.KeyStrindex)
		if err != nil {
			return "", fmt.Errorf("attribute key string not found %d: %w", i, err)
		}
		if keyStr == key {
			return attr.Value.GetStringValue(), nil
		}
	}
	return "", nil
}
