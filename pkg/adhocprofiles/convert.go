package adhocprofiles

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/grafana/pyroscope/pkg/og/agent/spy"
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

func PprofToProfile(b []byte, name string, maxNodes int) (_ []*flamebearer.FlamebearerProfile, err error) {
	var p tree.Profile
	if err := pprof.Decode(bytes.NewReader(b), &p); err != nil {
		return nil, fmt.Errorf("parsing pprof: %w", err)
	}
	fbs := make([]*flamebearer.FlamebearerProfile, 0)
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
		err := p.Get(stype, func(_labels *spy.Labels, name []byte, val int) error {
			t.Insert(name, uint64(val))
			return nil
		})
		if err != nil {
			return nil, err
		}
		finalName := name
		if len(p.SampleTypes()) > 1 {
			finalName = stype
		}
		fb := flamebearer.NewProfile(flamebearer.ProfileConfig{
			Tree:     t,
			Name:     finalName,
			MaxNodes: maxNodes,
			Metadata: metadata.Metadata{
				SpyName:    "unknown",
				SampleRate: sampleRate,
				Units:      units,
			},
		})
		fbs = append(fbs, &fb)
	}
	if len(fbs) == 0 {
		return nil, errors.New("no supported sample type found")
	}
	return fbs, nil
}
