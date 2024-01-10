package pprof

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/prometheus/model/labels"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	v1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/og/util/form"
	"github.com/grafana/pyroscope/pkg/pprof"
)

type RawProfile struct {
	RawData             []byte // Represents raw request body as per ingestion API.
	FormDataContentType string // Set optionally, if RawData is multipart form.
	// Initializes lazily on handleRawData, if not present.
	Profile []byte // Represents raw pprof data.

	SampleTypeConfig map[string]*tree.SampleTypeConfig
}

func (p *RawProfile) ContentType() string {
	if p.FormDataContentType == "" {
		return "binary/octet-stream"
	}
	return p.FormDataContentType
}

const (
	formFieldProfile          = "profile"
	formFieldPreviousProfile  = "prev_profile"
	formFieldSampleTypeConfig = "sample_type_config"
)

// ParseToPprof is not doing much now. It parses the profile with no processing/splitting, adds labels.
func (p *RawProfile) ParseToPprof(_ context.Context, md ingestion.Metadata) (res *distributormodel.PushRequest, err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("/ingest pprof.(*RawProfile).ParseToPprof panic %v", r)
		}
	}()
	err = p.handleRawData()
	if err != nil {
		return nil, fmt.Errorf("failed to parse pprof /ingest multipart form %w", err)
	}
	res = &distributormodel.PushRequest{
		RawProfileSize: len(p.Profile),
		RawProfileType: distributormodel.RawProfileTypePPROF,
		Series:         nil,
	}
	if len(p.Profile) == 0 {
		return res, nil
	}

	profile, err := pprof.RawFromBytes(p.Profile)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	fixTime(profile, md)
	FixFunctionNamesForScriptingLanguages(profile, md)
	if p.isDotnetspy(md) {
		FixFunctionIDForBrokenDotnet(profile.Profile)
		fixSampleTypes(profile.Profile)
	}

	res.Series = []*distributormodel.ProfileSeries{{
		Labels: p.createLabels(profile, md),
		Samples: []*distributormodel.ProfileSample{{
			Profile:    profile,
			RawProfile: p.Profile,
		}},
	}}
	return
}

func (p *RawProfile) isDotnetspy(md ingestion.Metadata) bool {
	if md.SpyName == "dotnetspy" {
		return true
	}
	stc := p.getSampleTypes()
	return md.SpyName == "unknown" && stc != nil && stc["inuse-space"] != nil
}

func fixTime(profile *pprof.Profile, md ingestion.Metadata) {
	// for old versions of pyspy, rbspy, pyroscope-rs
	// https://github.com/grafana/pyroscope-rs/pull/134
	// profile.TimeNanos can be in microseconds
	x := time.Unix(0, profile.TimeNanos)
	if x.IsZero() || x.Year() == 1970 {
		profile.TimeNanos = md.StartTime.UnixNano()
	}
}

func (p *RawProfile) Parse(_ context.Context, _ storage.Putter, _ storage.MetricsExporter, md ingestion.Metadata) error {
	return fmt.Errorf("parsing pprof to tree/storage.Putter is nolonger ")
}

func (p *RawProfile) handleRawData() (err error) {
	if p.FormDataContentType != "" {
		// The profile was ingested as a multipart form. Load parts to
		// Profile, PreviousProfile, and SampleTypeConfig.
		if err := p.loadPprofFromForm(); err != nil {
			return err
		}
	} else {
		p.Profile = p.RawData
	}

	return nil
}

func (p *RawProfile) loadPprofFromForm() error {
	boundary, err := form.ParseBoundary(p.FormDataContentType)
	if err != nil {
		return err
	}

	f, err := multipart.NewReader(bytes.NewReader(p.RawData), boundary).ReadForm(32 << 20)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.RemoveAll()
	}()

	p.Profile, err = form.ReadField(f, formFieldProfile)
	if err != nil {
		return err
	}
	PreviousProfile, err := form.ReadField(f, formFieldPreviousProfile)
	if err != nil {
		return err
	}
	if PreviousProfile != nil {
		return fmt.Errorf("unsupported client version. " +
			"Please update github.com/grafana/pyroscope-go to the latest version")
	}

	r, err := form.ReadField(f, formFieldSampleTypeConfig)
	if err != nil || r == nil {
		return err
	}
	var config map[string]*tree.SampleTypeConfig
	if err = json.Unmarshal(r, &config); err != nil {
		return err
	}
	p.SampleTypeConfig = config
	return nil
}

