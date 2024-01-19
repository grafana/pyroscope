package model

import (
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
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
