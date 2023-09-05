package model

import (
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type PushRequest struct {
	CompressedSize        int
	CompressedProfileType string // "jfr"

	Series []*ProfileSeries
}

type ProfileSample struct {
	Profile *pprof.Profile
	Raw     []byte // may be nil if the Profile is composed not from pprof ( e.g. jfr)
	ID      string
}

type ProfileSeries struct {
	Labels  []*v1.LabelPair
	Samples []*ProfileSample
}
