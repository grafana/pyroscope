package model

import "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"

// TODO(kolesnikovae): Consider map alternatives.

type SampleMerge map[uint64]map[uint32]int64

func (m SampleMerge) Add(partition uint64, stacktraceID uint32, value int64) {
	p, ok := m[partition]
	if !ok {
		p = make(map[uint32]int64, 128)
		m[partition] = p
	}
	p[stacktraceID] += value
}

func (m SampleMerge) AddSamples(partition uint64, samples v1.Samples) {
	p, ok := m[partition]
	if !ok {
		p = make(map[uint32]int64, len(samples.StacktraceIDs))
		m[partition] = p
	}
	for i, sid := range samples.StacktraceIDs {
		p[sid] = int64(samples.Values[i])
	}
}

func (m SampleMerge) WriteSamples(partition uint64, dst *v1.Samples) {
	p, ok := m[partition]
	if !ok {
		return
	}
	dst.StacktraceIDs = dst.StacktraceIDs[:0]
	dst.Values = dst.Values[:0]
	for k, v := range p {
		dst.StacktraceIDs = append(dst.StacktraceIDs, k)
		dst.Values = append(dst.Values, uint64(v))
	}
}
