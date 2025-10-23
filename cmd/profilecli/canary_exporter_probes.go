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
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/google/go-cmp/cmp"
	gprofile "github.com/google/pprof/profile"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	profilesv1 "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	otlpprofiles "go.opentelemetry.io/proto/otlp/profiles/v1development"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

const (
	profileTypeID             = "deadmans_switch:made_up:profilos:made_up:profilos"
	otlpProfileTypeID         = "otlp_test:otlp_test:count:otlp_test:samples"
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

	// for testing the span selection
	p.StringTable = append(p.StringTable, "profile_id", "00000bac2a5ab0c7")
	p.Sample[1].Label = []*googlev1.Label{{Key: int64(len(p.StringTable) - 2), Str: int64(len(p.StringTable) - 1)}}

	data, err := p.MarshalVT()
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

// generateOTLPProfile creates an OTLP profile with the specified ingestion method label

func (ce *canaryExporter) generateOTLPProfile(now time.Time, ingestionMethod string) *profilesv1.ExportProfilesServiceRequest {
	// Sanitize the ingestion method label value by replacing "/" with "_"
	sanitizedMethod := strings.ReplaceAll(ingestionMethod, "/", "_")

	// Create the profile dictionary with custom profile type similar to pprof probe
	dictionary := &otlpprofiles.ProfilesDictionary{
		StringTable: []string{
			"",                 // 0: empty string
			"otlp_test",        // 1
			"samples",          // 2
			"count",            // 3
			"func1",            // 4
			"func2",            // 5
			"ingestion_method", // 6
			sanitizedMethod,    // 7
		},
		MappingTable: []*otlpprofiles.Mapping{
			{}, // 0: empty mapping (required null entry)
		},
		FunctionTable: []*otlpprofiles.Function{
			{NameStrindex: 0}, // 0: empty
			{NameStrindex: 4}, // 1: func1
			{NameStrindex: 5}, // 2: func2
		},
		LocationTable: []*otlpprofiles.Location{
			{Line: []*otlpprofiles.Line{{FunctionIndex: 1}}}, // 0: func1
			{Line: []*otlpprofiles.Line{{FunctionIndex: 2}}}, // 1: func2
		},
		StackTable: []*otlpprofiles.Stack{
			{LocationIndices: []int32{}},     // 0: empty (required null entry)
			{LocationIndices: []int32{1, 0}}, // 1: func2, func1 stack
			{LocationIndices: []int32{0}},    // 2: func1 stack
		},
	}

	// Create profile with two samples matching the original pprof profile
	profile := &otlpprofiles.Profile{
		TimeUnixNano: uint64(now.UnixNano()),
		DurationNano: 0,
		Period:       1,
		SampleType: &otlpprofiles.ValueType{
			TypeStrindex: 1, // "otlp_test"
			UnitStrindex: 3, // "count"
		},
		PeriodType: &otlpprofiles.ValueType{
			TypeStrindex: 1, // "otlp_test"
			UnitStrindex: 2, // "samples"
		},
		Sample: []*otlpprofiles.Sample{
			{
				// func1>func2 with value 10
				StackIndex: 1, // stack_table[1]
				Values:     []int64{10},
			},
			{
				// func1 with value 20
				StackIndex: 2, // stack_table[2]
				Values:     []int64{20},
			},
		},
	}

	// Create the resource attributes
	resourceAttrs := []*commonv1.KeyValue{
		{
			Key:   "service.name",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: canaryExporterServiceName}},
		},
		{
			Key:   "job",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "canary-exporter"}},
		},
		{
			Key:   "instance",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: ce.hostname}},
		},
		{
			Key:   "ingestion_method",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: sanitizedMethod}},
		},
	}

	// Create the OTLP request
	req := &profilesv1.ExportProfilesServiceRequest{
		Dictionary: dictionary,
		ResourceProfiles: []*otlpprofiles.ResourceProfiles{
			{
				Resource: &resourcev1.Resource{
					Attributes: resourceAttrs,
				},
				ScopeProfiles: []*otlpprofiles.ScopeProfiles{
					{
						Scope: &commonv1.InstrumentationScope{
							Name: "pyroscope-canary-exporter",
						},
						Profiles: []*otlpprofiles.Profile{profile},
					},
				},
			},
		},
	}

	return req
}

