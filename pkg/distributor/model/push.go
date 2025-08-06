package model

import (
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/distributor/ingestlimits"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type RawProfileType string

const RawProfileTypePPROF = RawProfileType("pprof")
const RawProfileTypeJFR = RawProfileType("jfr")
const RawProfileTypeOTEL = RawProfileType("otel")

type PushRequest struct {
	Series []*ProfileSeries

	ReceivedCompressedProfileSize int
	RawProfileType                RawProfileType
}

type ProfileSample struct {
	Profile    *pprof.Profile
	RawProfile []byte // may be nil if the Profile is composed not from pprof ( e.g. jfr)
	ID         string
}

// todo better name
type ProfileSeries struct {
	// Caller provided, modified during processing
	Labels []*v1.LabelPair
	Sample *ProfileSample

	// todo split
	// Transient state
	TenantID string
	Language string

	Annotations []*v1.ProfileAnnotation

	// always 1
	TotalProfiles          int64
	TotalBytesUncompressed int64

	DiscardedProfilesRelabeling int64
	DiscardedBytesRelabeling    int64
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

func (req *ProfileSeries) Clone() *ProfileSeries {
	c := &ProfileSeries{
		TenantID:               req.TenantID,
		TotalProfiles:          req.TotalProfiles,
		TotalBytesUncompressed: req.TotalBytesUncompressed,
		Labels:                 phlaremodel.Labels(req.Labels).Clone(),
		Sample: &ProfileSample{
			Profile:    &pprof.Profile{Profile: req.Sample.Profile.CloneVT()},
			RawProfile: nil,
			ID:         req.Sample.ID,
		},
		Language:    req.Language,
		Annotations: req.Annotations,
	}
	return c
}

func (req *ProfileSeries) MarkThrottledTenant(l *ingestlimits.Config) error {
	annotation, err := ingestlimits.CreateTenantAnnotation(l)
	if err != nil {
		return err
	}
	req.Annotations = append(req.Annotations, &v1.ProfileAnnotation{
		Key:   ingestlimits.ProfileAnnotationKeyThrottled,
		Value: string(annotation),
	})
	return nil
}

func (req *ProfileSeries) MarkThrottledUsageGroup(l *ingestlimits.Config, usageGroup string) error {
	annotation, err := ingestlimits.CreateUsageGroupAnnotation(l, usageGroup)
	if err != nil {
		return err
	}
	req.Annotations = append(req.Annotations, &v1.ProfileAnnotation{
		Key:   ingestlimits.ProfileAnnotationKeyThrottled,
		Value: string(annotation),
	})
	return nil
}
