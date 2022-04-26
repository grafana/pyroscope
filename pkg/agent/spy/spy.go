// Package spy contains an interface (Spy) and functionality to register new spies
package spy

import (
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
)

type Spy interface {
	Stop() error
	Snapshot(cb func(*Labels, []byte, uint64) error) error
}

type Resettable interface {
	Reset()
}

type ProfileType string

const (
	ProfileCPU          ProfileType = "cpu"
	ProfileInuseObjects ProfileType = "inuse_objects"
	ProfileAllocObjects ProfileType = "alloc_objects"
	ProfileInuseSpace   ProfileType = "inuse_space"
	ProfileAllocSpace   ProfileType = "alloc_space"

	Go     = "gospy"
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

// TODO: this interface is not the best as different spies have different arguments
type SpyIntitializer func(pid int, profileType ProfileType, sampleRate uint32, disableGCRuns bool) (Spy, error)

var (
	supportedSpiesMap map[string]SpyIntitializer
	SupportedSpies    []string
)

var autoDetectionMapping = map[string]string{
	"python":  "pyspy",
	"python2": "pyspy",
	"python3": "pyspy",
	"uwsgi":   "pyspy",
	"pipenv":  "pyspy",

	"php": "phpspy",

	"ruby":   "rbspy",
	"bundle": "rbspy",
	"rails":  "rbspy",
	"rake":   "rbspy",

	"dotnet": "dotnetspy",
}

func init() {
	supportedSpiesMap = make(map[string]SpyIntitializer)
}

func RegisterSpy(name string, cb SpyIntitializer) {
	SupportedSpies = append(SupportedSpies, name)
	supportedSpiesMap[name] = cb
}

func StartFunc(name string) (SpyIntitializer, error) {
	if s, ok := supportedSpiesMap[name]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("unknown spy \"%s\". Make sure it's supported (run `pyroscope version` to check if your version supports it)", name)
}

func ResolveAutoName(s string) string {
	return autoDetectionMapping[s]
}

func SupportedExecSpies() []string {
	supportedSpies := []string{}
	for _, s := range SupportedSpies {
		if s != Go {
			supportedSpies = append(supportedSpies, s)
		}
	}

	return supportedSpies
}
