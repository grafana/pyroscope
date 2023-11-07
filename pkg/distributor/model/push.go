package model

import (
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type RawProfileType string

const RawProfileTypePPROF = RawProfileType("pprof")
const RawProfileTypeJFR = RawProfileType("jfr")

type PushRequest struct {
	RawProfileSize int
	RawProfileType RawProfileType

	Series []*ProfileSeries

	TotalProfiles          int64
	TotalBytesUncompressed int64
}

type ProfileSample struct {
	Profile    *pprof.Profile
	RawProfile []byte // may be nil if the Profile is composed not from pprof ( e.g. jfr)
	ID         string
}

type ProfileSeries struct {
	Labels  []*v1.LabelPair
	Samples []*ProfileSample
}
