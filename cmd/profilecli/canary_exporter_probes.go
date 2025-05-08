package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/google/go-cmp/cmp"
	gprofile "github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

const (
	profileTypeID             = "deadmans_switch:made_up:profilos:made_up:profilos"
	canaryExporterServiceName = "pyroscope-canary-exporter"
)

func (ce *canaryExporter) testIngestProfile(ctx context.Context, now time.Time) error {
	p := testhelper.NewProfileBuilder(now.UnixNano())
	p.Labels = p.Labels[:0]
	p.CustomProfile("deadmans_switch", "made_up", "profilos", "made_up", "profilos")
	p.WithLabels(
		"service_name", canaryExporterServiceName,
		"job", "canary-exporter",
		"instance", ce.hostname,
	)
	p.UUID = uuid.New()
	p.ForStacktraceString("func1", "func2").AddSamples(10)
	p.ForStacktraceString("func1").AddSamples(20)

	data, err := p.Profile.MarshalVT()
	if err != nil {
		return err
	}

	if _, err := ce.params.pusherClient().Push(ctx, connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{
			{
				Labels: p.Labels,
				Samples: []*pushv1.RawSample{{
					ID:         p.UUID.String(),
					RawProfile: data,
				}},
			},
		},
	})); err != nil {
		return err
	}

	level.Info(logger).Log("msg", "successfully ingested profile", "uuid", p.UUID.String())
	return nil
}

func (ce *canaryExporter) testSelectMergeProfile(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		Start:         now.UnixMilli(),
		End:           now.Add(5 * time.Second).UnixMilli(),
		LabelSelector: ce.createLabelSelector(),
		ProfileTypeID: profileTypeID,
	}))
	if err != nil {
		return err
	}

	buf, err := respQuery.Msg.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "failed to marshal protobuf")
	}

	gp, err := gprofile.Parse(bytes.NewReader(buf))
	if err != nil {
		return errors.Wrap(err, "failed to parse profile")
	}

	expected := map[string]int64{
		"func1>func2": 10,
		"func1":       20,
	}
	actual := make(map[string]int64)

	var sb strings.Builder
	for _, s := range gp.Sample {
		sb.Reset()
		for _, loc := range s.Location {
			if sb.Len() != 0 {
				_, err := sb.WriteRune('>')
				if err != nil {
					return err
				}
			}
			for _, line := range loc.Line {
				_, err := sb.WriteString(line.Function.Name)
				if err != nil {
					return err
				}
			}
		}
		actual[sb.String()] = actual[sb.String()] + s.Value[0]
	}

	if diff := cmp.Diff(expected, actual); diff != "" {
		return fmt.Errorf("query mismatch (-expected, +actual):\n%s", diff)
	}

	return nil
}

func (ce *canaryExporter) testProfileTypes(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().ProfileTypes(ctx, connect.NewRequest(&querierv1.ProfileTypesRequest{
		Start: now.UnixMilli(),
		End:   now.Add(5 * time.Second).UnixMilli(),
	}))
	if err != nil {
		return err
	}

	if len(respQuery.Msg.ProfileTypes) != 1 {
		return fmt.Errorf("expected 1 profile type, got %d", len(respQuery.Msg.ProfileTypes))
	}

	if respQuery.Msg.ProfileTypes[0].ID != profileTypeID {
		return fmt.Errorf("expected profile type to be %s, got %s", profileTypeID, respQuery.Msg.ProfileTypes[0].ID)
	}

	return nil
}

func (ce *canaryExporter) testSeries(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().Series(ctx, connect.NewRequest(&querierv1.SeriesRequest{
		Start:      now.UnixMilli(),
		End:        now.Add(5 * time.Second).UnixMilli(),
		LabelNames: []string{model.LabelNameServiceName, model.LabelNameProfileType},
	}))
	if err != nil {
		return err
	}
	labelSets := respQuery.Msg.LabelsSet

	if len(labelSets) != 1 {
		return fmt.Errorf("expected 1 label set, got %d", len(labelSets))
	}

	labels := model.Labels(labelSets[0].Labels)

	if len(labels) != 2 {
		return fmt.Errorf("expected 2 labels, got %d", len(labels))
	}

	serviceName := labels.Get(model.LabelNameServiceName)
	if serviceName == "" {
		return fmt.Errorf("expected service_name label to be set")
	}
	if serviceName != canaryExporterServiceName {
		return fmt.Errorf("expected service_name label to be %s, got %s", canaryExporterServiceName, serviceName)
	}

	profileType := labels.Get(model.LabelNameProfileType)
	if profileType == "" {
		return fmt.Errorf("expected profile_type label to be set")
	}
	if profileType != profileTypeID {
		return fmt.Errorf("expected profile_type label to be %s, got %s", profileTypeID, profileType)
	}

	return nil
}

