package agent

import (
	"testing"
	"time"

	"github.com/parca-dev/parca/pkg/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/phlare/pkg/model"
)

func TestPopulateLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   labels.Labels
		cfg      ScrapeConfig
		wantRes  labels.Labels
		wantOrig labels.Labels
		wantErr  bool
	}{
		{
			name: "regular case",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4:1000",
				phlaremodel.LabelNameServiceName: "svc",
				"custom":                         "value",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4:1000",
				phlaremodel.LabelNameServiceName: "svc",
				model.InstanceLabel:              "1.2.3.4:1000",
				model.SchemeLabel:                "https",
				model.JobLabel:                   "job",
				model.ScrapeIntervalLabel:        "1s",
				model.ScrapeTimeoutLabel:         "2s",
				"custom":                         "value",
			}),
			wantOrig: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4:1000",
				phlaremodel.LabelNameServiceName: "svc",
				model.SchemeLabel:                "https",
				model.JobLabel:                   "job",
				model.ScrapeIntervalLabel:        "1s",
				model.ScrapeTimeoutLabel:         "2s",
				"custom":                         "value",
			}),
			wantErr: false,
		},
		{
			name: "overwrite scrape config",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4",
				phlaremodel.LabelNameServiceName: "svc",
				model.SchemeLabel:                "http",
				model.JobLabel:                   "custom-job",
				model.ScrapeIntervalLabel:        "2s",
				model.ScrapeTimeoutLabel:         "3s",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4:80",
				phlaremodel.LabelNameServiceName: "svc",
				model.InstanceLabel:              "1.2.3.4:80",
				model.SchemeLabel:                "http",
				model.JobLabel:                   "custom-job",
				model.ScrapeIntervalLabel:        "2s",
				model.ScrapeTimeoutLabel:         "3s",
			}),
			wantOrig: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4",
				phlaremodel.LabelNameServiceName: "svc",
				model.SchemeLabel:                "http",
				model.JobLabel:                   "custom-job",
				model.ScrapeIntervalLabel:        "2s",
				model.ScrapeTimeoutLabel:         "3s",
			}),
			wantErr: false,
		},
		{
			name: "ipv6 instance label",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:               "[::1]",
				phlaremodel.LabelNameServiceName: "svc",
				model.InstanceLabel:              "custom-instance",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes: labels.FromMap(map[string]string{
				model.AddressLabel:               "[::1]:443",
				phlaremodel.LabelNameServiceName: "svc",
				model.InstanceLabel:              "custom-instance",
				model.SchemeLabel:                "https",
				model.JobLabel:                   "job",
				model.ScrapeIntervalLabel:        "1s",
				model.ScrapeTimeoutLabel:         "2s",
			}),
			wantOrig: labels.FromMap(map[string]string{
				model.AddressLabel:               "[::1]",
				phlaremodel.LabelNameServiceName: "svc",
				model.InstanceLabel:              "custom-instance",
				model.SchemeLabel:                "https",
				model.JobLabel:                   "job",
				model.ScrapeIntervalLabel:        "1s",
				model.ScrapeTimeoutLabel:         "2s",
			}),
			wantErr: false,
		},
		{
			name:   "address label missing",
			labels: labels.FromStrings("custom", "value"),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes:  nil,
			wantOrig: nil,
			wantErr:  true,
		},
		{
			name: "address label missing but relabelling",
			labels: labels.FromMap(map[string]string{
				"custom":                            "host:1234",
				phlaremodel.LabelNameServiceNameK8s: "svc",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
				RelabelConfigs: []*relabel.Config{
					{
						Action:       relabel.Replace,
						Regex:        relabel.MustNewRegexp("(.*)"),
						SourceLabels: model.LabelNames{"custom"},
						Replacement:  "${1}",
						TargetLabel:  model.AddressLabel,
					},
				},
			},
			wantRes: labels.FromMap(map[string]string{
				model.AddressLabel:               "host:1234",
				phlaremodel.LabelNameServiceName: "svc",
				model.InstanceLabel:              "host:1234",
				model.SchemeLabel:                "https",
				model.JobLabel:                   "job",
				model.ScrapeIntervalLabel:        "1s",
				model.ScrapeTimeoutLabel:         "2s",
				"custom":                         "host:1234",
			}),
			wantOrig: labels.FromMap(map[string]string{
				model.AddressLabel:                  "host:1234",
				phlaremodel.LabelNameServiceNameK8s: "svc",
				model.SchemeLabel:                   "https",
				model.JobLabel:                      "job",
				model.ScrapeIntervalLabel:           "1s",
				model.ScrapeTimeoutLabel:            "2s",
				"custom":                            "host:1234",
			}),
		},
		{
			name: "invalid UTF-8 label",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "\xbd",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes:  nil,
			wantOrig: nil,
			wantErr:  true,
		},
		{
			name: "invalid interval duration label",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "2notseconds",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes:  nil,
			wantOrig: nil,
			wantErr:  true,
		},
		{
			name: "invalid timeout duration label",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:       "1.2.3.4:1000",
				model.ScrapeTimeoutLabel: "2notseconds",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes:  nil,
			wantOrig: nil,
			wantErr:  true,
		},
		{
			name: "zero duration interval label",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "0s",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			wantRes:  nil,
			wantOrig: nil,
			wantErr:  true,
		},
		{
			name: "zero duration timeout label",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:       "1.2.3.4:1000",
				model.ScrapeTimeoutLabel: "0s",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes:  nil,
			wantOrig: nil,
			wantErr:  true,
		},
		{
			name: "timeout less than interval",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "2s",
				model.ScrapeTimeoutLabel:  "1s",
			}),
			cfg: ScrapeConfig{
				Scheme:         "https",
				JobName:        "job",
				ScrapeInterval: 2 * model.Duration(time.Second),
				ScrapeTimeout:  model.Duration(time.Second),
			},
			wantRes:  nil,
			wantOrig: nil,
			wantErr:  true,
		},
		{
			name: "service name unspecified",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
			}),
			cfg: ScrapeConfig{
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4:1000",
				phlaremodel.LabelNameServiceName: "unspecified",
				model.InstanceLabel:              "1.2.3.4:1000",
				model.ScrapeIntervalLabel:        "1s",
				model.ScrapeTimeoutLabel:         "2s",
			}),
			wantOrig: labels.FromMap(map[string]string{
				model.AddressLabel:        "1.2.3.4:1000",
				model.ScrapeIntervalLabel: "1s",
				model.ScrapeTimeoutLabel:  "2s",
			}),
			wantErr: false,
		},
		{
			name: "service name inferred from pod annotation",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
				"__meta_kubernetes_pod_annotation_pyroscope_io_service_name": "k8s-svc",
			}),
			cfg: ScrapeConfig{
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4:1000",
				phlaremodel.LabelNameServiceName: "k8s-svc",
				model.InstanceLabel:              "1.2.3.4:1000",
				model.ScrapeIntervalLabel:        "1s",
				model.ScrapeTimeoutLabel:         "2s",
			}),
			wantOrig: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
				"__meta_kubernetes_pod_annotation_pyroscope_io_service_name": "k8s-svc",
				model.ScrapeIntervalLabel:                                    "1s",
				model.ScrapeTimeoutLabel:                                     "2s",
			}),
			wantErr: false,
		},
		{
			name: "service name inferred from k8s",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:                     "1.2.3.4:1000",
				"__meta_kubernetes_namespace":          "ns",
				"__meta_kubernetes_pod_container_name": "container",
			}),
			cfg: ScrapeConfig{
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4:1000",
				phlaremodel.LabelNameServiceName: "ns/container",
				model.InstanceLabel:              "1.2.3.4:1000",
				model.ScrapeIntervalLabel:        "1s",
				model.ScrapeTimeoutLabel:         "2s",
			}),
			wantOrig: labels.FromMap(map[string]string{
				model.AddressLabel:                     "1.2.3.4:1000",
				"__meta_kubernetes_namespace":          "ns",
				"__meta_kubernetes_pod_container_name": "container",
				model.ScrapeIntervalLabel:              "1s",
				model.ScrapeTimeoutLabel:               "2s",
			}),
			wantErr: false,
		},
		{
			name: "service name inferred from docker",
			labels: labels.FromMap(map[string]string{
				model.AddressLabel:             "1.2.3.4:1000",
				"__meta_docker_container_name": "container",
			}),
			cfg: ScrapeConfig{
				ScrapeInterval: model.Duration(time.Second),
				ScrapeTimeout:  2 * model.Duration(time.Second),
			},
			wantRes: labels.FromMap(map[string]string{
				model.AddressLabel:               "1.2.3.4:1000",
				phlaremodel.LabelNameServiceName: "container",
				model.InstanceLabel:              "1.2.3.4:1000",
				model.ScrapeIntervalLabel:        "1s",
				model.ScrapeTimeoutLabel:         "2s",
			}),
			wantOrig: labels.FromMap(map[string]string{
				model.AddressLabel:             "1.2.3.4:1000",
				"__meta_docker_container_name": "container",
				model.ScrapeIntervalLabel:      "1s",
				model.ScrapeTimeoutLabel:       "2s",
			}),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, gotOrig, err := populateLabels(tt.labels, tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantRes, gotRes)
			require.Equal(t, tt.wantOrig, gotOrig)
		})
	}
}

