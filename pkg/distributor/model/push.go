package model

import (
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/distributor/annotation"
	"github.com/grafana/pyroscope/pkg/distributor/ingestlimits"
	"github.com/grafana/pyroscope/pkg/distributor/sampling"
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

// todo better name
type ProfileSeries struct {
	// Caller provided, modified during processing
	Labels     []*v1.LabelPair
	Profile    *pprof.Profile
	RawProfile []byte // may be nil if the Profile is composed not from pprof ( e.g. jfr)
	ID         string

	// todo split
	// Transient state
	TenantID string
	Language string

	Annotations []*v1.ProfileAnnotation

	// always 1 todo delete
	TotalProfiles          int64
	TotalBytesUncompressed int64

	DiscardedProfilesRelabeling int64
	DiscardedBytesRelabeling    int64
}

func (req *ProfileSeries) GetLanguage() string {
	spyName := phlaremodel.Labels(req.Labels).Get(phlaremodel.LabelNamePyroscopeSpy)
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
		TotalBytesUncompressed: req.TotalBytesUncompressed,
		Labels:                 phlaremodel.Labels(req.Labels).Clone(),
		Profile:                &pprof.Profile{Profile: req.Profile.CloneVT()},
		RawProfile:             nil,
		ID:                     req.ID,
		Language:               req.Language,
		Annotations:            req.Annotations,
	}
	return c
}

func (req *ProfileSeries) MarkThrottledTenant(l *ingestlimits.Config) error {
	a, err := annotation.CreateTenantAnnotation(l)
	if err != nil {
		return err
	}
	req.Annotations = append(req.Annotations, &v1.ProfileAnnotation{
		Key:   annotation.ProfileAnnotationKeyThrottled,
		Value: string(a),
	})
	return nil
}

func (req *ProfileSeries) MarkThrottledUsageGroup(l *ingestlimits.Config, usageGroup string) error {
	a, err := annotation.CreateUsageGroupAnnotation(l, usageGroup)
	if err != nil {
		return err
	}
	req.Annotations = append(req.Annotations, &v1.ProfileAnnotation{
		Key:   annotation.ProfileAnnotationKeyThrottled,
		Value: string(a),
	})
	return nil
}

func (req *ProfileSeries) MarkSampledRequest(source *sampling.Source) error {
	a, err := annotation.CreateProfileAnnotation(source)
	if err != nil {
		return err
	}
	req.Annotations = append(req.Annotations, &v1.ProfileAnnotation{
		Key:   annotation.ProfileAnnotationKeySampled,
		Value: string(a),
	})
	return nil
}