func (ce *canaryExporter) testLabelNames(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().LabelNames(ctx, connect.NewRequest(&typesv1.LabelNamesRequest{
		Start: now.UnixMilli(),
		End:   now.Add(5 * time.Second).UnixMilli(),
	}))

	if err != nil {
		return err
	}

	if len(respQuery.Msg.Names) != 10 {
		level.Error(logger).Log("msg", "received an invalid number of labels", "expected", 10, "received", len(respQuery.Msg.Names), "labels", strings.Join(respQuery.Msg.Names, ","))
		return fmt.Errorf("expected 10 label names, got %d", len(respQuery.Msg.Names))
	}

	labelNames := respQuery.Msg.Names
	slices.Sort(labelNames)

	expectedLabelNames := []string{
		model.LabelNameProfileName,
		model.LabelNamePeriodType,
		model.LabelNamePeriodUnit,
		model.LabelNameProfileType,
		model.LabelNameServiceNamePrivate,
		model.LabelNameType,
		model.LabelNameUnit,
		"instance",
		"job",
		model.LabelNameServiceName,
	}

	if !slices.Equal(labelNames, expectedLabelNames) {
		return fmt.Errorf("expected label names to be %s, got %s", expectedLabelNames, labelNames)
	}

	return nil
}

func (ce *canaryExporter) testLabelValues(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().LabelValues(ctx, connect.NewRequest(&typesv1.LabelValuesRequest{
		Start: now.UnixMilli(),
		End:   now.Add(5 * time.Second).UnixMilli(),
		Name:  model.LabelNameServiceName,
	}))

	if err != nil {
		return err
	}

	if len(respQuery.Msg.Names) != 1 {
		return fmt.Errorf("expected 1 label value, got %d", len(respQuery.Msg.Names))
	}

	serviceName := respQuery.Msg.Names[0]

	if serviceName != canaryExporterServiceName {
		return fmt.Errorf("expected service_name label to be %s, got %s", canaryExporterServiceName, serviceName)
	}

	return nil
}

func (ce *canaryExporter) testSelectSeries(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().SelectSeries(ctx, connect.NewRequest(&querierv1.SelectSeriesRequest{
		Start:         now.UnixMilli(),
		End:           now.Add(5 * time.Second).UnixMilli(),
		Step:          1000,
		LabelSelector: ce.createLabelSelector(),
		ProfileTypeID: profileTypeID,
		GroupBy:       []string{model.LabelNameServiceName},
	}))

	if err != nil {
		return err
	}

	if len(respQuery.Msg.Series) != 1 {
		return fmt.Errorf("expected 1 series, got %d", len(respQuery.Msg.Series))
	}

	series := respQuery.Msg.Series[0]

	if len(series.Points) != 1 {
		return fmt.Errorf("expected 2 points, got %d", len(series.Points))
	}

	labels := model.Labels(series.Labels)

	if len(labels) != 1 {
		return fmt.Errorf("expected 1 labels, got %d", len(labels))
	}

	serviceName := labels.Get(model.LabelNameServiceName)
	if serviceName == "" {
		return fmt.Errorf("expected service_name label to be set")
	}
	if serviceName != canaryExporterServiceName {
		return fmt.Errorf("expected service_name label to be %s, got %s", canaryExporterServiceName, serviceName)
	}

	return nil
}

func (ce *canaryExporter) testSelectMergeStacktraces(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		Start:         now.UnixMilli(),
		End:           now.Add(5 * time.Second).UnixMilli(),
		LabelSelector: ce.createLabelSelector(),
		ProfileTypeID: profileTypeID,
	}))

	if err != nil {
		return err
	}

	flamegraph := respQuery.Msg.Flamegraph

	if len(flamegraph.Names) != 3 {
		return fmt.Errorf("expected 3 names in flamegraph, got %d", len(flamegraph.Names))
	}

	if len(flamegraph.Levels) != 3 {
		return fmt.Errorf("expected 3 levels in flamegraph, got %d", len(flamegraph.Levels))
	}

	return nil
}

