package strprofile

import (
	"encoding/json"
	"fmt"
	"strconv"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

// Options controls what information is included in the stringified output
type Options struct {
	NoPrettyPrint bool
}

// Location represents a denormalized location
type Location struct {
	Address string   `json:"address,omitempty"`
	Lines   []Line   `json:"lines,omitempty"`
	Mapping *Mapping `json:"mapping,omitempty"`
}

type Line struct {
	Function *Function `json:"function"`
	Line     int64     `json:"line,omitempty"`
}

type Function struct {
	Name       string `json:"name"`
	SystemName string `json:"system_name,omitempty"`
	Filename   string `json:"filename,omitempty"`
	StartLine  int64  `json:"start_line,omitempty"`
}

type Mapping struct {
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

// Stringify converts a profile to a human-readable JSON representation
func Stringify(p *profilev1.Profile, opts Options) (string, error) {
	sp := Profile{
		TimeNanos:     fmt.Sprintf("%d", p.TimeNanos),
		DurationNanos: fmt.Sprintf("%d", p.DurationNanos),
		Period:        fmt.Sprintf("%d", p.Period),
	}

	// Process sample types
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

	// Process samples
	for _, sample := range p.Sample {
		ss := Sample{
			Values: sample.Value,
		}

		// Process locations
		for _, locID := range sample.LocationId {
			loc := findLocation(p.Location, locID)
			if loc == nil {
				continue
			}

			sLoc := Location{
				Address: fmt.Sprintf("0x%x", loc.Address),
			}

			// Process mapping
			if loc.MappingId != 0 {
				if mapping := mappingMap[loc.MappingId]; mapping != nil {
					sLoc.Mapping = &Mapping{
						Start:    fmt.Sprintf("0x%x", mapping.MemoryStart),
						Limit:    fmt.Sprintf("0x%x", mapping.MemoryLimit),
						Offset:   fmt.Sprintf("0x%x", mapping.FileOffset),
						Filename: p.StringTable[mapping.Filename],
						BuildID:  p.StringTable[mapping.BuildId],
					}
				}
			}

			// Process lines
			for _, line := range loc.Line {
				if fn := functionMap[line.FunctionId]; fn != nil {
					sLine := Line{
						Function: &Function{
							Name:       p.StringTable[fn.Name],
							SystemName: p.StringTable[fn.SystemName],
							Filename:   p.StringTable[fn.Filename],
							StartLine:  fn.StartLine,
						},
						Line: line.Line,
					}
					sLoc.Lines = append(sLoc.Lines, sLine)
				}
			}

			ss.Locations = append(ss.Locations, sLoc)
		}

		// Process labels
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

	var jsonData []byte
	var err error
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

// Helper function to find a location by ID
func findLocation(locations []*profilev1.Location, id uint64) *profilev1.Location {
	for _, loc := range locations {
		if loc.Id == id {
			return loc
		}
	}
	return nil
}