/*
	 func (ce *canaryExporter) testIngestOTLPGrpc(ctx context.Context, now time.Time) error {
		// Generate the OTLP profile with the appropriate ingestion method label
		req := ce.generateOTLPProfile(now, "otlp/grpc")

		// Parse URL to extract host and port
		parsedURL, err := url.Parse(ce.params.URL)
		if err != nil {
			return fmt.Errorf("failed to parse URL: %w", err)
		}
		port := parsedURL.Port()
		if port == "" && parsedURL.Scheme == "http" {
			port = "80"
		} else if port == "" && parsedURL.Scheme == "https" {
			port = "443"
		} else {
			port = "4317" // default OTLP gRPC port
		}
		grpcAddr := fmt.Sprintf("%s:%s", parsedURL.Hostname(), port)

		// Create gRPC connection
		conn, err := grpc.NewClient(grpcAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect to gRPC server: %w", err)
		}
		defer conn.Close()

		// Create OTLP profiles service client
		client := profilesv1.NewProfilesServiceClient(conn)

		// Send the profile
		_, err = client.Export(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to export OTLP profile via gRPC: %w", err)
		}

		level.Info(logger).Log("msg", "successfully ingested OTLP profile via gRPC")
		return nil
	}
*/

func (ce *canaryExporter) testIngestOTLPHttpJson(ctx context.Context, now time.Time) error {
	// Generate the OTLP profile with the appropriate ingestion method label
	req := ce.generateOTLPProfile(now, "otlp/http/json")

	// Marshal to JSON
	jsonData, err := protojson.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal OTLP profile to JSON: %w", err)
	}

	// Create HTTP request using the instrumented client
	url := ce.params.URL + "/v1development/profiles"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send the request using the instrumented client (ce.params.client is set by doTrace)
	resp, err := ce.params.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read the body to ensure the transport is fully traced
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	level.Info(logger).Log("msg", "successfully ingested OTLP profile via HTTP/JSON")
	return nil
}

func (ce *canaryExporter) testIngestOTLPHttpProtobuf(ctx context.Context, now time.Time) error {
	// Generate the OTLP profile with the appropriate ingestion method label
	req := ce.generateOTLPProfile(now, "otlp/http/protobuf")

	// Marshal to protobuf
	protoData, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal OTLP profile to protobuf: %w", err)
	}

	// Create HTTP request using the instrumented client
	url := ce.params.URL + "/v1development/profiles"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(protoData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	// Send the request using the instrumented client (ce.params.client is set by doTrace)
	resp, err := ce.params.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read the body to ensure the transport is fully traced
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	level.Info(logger).Log("msg", "successfully ingested OTLP profile via HTTP/Protobuf")
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

func (ce *canaryExporter) testSelectMergeOTLPProfile(ctx context.Context, now time.Time) error {
	// Query specifically for OTLP gRPC ingested profiles using the custom profile type
	//labelSelector := fmt.Sprintf(`{service_name="%s", job="canary-exporter", instance="%s"}`, canaryExporterServiceName, ce.hostname)

	respQuery, err := ce.params.queryClient().SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		Start:         now.UnixMilli(),
		End:           now.Add(5 * time.Second).UnixMilli(),
		LabelSelector: ce.createLabelSelector(),
		ProfileTypeID: otlpProfileTypeID,
	}))
	if err != nil {
		return fmt.Errorf("failed to query OTLP profile: %w", err)
	}

	buf, err := respQuery.Msg.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "failed to marshal protobuf")
	}

	gp, err := gprofile.Parse(bytes.NewReader(buf))
	if err != nil {
		return errors.Wrap(err, "failed to parse profile")
	}

	// Verify the expected stacktraces from the OTLP profile
	expected := map[string]int64{
		"func2>func1": 20, // 10 samples from each of the 2 ingestion methods
		"func1":       40, // 20 samples * 2
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
		return fmt.Errorf("OTLP profile query mismatch (-expected, +actual):\n%s", diff)
	}

	level.Info(logger).Log("msg", "successfully queried OTLP profile via gRPC")
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

	for _, pt := range respQuery.Msg.ProfileTypes {
		if pt.ID == profileTypeID {
			level.Info(logger).Log("msg", "found expected profile type", "id", pt.ID)
			return nil
		}
	}

	return fmt.Errorf("expected profile type %s not found", profileTypeID)
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

	if len(labelSets) < 1 {
		return fmt.Errorf("expected at least 1 label set, got %d", len(labelSets))
	}

	for _, ls := range labelSets {
		labels := model.Labels(ls.Labels)

		serviceName := labels.Get(model.LabelNameServiceName)
		if serviceName == "" {
			return fmt.Errorf("expected service_name label to be set")
		}
		if serviceName != canaryExporterServiceName {
			continue
		}
		profileType := labels.Get(model.LabelNameProfileType)
		if profileType == "" {
			return fmt.Errorf("expected profile_type label to be set")
		}
		if profileType != profileTypeID {
			continue
		}
		return nil // found the expected series
	}

	return fmt.Errorf("expected series with service_name=%s and profile_type=%s not found", canaryExporterServiceName, profileTypeID)
}

