// Package spy contains an interface (Spy) and functionality to register new spies
package spy

import (
	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
)

type ProfileType string

const (
	ProfileCPU          ProfileType = "cpu"
	ProfileInuseObjects ProfileType = "inuse_objects"
	ProfileAllocObjects ProfileType = "alloc_objects"
	ProfileInuseSpace   ProfileType = "inuse_space"
	ProfileAllocSpace   ProfileType = "alloc_space"

	Go     = "gospy"
	EBPF   = "ebpfspy"
	Python = "pyspy"
	Ruby   = "rbspy"
)

func (t ProfileType) IsCumulative() bool {
	return t == ProfileAllocObjects || t == ProfileAllocSpace
}

func (t ProfileType) Units() metadata.Units {
	if t == ProfileInuseObjects || t == ProfileAllocObjects {
		return metadata.ObjectsUnits
	}
	if t == ProfileInuseSpace || t == ProfileAllocSpace {
		return metadata.BytesUnits
	}

	return metadata.SamplesUnits
}

func (t ProfileType) AggregationType() metadata.AggregationType {
	if t == ProfileInuseObjects || t == ProfileInuseSpace {
		return metadata.AverageAggregationType
	}

	return metadata.SumAggregationType
}