func (ce *canaryExporter) testSelectMergeSpanProfile(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().SelectMergeSpanProfile(ctx, connect.NewRequest(&querierv1.SelectMergeSpanProfileRequest{
		Start:         now.UnixMilli(),
		End:           now.Add(5 * time.Second).UnixMilli(),
		LabelSelector: ce.createLabelSelector(),
		ProfileTypeID: profileTypeID,
	}))

	if err != nil {
		return err
	}

	flamegraph := respQuery.Msg.Flamegraph

	if flamegraph == nil {
		return fmt.Errorf("expected flamegraph to be set")
	}

	if len(flamegraph.Names) != 1 {
		return fmt.Errorf("expected 1 name in flamegraph, got %d", len(flamegraph.Names))
	}

	if len(flamegraph.Levels) != 1 {
		return fmt.Errorf("expected 1 level in flamegraph, got %d", len(flamegraph.Levels))
	}

	return nil
}

func (ce *canaryExporter) testRender(ctx context.Context, now time.Time) error {
	query := profileTypeID + ce.createLabelSelector()
	startTime := now.UnixMilli()
	endTime := now.Add(5 * time.Second).UnixMilli()

	baseURL, err := url.Parse(ce.params.URL)
	if err != nil {
		return err
	}
	baseURL.Path = "/pyroscope/render"

	params := url.Values{}
	params.Add("query", query)
	params.Add("from", fmt.Sprintf("%d", startTime))
	params.Add("until", fmt.Sprintf("%d", endTime))

	baseURL.RawQuery = params.Encode()
	reqURL := baseURL.String()
	level.Debug(logger).Log("msg", "requesting render", "url", reqURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := ce.params.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var flamebearerProfile flamebearer.FlamebearerProfile
	if err := json.NewDecoder(resp.Body).Decode(&flamebearerProfile); err != nil {
		return err
	}

	if len(flamebearerProfile.Flamebearer.Names) != 3 {
		return fmt.Errorf("expected 3 names in flamegraph, got %d", len(flamebearerProfile.Flamebearer.Names))
	}

	if len(flamebearerProfile.Flamebearer.Levels) != 3 {
		return fmt.Errorf("expected 3 levels in flamegraph, got %d", len(flamebearerProfile.Flamebearer.Levels))
	}

	return nil
}

func (ce *canaryExporter) testRenderDiff(ctx context.Context, now time.Time) error {
	query := profileTypeID + ce.createLabelSelector()
	startTime := now.UnixMilli()
	endTime := now.Add(5 * time.Second).UnixMilli()

	baseURL, err := url.Parse(ce.params.URL)
	if err != nil {
		return err
	}
	baseURL.Path = "/pyroscope/render-diff"

	params := url.Values{}
	params.Add("leftQuery", query)
	params.Add("leftFrom", fmt.Sprintf("%d", startTime))
	params.Add("leftUntil", fmt.Sprintf("%d", endTime))
	params.Add("rightQuery", query)
	params.Add("rightFrom", fmt.Sprintf("%d", startTime))
	params.Add("rightUntil", fmt.Sprintf("%d", endTime))

	baseURL.RawQuery = params.Encode()
	reqURL := baseURL.String()
	level.Debug(logger).Log("msg", "requesting diff", "url", reqURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := ce.params.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var flamebearerProfile flamebearer.FlamebearerProfile
	if err := json.NewDecoder(resp.Body).Decode(&flamebearerProfile); err != nil {
		return err
	}

	if len(flamebearerProfile.Flamebearer.Names) != 3 {
		return fmt.Errorf("expected 3 names in flamegraph, got %d", len(flamebearerProfile.Flamebearer.Names))
	}

	if len(flamebearerProfile.Flamebearer.Levels) != 3 {
		return fmt.Errorf("expected 3 levels in flamegraph, got %d", len(flamebearerProfile.Flamebearer.Levels))
	}

	return nil
}

func (ce *canaryExporter) testGetProfileStats(ctx context.Context, now time.Time) error {
	resp, err := ce.params.queryClient().GetProfileStats(ctx, connect.NewRequest(&typesv1.GetProfileStatsRequest{}))

	if err != nil {
		return err
	}

	if !resp.Msg.DataIngested {
		return fmt.Errorf("expected data to be ingested")
	}

	if resp.Msg.OldestProfileTime == math.MinInt64 {
		return fmt.Errorf("expected oldest profile time to be set")
	}

	if resp.Msg.NewestProfileTime == math.MaxInt64 {
		return fmt.Errorf("expected newest profile time to be set")
	}

	return nil
}

func (ce *canaryExporter) createLabelSelector() string {
	return fmt.Sprintf(`{service_name="%s", job="canary-exporter", instance="%s"}`, canaryExporterServiceName, ce.hostname)
}
