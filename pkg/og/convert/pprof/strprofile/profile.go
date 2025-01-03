package strprofile

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

type Options struct {
	NoPrettyPrint bool
	NoDuration    bool
	NoTime        bool
	NoCompact     bool
	IncludeIDs    bool
}

type Location struct {
	ID      uint64   `json:"id,omitempty"`
	Address string   `json:"address,omitempty"`
	Lines   []Line   `json:"lines,omitempty"`
	Mapping *Mapping `json:"mapping,omitempty"`
}

type Line struct {
	Function *Function `json:"function"`
	Line     int64     `json:"line,omitempty"`
}

type Function struct {
	ID         uint64 `json:"id,omitempty"`
	Name       string `json:"name"`
	SystemName string `json:"system_name,omitempty"`
	Filename   string `json:"filename,omitempty"`
	StartLine  int64  `json:"start_line,omitempty"`
}

type Mapping struct {
	ID       uint64 `json:"id,omitempty"`
	Start    string `json:"start"`
	Limit    string `json:"limit"`
	Offset   string `json:"offset,omitempty"`
	Filename string `json:"filename,omitempty"`
	BuildID  string `json:"build_id,omitempty"`
}

type SampleType struct {
	Type string `json:"type"`
	Unit string `json:"unit"`
}

type Label struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Sample struct {
	Locations []Location `json:"locations,omitempty"`
	Values    []int64    `json:"values"`
	Labels    []Label    `json:"labels,omitempty"`
}

type Profile struct {
	SampleTypes   []SampleType `json:"sample_types"`
	Samples       []Sample     `json:"samples"`
	TimeNanos     string       `json:"time_nanos,omitempty"`
	DurationNanos string       `json:"duration_nanos,omitempty"`
	Period        string       `json:"period,omitempty"`
}

type CompactLocation struct {
	ID      uint64   `json:"id,omitempty"`
	Address string   `json:"address,omitempty"`
	Lines   []string `json:"lines,omitempty"`
	Mapping string   `json:"mapping,omitempty"`
}

type CompactSample struct {
	Locations []CompactLocation `json:"locations,omitempty"`
	Values    string            `json:"values"`
	Labels    string            `json:"labels,omitempty"`
}

type CompactProfile struct {
	SampleTypes   []SampleType    `json:"sample_types"`
	Samples       []CompactSample `json:"samples"`
	TimeNanos     string          `json:"time_nanos,omitempty"`
	DurationNanos string          `json:"duration_nanos,omitempty"`
	Period        string          `json:"period,omitempty"`
}

