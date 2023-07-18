package types

import "github.com/grafana/pyroscope/pkg/og/agent/spy"

const (
	DefaultSampleRate = 100 // 100 times per second
	GoSpy             = spy.Go
	PySpy             = spy.Python
	RbSpy             = spy.Ruby
)

var DefaultProfileTypes = []spy.ProfileType{
	spy.ProfileCPU,
	spy.ProfileAllocObjects,
	spy.ProfileAllocSpace,
	spy.ProfileInuseObjects,
	spy.ProfileInuseSpace,
}
