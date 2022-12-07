package cumulativepprof

import (
	"bytes"
	"fmt"
	pprofile "github.com/google/pprof/profile"
	"github.com/pyroscope-io/client/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type Merger struct {
	SampleTypes      []string
	MergeRatios      []float64
	SampleTypeConfig map[string]*upstream.SampleType
	Name             string

	prev *pprofile.Profile
}

type Mergers struct {
	Heap  *Merger
	Block *Merger
	Mutex *Merger
}

func NewMergers() *Mergers {
	return &Mergers{
		Block: &Merger{
			SampleTypes: []string{"contentions", "delay"},
			MergeRatios: []float64{-1, -1},
			SampleTypeConfig: map[string]*upstream.SampleType{
				"contentions": {
					DisplayName: "block_count",
					Units:       "lock_samples",
				},
				"delay": {
					DisplayName: "block_duration",
					Units:       "lock_nanoseconds",
				},
			},
			Name: "block",
		},
		Mutex: &Merger{
			SampleTypes: []string{"contentions", "delay"},
			MergeRatios: []float64{-1, -1},
			SampleTypeConfig: map[string]*upstream.SampleType{
				"contentions": {
					DisplayName: "mutex_count",
					Units:       "lock_samples",
				},
				"delay": {
					DisplayName: "mutex_duration",
					Units:       "lock_nanoseconds",
				},
			},
			Name: "mutex",
		},
		Heap: &Merger{
			SampleTypes: []string{"alloc_objects", "alloc_space", "inuse_objects", "inuse_space"},
			MergeRatios: []float64{-1, -1, 0, 0},
			SampleTypeConfig: map[string]*upstream.SampleType{
				"alloc_objects": {
					Units: "objects",
				},
				"alloc_space": {
					Units: "bytes",
				},
				"inuse_space": {
					Units:       "bytes",
					Aggregation: "average",
				},
				"inuse_objects": {
					Units:       "objects",
					Aggregation: "average",
				},
			},
			Name: "heap",
		},
	}
}

func (m *Mergers) SelectMerger(sampleTypeConfig map[string]*tree.SampleTypeConfig) *Merger {
	if len(sampleTypeConfig) == 4 {
		cfg, ok := sampleTypeConfig["alloc_objects"]
		if cfg != nil && ok {
			if !cfg.Cumulative {
				return nil
			}
			return m.Heap
		} else {
			return nil
		}
	}
	if len(sampleTypeConfig) == 2 {
		cfg, ok := sampleTypeConfig["contentions"]
		if cfg != nil && ok {
			if !cfg.Cumulative {
				return nil
			}
			if cfg.DisplayName == "mutex_count" {
				return m.Mutex
			}
			if cfg.DisplayName == "block_count" {
				return m.Block
			}
			return nil
		} else {
			return nil
		}
	}
	return nil
}

func (m *Merger) Merge(prev, cur []byte) (*pprofile.Profile, error) {
	p2, err := m.parseProfile(cur)
	if err != nil {
		return nil, err
	}
	p1 := m.prev
	if p1 == nil {
		p1, err = m.parseProfile(prev)
		if err != nil {
			return nil, err
		}
	}

	err = p1.ScaleN(m.MergeRatios)
	if err != nil {
		return nil, err
	}

	p, err := pprofile.Merge([]*pprofile.Profile{p1, p2})
	if err != nil {
		return nil, err
	}

	for _, sample := range p.Sample {
		if len(sample.Value) > 0 && sample.Value[0] < 0 {
			for i := range sample.Value {
				sample.Value[i] = 0
			}
		}
	}

	m.prev = p2
	return p, nil
}

func (m *Merger) parseProfile(bs []byte) (*pprofile.Profile, error) {
	var prof = bytes.NewBuffer(bs)
	p, err := pprofile.Parse(prof)
	if err != nil {
		return nil, err
	}
	if got := len(p.SampleType); got != len(m.SampleTypes) {
		return nil, fmt.Errorf("invalid  profile: got %d sample types, want %d", got, len(m.SampleTypes))
	}
	for i, want := range m.SampleTypes {
		if got := p.SampleType[i].Type; got != want {
			return nil, fmt.Errorf("invalid profile: got %q sample type at index %d, want %q", got, i, want)
		}
	}
	return p, nil
}
