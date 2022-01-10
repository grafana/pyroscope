package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

func jsonToProfile(b []byte, _ string, _ int) (*flamebearer.FlamebearerProfile, error) {
	var profile flamebearer.FlamebearerProfile
	if err := json.Unmarshal(b, &profile); err != nil {
		return nil, fmt.Errorf("unable to unmarshall JSON: %w", err)
	}
	return &profile, nil
}

func pprofToProfile(b []byte, name string, maxNodes int) (*flamebearer.FlamebearerProfile, error) {
	p, err := convert.ParsePprof(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("parsing pprof: %w", err)
	}
	// TODO(abeaumont): Support multiple sample types
	for _, stype := range p.SampleTypes() {
		sampleRate := uint32(100)
		units := "samples"
		if c, ok := tree.DefaultSampleTypeMapping[stype]; ok {
			units = c.Units
			if c.Sampled && p.Period > 0 {
				sampleRate = uint32(time.Second / time.Duration(p.Period))
			}
		}
		t := tree.New()
		p.Get(stype, func(_labels *spy.Labels, name []byte, val int) {
			t.Insert(name, uint64(val))
		})

		out := &storage.GetOutput{
			Tree:       t,
			Units:      units,
			SpyName:    name,
			SampleRate: sampleRate,
		}
		profile := flamebearer.NewProfile(out, maxNodes)
		return &profile, nil
	}
	return nil, errors.New("no supported sample type found")
}

func collapsedToProfile(b []byte, name string, maxNodes int) (*flamebearer.FlamebearerProfile, error) {
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
	out := &storage.GetOutput{
		Tree:       t,
		SpyName:    name,
		SampleRate: 100, // We don't have this information, use the default
	}
	profile := flamebearer.NewProfile(out, maxNodes)
	return &profile, nil
}

func profileToTree(fb flamebearer.FlamebearerProfile) (*tree.Tree, error) {
	if fb.Metadata.Format != string(tree.FormatSingle) {
		return nil, fmt.Errorf("unsupported flamebearer format %s", fb.Metadata.Format)
	}
	return flamebearerV1ToTree(fb.Flamebearer)
}

func flamebearerV1ToTree(fb flamebearer.FlamebearerV1) (*tree.Tree, error) {
	t := tree.New()
	deltaDecoding(fb.Levels, 0, 4)
	for i, l := range fb.Levels {
		for j := 0; j < len(l); j += 4 {
			self := l[j+2]
			if self > 0 {
				t.InsertStackString(buildStack(fb, i, j), uint64(self))
			}
		}
	}
	return t, nil
}

func deltaDecoding(levels [][]int, start, step int) {
	for _, l := range levels {
		prev := 0
		for i := start; i < len(l); i += step {
			delta := l[i] + l[i+1]
			l[i] += prev
			prev += delta
		}
	}
}

func buildStack(fb flamebearer.FlamebearerV1, level, idx int) []string {
	stack := make([]string, level+1)
	stack[level] = fb.Names[fb.Levels[level][idx+3]]
	x := fb.Levels[level][idx]
	for i := level - 1; i >= 0; i-- {
		j := sort.Search(len(fb.Levels[i])/4, func(j int) bool { return fb.Levels[i][j*4] > x }) - 1
		stack[i] = fb.Names[fb.Levels[i][j*4+3]]
		x = fb.Levels[i][j*4]
	}
	return stack
}
