package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/model/labels"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
)

type RecordingRule struct {
	profileType string
	metricName  string
	matchers    []*labels.Matcher
	keepLabels  []string
}

func recordingRulesFromTenant(tenantId string) []*RecordingRule {
	// TODO err handling in general
	ctx := tenant.InjectTenantID(context.Background(), "1218") // TODO, tenantId here is empty. This is normal at compaction level 0 as we took this tenant from the block (L0 blocs mix data of different tenants), while we should take it from the dataset.
	client := SettingsClient()
	response, err := client.Get(ctx, connect.NewRequest(
		&settingsv1.GetSettingsRequest{}))

	if err != nil {
		level.Error(log.NewLogfmtLogger(os.Stderr)).Log("msg", "failed to get settings", "err", err)
		return nil // this will probably cause a panic in the caller
	}
	rules := []*RecordingRule{}
	for _, rule := range response.Msg.Settings {
		if !strings.HasPrefix(rule.Name, "metric.") {
			continue
		}
		rules = append(rules, parseRule(rule))
	}
	for _, rule := range rules {
		level.Debug(log.NewLogfmtLogger(os.Stderr)).Log("rule", rule.metricName)
	}
	return rules
}

func recordingRulesFromTenantStatic(tenant2 string) []*RecordingRule {
	return []*RecordingRule{
		{
			profileType: "process_cpu:samples:count:cpu:nanoseconds",
			metricName:  "ride_sharing_app_car_cpu_nanoseconds",
			matchers: []*labels.Matcher{
				{
					Type:  labels.MatchEqual,
					Name:  "service_name",
					Value: "ride-sharing-app",
				},
				{
					Type:  labels.MatchEqual,
					Name:  "vehicle",
					Value: "car",
				},
			},
			keepLabels: []string{"region"},
		},
		{
			profileType: "process_cpu:samples:count:cpu:nanoseconds",
			metricName:  "ride_sharing_app_car_all_regions_cpu_nanoseconds",
			matchers: []*labels.Matcher{
				{
					Type:  labels.MatchEqual,
					Name:  "service_name",
					Value: "ride-sharing-app",
				},
				{
					Type:  labels.MatchEqual,
					Name:  "vehicle",
					Value: "car",
				},
			},
			keepLabels: []string{},
		},
		{
			profileType: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
			metricName:  "pyroscope_exported_metrics_mimir_dev_10_ingester",
			matchers: []*labels.Matcher{
				{
					Type:  labels.MatchEqual,
					Name:  "service_name",
					Value: "mimir-dev-10/ingester",
				},
			},
			keepLabels: []string{"controller_revision_hash"},
		},
		{
			profileType: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
			metricName:  "pyroscope_exported_metrics_mimir_dev_10_ingester_with_span_names",
			matchers: []*labels.Matcher{
				{
					Type:  labels.MatchEqual,
					Name:  "service_name",
					Value: "mimir-dev-10/ingester",
				},
			},
			keepLabels: []string{"controller_revision_hash", "span_name"},
		},
	}
}

func SettingsClient() settingsv1connect.SettingsServiceClient {
	// TODO: refactor this. Very rudimentary, only intended for experimental
	httpClient := util.InstrumentedDefaultHTTPClient()
	opts := connectapi.DefaultClientOptions()
	opts = append(opts, connect.WithInterceptors(tenant.NewAuthInterceptor(true)))
	return settingsv1connect.NewSettingsServiceClient(
		httpClient,
		"http://fire-tenant-settings-headless.fire-dev-001.svc.cluster.local:4100", // TODO: get it from config (set it in deployment tools) use "http://localhost:4040" for local
		opts...,
	)
}

func parseRule(rule *settingsv1.Setting) *RecordingRule {
	var parsed RecordingRuleSetting
	err := json.Unmarshal([]byte(rule.Value), &parsed)
	if err != nil {
		// TODO
		fmt.Println("Error parsing JSON:", err)
	}

	return &RecordingRule{
		profileType: parseProfileType(parsed.ProfileType),
		metricName:  "pyroscope_exported_metrics_" + parsed.Name, // TODO sanitize
		matchers:    parseMatchers(parsed.Matcher, parsed.ServiceName),
		keepLabels:  parsed.Labels, // [] != All
	}
}

func parseProfileType(profileType string) string {
	//TODO
	switch profileType {
	case "cpu":
		return "process_cpu:cpu:nanoseconds:cpu:nanoseconds"
	}
	return "process_cpu:cpu:nanoseconds:cpu:nanoseconds"
}

func parseMatchers(matchersString string, serviceName string) []*labels.Matcher {
	matchers := []*labels.Matcher{
		{Type: labels.MatchEqual, Name: "service_name", Value: serviceName}, // TODO __service_name__ or service_name?
	}
	// TODO: can't believe there's not a reversed labels.Matcher map.
	matcherRegex := regexp.MustCompile(`([a-zA-Z0-9_]+)(=|!=|=~|!~)"([^"]+)"`) // TODO: let's store this statically so we don't need to compile every time
	matches := matcherRegex.FindAllStringSubmatch(matchersString, -1)

	if matches == nil {
		return matchers
	}

	for _, match := range matches {
		matcher, _ := labels.NewMatcher(parseType(match[2]), match[1], match[3]) // TODO err
		matchers = append(matchers, matcher)
	}

	return matchers
}

func parseType(t string) labels.MatchType {
	switch t {
	case "=":
		return labels.MatchEqual
	case "!=":
		return labels.MatchNotEqual
	case "=~":
		return labels.MatchRegexp
	case "!~":
		return labels.MatchNotRegexp
	}
	return labels.MatchEqual // TODO
}

type RecordingRuleSetting struct {
	Version              int      `json:"version"`
	Name                 string   `json:"name"`
	ServiceName          string   `json:"serviceName"`
	ProfileType          string   `json:"profileType"`
	Matcher              string   `json:"matcher"`
	PrometheusDataSource string   `json:"prometheusDataSource"`
	Labels               []string `json:"labels"`
}