func Stringify(p *profilev1.Profile, opts Options) (string, error) {
	var err error
	var sp interface{}

	if !opts.NoCompact {
		sp = ToCompactProfile(p, opts)
	} else {
		sp = ToDetailedProfile(p, opts)
	}

	var jsonData []byte
	if !opts.NoPrettyPrint {
		jsonData, err = json.MarshalIndent(sp, "", "  ")
	} else {
		jsonData, err = json.Marshal(sp)
	}
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

func ToDetailedProfile(p *profilev1.Profile, opts Options) Profile {
	sp := Profile{
		Period: fmt.Sprintf("%d", p.Period),
	}
	if !opts.NoTime {
		sp.TimeNanos = fmt.Sprintf("%d", p.TimeNanos)
	}
	if !opts.NoDuration {
		sp.DurationNanos = fmt.Sprintf("%d", p.DurationNanos)
	}

	for _, st := range p.SampleType {
		sp.SampleTypes = append(sp.SampleTypes, SampleType{
			Type: p.StringTable[st.Type],
			Unit: p.StringTable[st.Unit],
		})
	}

	// Create maps for quick lookups
	functionMap := make(map[uint64]*profilev1.Function)
	for _, f := range p.Function {
		functionMap[f.Id] = f
	}

	mappingMap := make(map[uint64]*profilev1.Mapping)
	for _, m := range p.Mapping {
		mappingMap[m.Id] = m
	}

	for _, sample := range p.Sample {
		ss := Sample{
			Values: sample.Value,
		}

		for _, locID := range sample.LocationId {
			loc := findLocation(p.Location, locID)
			if loc == nil {
				continue
			}

			sLoc := Location{
				Address: fmt.Sprintf("0x%x", loc.Address),
			}
			if opts.IncludeIDs {
				sLoc.ID = loc.Id
			}

			if loc.MappingId != 0 {
				if mapping := mappingMap[loc.MappingId]; mapping != nil {
					m := &Mapping{
						Start:    fmt.Sprintf("0x%x", mapping.MemoryStart),
						Limit:    fmt.Sprintf("0x%x", mapping.MemoryLimit),
						Offset:   fmt.Sprintf("0x%x", mapping.FileOffset),
						Filename: p.StringTable[mapping.Filename],
						BuildID:  p.StringTable[mapping.BuildId],
					}
					if opts.IncludeIDs {
						m.ID = mapping.Id
					}
					sLoc.Mapping = m
				}
			}

			for _, line := range loc.Line {
				if fn := functionMap[line.FunctionId]; fn != nil {
					f := &Function{
						Name:       p.StringTable[fn.Name],
						SystemName: p.StringTable[fn.SystemName],
						Filename:   p.StringTable[fn.Filename],
						StartLine:  fn.StartLine,
					}
					if opts.IncludeIDs {
						f.ID = fn.Id
					}
					sLine := Line{
						Function: f,
						Line:     line.Line,
					}
					sLoc.Lines = append(sLoc.Lines, sLine)
				}
			}

			ss.Locations = append(ss.Locations, sLoc)
		}

		if len(sample.Label) > 0 {
			ss.Labels = make([]Label, 0, len(sample.Label))
			for _, label := range sample.Label {
				key := p.StringTable[label.Key]
				var value string
				if label.Str != 0 {
					value = p.StringTable[label.Str]
				} else {
					value = strconv.FormatInt(label.Num, 10)
				}
				ss.Labels = append(ss.Labels, Label{
					Key:   key,
					Value: value,
				})
			}
		}

		sp.Samples = append(sp.Samples, ss)
	}
	return sp
}

func ToCompactProfile(p *profilev1.Profile, opts Options) CompactProfile {
	sp := CompactProfile{
		Period: fmt.Sprintf("%d", p.Period),
	}
	if !opts.NoTime {
		sp.TimeNanos = fmt.Sprintf("%d", p.TimeNanos)
	}
	if !opts.NoDuration {
		sp.DurationNanos = fmt.Sprintf("%d", p.DurationNanos)
	}

	for _, st := range p.SampleType {
		sp.SampleTypes = append(sp.SampleTypes, SampleType{
			Type: p.StringTable[st.Type],
			Unit: p.StringTable[st.Unit],
		})
	}

	functionMap := make(map[uint64]*profilev1.Function)
	for _, f := range p.Function {
		functionMap[f.Id] = f
	}

	mappingMap := make(map[uint64]*profilev1.Mapping)
	for _, m := range p.Mapping {
		mappingMap[m.Id] = m
	}

	for _, sample := range p.Sample {
		values := make([]string, len(sample.Value))
		for i, v := range sample.Value {
			values[i] = strconv.FormatInt(v, 10)
		}

		ss := CompactSample{
			Values: strings.Join(values, ","),
		}

		for _, locID := range sample.LocationId {
			loc := findLocation(p.Location, locID)
			if loc == nil {
				continue
			}

			sLoc := CompactLocation{
				Address: fmt.Sprintf("0x%x", loc.Address),
			}
			if opts.IncludeIDs {
				sLoc.ID = loc.Id
			}

			if loc.MappingId != 0 {
				if mapping := mappingMap[loc.MappingId]; mapping != nil {
					idStr := ""
					if opts.IncludeIDs {
						idStr = fmt.Sprintf("[id=%d]", mapping.Id)
					}
					sLoc.Mapping = fmt.Sprintf("0x%x-0x%x@0x%x %s(%s)%s",
						mapping.MemoryStart,
						mapping.MemoryLimit,
						mapping.FileOffset,
						p.StringTable[mapping.Filename],
						p.StringTable[mapping.BuildId],
						idStr)
				}
			}

			for _, line := range loc.Line {
				if fn := functionMap[line.FunctionId]; fn != nil {
					idStr := ""
					if opts.IncludeIDs {
						idStr = fmt.Sprintf("[id=%d]", fn.Id)
					}
					lineStr := fmt.Sprintf("%s[%s]@%s:%d%s",
						p.StringTable[fn.Name],
						p.StringTable[fn.SystemName],
						p.StringTable[fn.Filename],
						line.Line,
						idStr)
					sLoc.Lines = append(sLoc.Lines, lineStr)
				}
			}

			ss.Locations = append(ss.Locations, sLoc)
		}

		if len(sample.Label) > 0 {
			labels := make([]string, 0, len(sample.Label))
			for _, label := range sample.Label {
				key := p.StringTable[label.Key]
				var value string
				if label.Str != 0 {
					value = p.StringTable[label.Str]
				} else {
					value = strconv.FormatInt(label.Num, 10)
				}
				labels = append(labels, fmt.Sprintf("%s=%s", key, value))
			}
			sort.Strings(labels)
			ss.Labels = strings.Join(labels, ",")
		}

		sp.Samples = append(sp.Samples, ss)
	}
	return sp
}

func findLocation(locations []*profilev1.Location, id uint64) *profilev1.Location {
	for _, loc := range locations {
		if loc.Id == id {
			return loc
		}
	}
	return nil
}
