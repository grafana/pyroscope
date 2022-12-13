package cumulativepprof

import (
	"fmt"
	pprofile "github.com/google/pprof/profile"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type Merger struct {
	SampleTypes      []string
	MergeRatios      []float64
	SampleTypeConfig map[string]*tree.SampleTypeConfig
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
			SampleTypeConfig: map[string]*tree.SampleTypeConfig{
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
			SampleTypeConfig: map[string]*tree.SampleTypeConfig{
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
			SampleTypeConfig: map[string]*tree.SampleTypeConfig{
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

func (m *Mergers) Merge(prev, cur []byte, sampleTypeConfig map[string]*tree.SampleTypeConfig) (*pprofile.Profile, map[string]*tree.SampleTypeConfig, error) {
	p, err := pprofile.ParseData(cur)
	if err != nil {
		return nil, nil, err
	}
	if len(p.SampleType) == 4 {
		for i := 0; i < 4; i++ {
			if p.SampleType[i].Type != m.Heap.SampleTypes[i] {
				return nil, nil, fmt.Errorf("unknown sample type order %v", p.SampleType)
			}
		}
		cfg, ok := sampleTypeConfig["alloc_objects"]
		if cfg != nil && ok {
			if !cfg.Cumulative {
				return nil, nil, fmt.Errorf("alloc_objects profile is already not cumulative: %v", sampleTypeConfig)
			}
		}
		return m.Heap.Merge(prev, p)
	}
	if len(sampleTypeConfig) == 2 {
		for i := 0; i < 2; i++ {
			if p.SampleType[i].Type != m.Block.SampleTypes[i] {
				return nil, nil, fmt.Errorf("unknown sample type order %v", p.SampleType)
			}
		}
		cfg, ok := sampleTypeConfig["contentions"]
		if cfg != nil && ok {
			if !cfg.Cumulative {
				return nil, nil, fmt.Errorf("contentions profile is already not cumulative: %v", sampleTypeConfig)
			}
			if cfg.DisplayName == "mutex_count" {
				return m.Mutex.Merge(prev, p)
			}
			if cfg.DisplayName == "block_count" {
				return m.Block.Merge(prev, p)
			}
		}
		return nil, nil, fmt.Errorf("unkown profile: %v %v", p.SampleType, sampleTypeConfig)
	}
	return nil, nil, fmt.Errorf("unknown profile %v %v", p.SampleType, sampleTypeConfig)
}

func (m *Merger) Merge(prev []byte, cur *pprofile.Profile) (*pprofile.Profile, map[string]*tree.SampleTypeConfig, error) {
	var err error
	p2 := cur

	p1 := m.prev
	if p1 == nil {
		p1, err = pprofile.ParseData(prev)
		if err != nil {
			return nil, nil, err
		}
	}

	err = p1.ScaleN(m.MergeRatios)
	if err != nil {
		return nil, nil, err
	}

	p, err := pprofile.Merge([]*pprofile.Profile{p1, p2})
	if err != nil {
		return nil, nil, err
	}

	for _, sample := range p.Sample {
		if len(sample.Value) > 0 && sample.Value[0] < 0 {
			for i := range sample.Value {
				sample.Value[i] = 0
			}
		}
	}

	m.prev = p2
	return p, m.SampleTypeConfig, nil
}
