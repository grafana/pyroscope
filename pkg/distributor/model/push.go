package model

import (
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type RawProfileType string

const RawProfileTypePPROF = RawProfileType("pprof")
const RawProfileTypeJFR = RawProfileType("jfr")
const RawProfileTypeOTEL = RawProfileType("otel")

type PushRequest struct {
	TenantID       string
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
	Labels   []*v1.LabelPair
	Samples  []*ProfileSample
	Language string
}

func (p *ProfileSeries) GetLanguage() string {
	spyName := phlaremodel.Labels(p.Labels).Get(phlaremodel.LabelNamePyroscopeSpy)
	if spyName != "" {
		lang := getProfileLanguageFromSpy(spyName)
		if lang != "" {
			return lang
		}
	}
	return ""
}

func getProfileLanguageFromSpy(spyName string) string {
	switch spyName {
	default:
		return ""
	case "dotnetspy":
		return "dotnet"
	case "gospy":
		return "go"
	case "phpspy":
		return "php"
	case "pyspy":
		return "python"
	case "rbspy":
		return "ruby"
	case "nodespy":
		return "nodejs"
	case "javaspy", "grafana-agent.java":
		return "java"
	case "pyroscope-rs":
		return "rust"
	}
}

func (req *PushRequest) Clone() *PushRequest {
	c := &PushRequest{
		TenantID:               req.TenantID,
		RawProfileSize:         req.RawProfileSize,
		RawProfileType:         req.RawProfileType,
		Series:                 make([]*ProfileSeries, len(req.Series)),
		TotalProfiles:          req.TotalProfiles,
		TotalBytesUncompressed: req.TotalBytesUncompressed,
	}
	for i, s := range req.Series {
		c.Series[i] = &ProfileSeries{
			Labels:   phlaremodel.Labels(s.Labels).Clone(),
			Samples:  make([]*ProfileSample, len(s.Samples)),
			Language: s.Language,
		}
		for j, p := range s.Samples {
			c.Series[i].Samples[j] = &ProfileSample{
				Profile:    &pprof.Profile{Profile: p.Profile.Profile.CloneVT()},
				RawProfile: nil,
				ID:         p.ID,
			}
		}
	}
	return c
}