func (p *RawProfile) metricName(profile *pprof.Profile) string {
	stConfigs := p.getSampleTypes()
	var st string
	for _, ist := range profile.Profile.SampleType {
		st = profile.StringTable[ist.Type]
		if st == "wall" {
			return st
		}
	}
	for _, ist := range profile.Profile.SampleType {
		st = profile.StringTable[ist.Type]
		stConfig := stConfigs[st]

		if stConfig != nil && stConfig.DisplayName != "" {
			st = stConfig.DisplayName
		}
		if strings.Contains(st, "cpu") {
			return "process_cpu"
		}
		if strings.Contains(st, "alloc_") || strings.Contains(st, "inuse_") || st == "space" || st == "objects" {
			return "memory"
		}
		if strings.Contains(st, "mutex_") {
			return "mutex"
		}
		if strings.Contains(st, "block_") {
			return "block"
		}
		if strings.Contains(st, "goroutines") {
			return "goroutines"
		}
	}
	return st // should not happen

}

func (p *RawProfile) createLabels(profile *pprof.Profile, md ingestion.Metadata) []*v1.LabelPair {
	ls := make([]*v1.LabelPair, 0, len(md.Key.Labels())+4)
	ls = append(ls, &v1.LabelPair{
		Name:  labels.MetricName,
		Value: p.metricName(profile),
	}, &v1.LabelPair{
		Name:  phlaremodel.LabelNameDelta,
		Value: "false",
	}, &v1.LabelPair{
		Name:  "service_name",
		Value: md.Key.AppName(),
	}, &v1.LabelPair{
		Name:  phlaremodel.LabelNamePyroscopeSpy,
		Value: md.SpyName,
	})
	for k, v := range md.Key.Labels() {
		if !phlaremodel.IsLabelAllowedForIngestion(k) {
			continue
		}
		ls = append(ls, &v1.LabelPair{
			Name:  k,
			Value: v,
		})
	}
	return ls
}
func (p *RawProfile) getSampleTypes() map[string]*tree.SampleTypeConfig {
	sampleTypes := tree.DefaultSampleTypeMapping
	if p.SampleTypeConfig != nil {
		sampleTypes = p.SampleTypeConfig
	}
	return sampleTypes
}

func needFunctionNameRewrite(md ingestion.Metadata) bool {
	return isScriptingSpy(md)
}

func SpyNameForFunctionNameRewrite() string {
	return "scripting"
}

func isScriptingSpy(md ingestion.Metadata) bool {
	return md.SpyName == "pyspy" || md.SpyName == "rbspy" || md.SpyName == "scripting"
}

func FixFunctionNamesForScriptingLanguages(p *pprof.Profile, md ingestion.Metadata) {
	if !needFunctionNameRewrite(md) {
		return
	}
	smap := map[string]int{}
	for _, fn := range p.Function {
		// obtaining correct line number will require rewriting functions and slices
		// lets not do it and wait until we render line numbers on frontend
		const lineNumber = -1
		name := fmt.Sprintf("%s:%d - %s",
			p.StringTable[fn.Filename],
			lineNumber,
			p.StringTable[fn.Name])
		sid := smap[name]
		if sid == 0 {
			sid = len(p.StringTable)
			p.StringTable = append(p.StringTable, name)
			smap[name] = sid
		}
		fn.Name = int64(sid)
	}
}

func fixSampleTypes(profile *profilev1.Profile) {
	for _, st := range profile.SampleType {
		sts := profile.StringTable[st.Type]
		if strings.Contains(sts, "-") {
			sts = strings.ReplaceAll(sts, "-", "_")
			profile.StringTable[st.Type] = sts
		}
	}
}

func FixFunctionIDForBrokenDotnet(profile *profilev1.Profile) {
	for _, function := range profile.Function {
		if function.Id != 0 {
			return
		}
	}
	if len(profile.Function) != len(profile.Location) {
		return
	}
	for i := range profile.Location {
		profile.Function[i].Id = profile.Location[i].Id
	}
}