func (ce *canaryExporter) testLabelNames(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().LabelNames(ctx, connect.NewRequest(&typesv1.LabelNamesRequest{
		Start: now.UnixMilli(),
		End:   now.Add(5 * time.Second).UnixMilli(),
		// we have to pass this matcher to skip the tenant-wide index in v2 which is not ready until after compaction
		Matchers: []string{fmt.Sprintf(`{service_name="%s"}`, canaryExporterServiceName)},
	}))

	if err != nil {
		return err
	}

	labelNames := respQuery.Msg.Names

	expectedLabelNames := []string{
		model.LabelNameProfileName,
		model.LabelNamePeriodType,
		model.LabelNamePeriodUnit,
		model.LabelNameProfileType,
		model.LabelNameServiceNamePrivate,
		model.LabelNameType,
		model.LabelNameUnit,
		"service.name",
		model.LabelNameServiceName,
	}

	// Use map as set for O(1) lookups
	labelNamesSet := make(map[string]struct{}, len(labelNames))
	for _, label := range labelNames {
		labelNamesSet[label] = struct{}{}
	}

	missingLabels := []string{}
	for _, expectedLabel := range expectedLabelNames {
		if _, exists := labelNamesSet[expectedLabel]; !exists {
			missingLabels = append(missingLabels, expectedLabel)
		}
	}
	if len(missingLabels) > 0 {
		return fmt.Errorf("missing expected labels: %s", missingLabels)
	}

	return nil
}

func (ce *canaryExporter) testLabelValues(ctx context.Context, now time.Time) error {
	respQuery, err := ce.params.queryClient().LabelValues(ctx, connect.NewRequest(&typesv1.LabelValuesRequest{
		Start: now.UnixMilli(),
		End:   now.Add(5 * time.Second).UnixMilli(),
		Name:  model.LabelNameServiceName,
		// we have to pass this matcher to skip the tenant-wide index in v2 which is not ready until after compaction
		Matchers: []string{fmt.Sprintf(`{service_name="%s"}`, canaryExporterServiceName)},
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
		SpanSelector:  []string{"00000bac2a5ab0c7"},
	}))

	if err != nil {
		return err
	}

	flamegraph := respQuery.Msg.Flamegraph

	if flamegraph == nil {
		return fmt.Errorf("expected flamegraph to be set")
	}

	if len(flamegraph.Names) != 2 {
		return fmt.Errorf("expected 2 names in flamegraph, got %d", len(flamegraph.Names))
	}

	if len(flamegraph.Levels) != 2 {
		return fmt.Errorf("expected 2 levels in flamegraph, got %d", len(flamegraph.Levels))
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
