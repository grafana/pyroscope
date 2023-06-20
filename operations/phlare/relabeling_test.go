package phlare

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/grafana/phlare/pkg/agent"
)

func Test_Relabeling(t *testing.T) {
	// This tests the expected behavior of the relabeling rules in the helm chart.
	fdata, err := os.Open("helm/phlare/rendered/single-binary.yaml")
	require.NoError(t, err)
	defer fdata.Close()

	values := map[string]interface{}{}
	configString := ``
	dec := yaml.NewDecoder(fdata)
	for dec.Decode(&values) == nil {
		if values["metadata"].(map[string]interface{})["name"] == "phlare-dev-config" {
			configString = values["data"].(map[string]interface{})["config.yaml"].(string)
			break
		}
	}
	type Config struct {
		ScrapeConfigs []agent.ScrapeConfig `yaml:"scrape_configs"`
	}
	cfg := &Config{}
	require.NoError(t, yaml.Unmarshal([]byte(configString), cfg))

	groups := []*agent.TargetGroup{}
	for _, scrapeCfg := range cfg.ScrapeConfigs {
		scrapeCfg.ScrapeInterval = model.Duration(15 * time.Second)
		scrapeCfg.ScrapeTimeout = model.Duration(30 * time.Second)
		groups = append(groups, agent.NewTargetGroup(context.Background(), "test", scrapeCfg, nil, "test", log.NewLogfmtLogger(os.Stdout)))
	}

	for _, ty := range []string{"memory", "cpu", "block", "mutex", "goroutine"} {
		ty := ty
		for _, tt := range []struct {
			name            string
			in              []model.LabelSet
			expectedActived []labels.Labels
		}{
			{
				"no targets",
				[]model.LabelSet{},
				nil,
			},
			{
				"default port",
				[]model.LabelSet{
					{
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_scrape", ty): "true",
						"__address__":  "foo:1234",
						"service_name": "bar",
					},
				},
				[]labels.Labels{
					{
						labels.Label{Name: "instance", Value: "foo:1234"},
						labels.Label{Name: "job", Value: fmt.Sprintf("kubernetes-pods-%s-default-name", ty)},
						labels.Label{Name: "service_name", Value: "bar"},
					},
				},
			},
			{
				"overrides port",
				[]model.LabelSet{
					{
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_scrape", ty): "true",
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_port", ty):   "4100",

						"__address__":  "foo:1234",
						"service_name": "bar",
					},
				},
				[]labels.Labels{
					{
						labels.Label{Name: "instance", Value: "foo:4100"},
						labels.Label{Name: "job", Value: fmt.Sprintf("kubernetes-pods-%s-default-name", ty)},
						labels.Label{Name: "service_name", Value: "bar"},
					},
				},
			},
			{
				"overrides missing port",
				[]model.LabelSet{
					{
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_scrape", ty): "true",
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_port", ty):   "4100",

						"__address__":  "foo",
						"service_name": "bar",
					},
				},
				[]labels.Labels{
					{
						labels.Label{Name: "instance", Value: "foo:4100"},
						labels.Label{Name: "job", Value: fmt.Sprintf("kubernetes-pods-%s-default-name", ty)},
						labels.Label{Name: "service_name", Value: "bar"},
					},
				},
			},
			{
				"custom port name ",
				[]model.LabelSet{
					{
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_scrape", ty):    "true",
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_port_name", ty): "http2",

						"__meta_kubernetes_pod_container_port_name": "http2",
						"__address__":  "foo:123",
						"service_name": "bar",
					},
					{
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_scrape", ty):    "true",
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_port", ty):      "4200",
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_port_name", ty): "http2",

						"__meta_kubernetes_pod_container_port_name": "http",
						"__address__":  "foo:123",
						"service_name": "bar",
					},
				},
				[]labels.Labels{
					{
						labels.Label{Name: "instance", Value: "foo:123"},
						labels.Label{Name: "job", Value: fmt.Sprintf("kubernetes-pods-%s-custom-name", ty)},
						labels.Label{Name: "service_name", Value: "bar"},
					},
				},
			},
			{
				"custom port name overrides port",
				[]model.LabelSet{
					{
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_scrape", ty):    "true",
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_port", ty):      "4100",
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_port_name", ty): "http2",

						"__meta_kubernetes_pod_container_port_name": "http2",
						"__address__":  "foo:123",
						"service_name": "bar",
					},
					{
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_scrape", ty):    "true",
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_port", ty):      "4200",
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_port_name", ty): "http2",

						"__meta_kubernetes_pod_container_port_name": "http",
						"__address__":  "foo:123",
						"service_name": "bar",
					},
				},
				[]labels.Labels{
					{
						labels.Label{Name: "instance", Value: "foo:4100"},
						labels.Label{Name: "job", Value: fmt.Sprintf("kubernetes-pods-%s-custom-name", ty)},
						labels.Label{Name: "service_name", Value: "bar"},
					},
				},
			},
			{
				"service name from annotation",
				[]model.LabelSet{
					{
						labeNameF("__meta_kubernetes_pod_annotation_profiles_grafana_com_%s_scrape", ty): "true",

						"__meta_kubernetes_pod_annotation_pyroscope_io_service_name": "bar",
						"__address__": "foo:123",
					},
				},
				[]labels.Labels{
					{
						labels.Label{Name: "instance", Value: "foo:123"},
						labels.Label{Name: "job", Value: fmt.Sprintf("kubernetes-pods-%s-default-name", ty)},
						labels.Label{Name: "service_name", Value: "bar"},
					},
				},
			},
		} {
			tt := tt
			t.Run(fmt.Sprintf(tt.name+"_"+ty), func(t *testing.T) {
				t.Parallel()
				var (
					actives []labels.Labels
					dropped []labels.Labels
				)
				for _, tg := range groups {
					a, d, err := tg.TargetsFromGroup(&targetgroup.Group{
						Targets: tt.in,
					})
					require.NoError(t, err)
					for _, ta := range a {
						if len(ta.Target.Labels()) > 0 {
							ty := ty
							if ty == "cpu" {
								ty = "process_cpu"
							}
							require.Equal(t, ty, ta.Target.DiscoveredLabels().Get(model.MetricNameLabel))
							actives = append(actives, ta.Target.Labels())
						}
					}
					for _, t := range d {
						if len(t.Target.Labels()) > 0 {
							dropped = append(dropped, t.Target.Labels())
						}
					}
				}

				require.Equal(t, tt.expectedActived, actives, " mismatched actives dropped %v", dropped)
			})
		}
	}
}

func labeNameF(format string, ty string) model.LabelName {
	return model.LabelName(fmt.Sprintf(format, ty))
}