func TestTargetGroup_targetsFromGroup(t *testing.T) {
	tests := []struct {
		name        string
		tg          *TargetGroup
		group       *targetgroup.Group
		wantTargets int
		wantDropped int
		wantErr     bool
	}{
		{
			name: "regular case",
			tg: &TargetGroup{
				jobName: "job",
				config: ScrapeConfig{
					ScrapeTimeout:  model.Duration(10 * time.Minute),
					ScrapeInterval: model.Duration(time.Minute),
					ProfilingConfig: &config.ProfilingConfig{
						PprofConfig: config.PprofConfig{
							pprofProcessCPU: &config.PprofProfilingConfig{
								Enabled: trueValue(),
							},
						},
					},
				},
			},
			group: &targetgroup.Group{
				Targets: []model.LabelSet{
					{
						model.AddressLabel:               "localhost:9000",
						phlaremodel.LabelNameServiceName: "svc",
					},
				},
			},
			wantTargets: 1,
			wantDropped: 0,
			wantErr:     false,
		},
		{
			name: "overwrite timeout and interval",
			tg: &TargetGroup{
				jobName: "job",
				config: ScrapeConfig{
					ScrapeTimeout:  model.Duration(10 * time.Second),
					ScrapeInterval: model.Duration(time.Minute),
					ProfilingConfig: &config.ProfilingConfig{
						PprofConfig: config.PprofConfig{
							pprofProcessCPU: &config.PprofProfilingConfig{
								Enabled: trueValue(),
							},
						},
					},
				},
			},
			group: &targetgroup.Group{
				Targets: []model.LabelSet{
					{
						model.AddressLabel:               "localhost:9000",
						phlaremodel.LabelNameServiceName: "svc",
					},
				},
				Labels: model.LabelSet{
					model.ScrapeIntervalLabel: "30s",
					model.ScrapeTimeoutLabel:  "40s",
				},
			},
			wantTargets: 1,
			wantDropped: 0,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTargets, gotDropped, err := tt.tg.TargetsFromGroup(tt.group)
			if tt.wantErr {
				require.NoError(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, gotTargets, tt.wantTargets)
			require.Len(t, gotDropped, tt.wantDropped)
		})
	}
}

func trueValue() *bool {
	b := true
	return &b
}
