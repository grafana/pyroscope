// Provenance-includes-location: https://github.com/prometheus/prometheus/blob/v2.51.2/model/relabel/relabel_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Prometheus Authors

package relabel

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func TestRelabel(t *testing.T) {
	tests := []struct {
		input   phlaremodel.Labels
		relabel []*relabel.Config
		output  phlaremodel.Labels
		drop    bool
	}{
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch${1}-ch${1}",
					Action:       relabel.Replace,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "choo-choo",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a", "b"},
					Regex:        relabel.MustNewRegexp("f(.*);(.*)r"),
					TargetLabel:  "a",
					Separator:    ";",
					Replacement:  "b${1}${2}m", // boobam
					Action:       relabel.Replace,
				},
				{
					SourceLabels: model.LabelNames{"c", "a"},
					Regex:        relabel.MustNewRegexp("(b).*b(.*)ba(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1$2$2$3",
					Action:       relabel.Replace,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "boobam",
				"b": "bar",
				"c": "baz",
				"d": "boooom",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp(".*o.*"),
					Action:       relabel.Drop,
				}, {
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch$1-ch$1",
					Action:       relabel.Replace,
				},
			},
			drop: true,
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
				"b": "bar",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp(".*o.*"),
					Action:       relabel.Drop,
				},
			},
			drop: true,
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "abc",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp(".*(b).*"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1",
					Action:       relabel.Replace,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "abc",
				"d": "b",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("no-match"),
					Action:       relabel.Drop,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("f|o"),
					Action:       relabel.Drop,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("no-match"),
					Action:       relabel.Keep,
				},
			},
			drop: true,
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("f.*"),
					Action:       relabel.Keep,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			// No replacement must be applied if there is no match.
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "boo",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("f"),
					TargetLabel:  "b",
					Replacement:  "bar",
					Action:       relabel.Replace,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "boo",
			}),
		},
		{
			// Blank replacement should delete the label.
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
				"f": "baz",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("(f).*"),
					TargetLabel:  "$1",
					Replacement:  "$2",
					Action:       relabel.Replace,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"c"},
					TargetLabel:  "d",
					Separator:    ";",
					Action:       relabel.HashMod,
					Modulus:      1000,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "976",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo\nbar",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					TargetLabel:  "b",
					Separator:    ";",
					Action:       relabel.HashMod,
					Modulus:      1000,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo\nbar",
				"b": "734",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			}),
			relabel: []*relabel.Config{
				{
					Regex:       relabel.MustNewRegexp("(b.*)"),
					Replacement: "bar_${1}",
					Action:      relabel.LabelMap,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a":      "foo",
				"b1":     "bar",
				"b2":     "baz",
				"bar_b1": "bar",
				"bar_b2": "baz",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a":             "foo",
				"__meta_my_bar": "aaa",
				"__meta_my_baz": "bbb",
				"__meta_other":  "ccc",
			}),
			relabel: []*relabel.Config{
				{
					Regex:       relabel.MustNewRegexp("__meta_(my.*)"),
					Replacement: "${1}",
					Action:      relabel.LabelMap,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a":             "foo",
				"__meta_my_bar": "aaa",
				"__meta_my_baz": "bbb",
				"__meta_other":  "ccc",
				"my_bar":        "aaa",
				"my_baz":        "bbb",
			}),
		},
		{ // valid case
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "some-name-value",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       relabel.Replace,
					Replacement:  "${2}",
					TargetLabel:  "${1}",
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a":    "some-name-value",
				"name": "value",
			}),
		},
		{ // invalid replacement ""
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "some-name-value",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       relabel.Replace,
					Replacement:  "${3}",
					TargetLabel:  "${1}",
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "some-name-value",
			}),
		},
		{ // invalid target_labels
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a": "some-name-0",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       relabel.Replace,
					Replacement:  "${1}",
					TargetLabel:  "${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       relabel.Replace,
					Replacement:  "${1}",
					TargetLabel:  "${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        relabel.MustNewRegexp("some-([^-]+)(-[^,]+)"),
					Action:       relabel.Replace,
					Replacement:  "${1}",
					TargetLabel:  "${3}",
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "some-name-0",
			}),
		},
		{ // more complex real-life like usecase
			input: phlaremodel.LabelsFromMap(map[string]string{
				"__meta_sd_tags": "path:/secret,job:some-job,label:foo=bar",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        relabel.MustNewRegexp("(?:.+,|^)path:(/[^,]+).*"),
					Action:       relabel.Replace,
					Replacement:  "${1}",
					TargetLabel:  "__metrics_path__",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        relabel.MustNewRegexp("(?:.+,|^)job:([^,]+).*"),
					Action:       relabel.Replace,
					Replacement:  "${1}",
					TargetLabel:  "job",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        relabel.MustNewRegexp("(?:.+,|^)label:([^=]+)=([^,]+).*"),
					Action:       relabel.Replace,
					Replacement:  "${2}",
					TargetLabel:  "${1}",
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"__meta_sd_tags":   "path:/secret,job:some-job,label:foo=bar",
				"__metrics_path__": "/secret",
				"job":              "some-job",
				"foo":              "bar",
			}),
		},
		{ // From https://github.com/prometheus/prometheus/issues/12283
			input: phlaremodel.LabelsFromMap(map[string]string{
				"__meta_kubernetes_pod_container_port_name":         "foo",
				"__meta_kubernetes_pod_annotation_XXX_metrics_port": "9091",
			}),
			relabel: []*relabel.Config{
				{
					Regex:  relabel.MustNewRegexp("^__meta_kubernetes_pod_container_port_name$"),
					Action: relabel.LabelDrop,
				},
				{
					SourceLabels: model.LabelNames{"__meta_kubernetes_pod_annotation_XXX_metrics_port"},
					Regex:        relabel.MustNewRegexp("(.+)"),
					Action:       relabel.Replace,
					Replacement:  "metrics",
					TargetLabel:  "__meta_kubernetes_pod_container_port_name",
				},
				{
					SourceLabels: model.LabelNames{"__meta_kubernetes_pod_container_port_name"},
					Regex:        relabel.MustNewRegexp("^metrics$"),
					Action:       relabel.Keep,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"__meta_kubernetes_pod_annotation_XXX_metrics_port": "9091",
				"__meta_kubernetes_pod_container_port_name":         "metrics",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			}),
			relabel: []*relabel.Config{
				{
					Regex:  relabel.MustNewRegexp("(b.*)"),
					Action: relabel.LabelKeep,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"b1": "bar",
				"b2": "baz",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			}),
			relabel: []*relabel.Config{
				{
					Regex:  relabel.MustNewRegexp("(b.*)"),
					Action: relabel.LabelDrop,
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"foo": "bAr123Foo",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"foo"},
					Action:       relabel.Uppercase,
					TargetLabel:  "foo_uppercase",
				},
				{
					SourceLabels: model.LabelNames{"foo"},
					Action:       relabel.Lowercase,
					TargetLabel:  "foo_lowercase",
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"foo":           "bAr123Foo",
				"foo_lowercase": "bar123foo",
				"foo_uppercase": "BAR123FOO",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       relabel.KeepEqual,
					TargetLabel:  "__port1",
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       relabel.DropEqual,
					TargetLabel:  "__port1",
				},
			},
			drop: true,
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       relabel.DropEqual,
					TargetLabel:  "__port2",
				},
			},
			output: phlaremodel.LabelsFromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
		},
		{
			input: phlaremodel.LabelsFromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       relabel.KeepEqual,
					TargetLabel:  "__port2",
				},
			},
			drop: true,
		},
	}

	for _, test := range tests {
		// Setting default fields, mimicking the behaviour in Prometheus.
		for _, cfg := range test.relabel {
			if cfg.Action == "" {
				cfg.Action = relabel.DefaultRelabelConfig.Action
			}
			if cfg.Separator == "" {
				cfg.Separator = relabel.DefaultRelabelConfig.Separator
			}
			if cfg.Regex.Regexp == nil || cfg.Regex.String() == "" {
				cfg.Regex = relabel.DefaultRelabelConfig.Regex
			}
			if cfg.Replacement == "" {
				cfg.Replacement = relabel.DefaultRelabelConfig.Replacement
			}
			require.NoError(t, cfg.Validate())
		}

		res, keep := Process(test.input, test.relabel...)
		require.Equal(t, !test.drop, keep)
		if keep {
			testutil.RequireEqualWithOptions(t, test.output, res, []cmp.Option{cmpopts.IgnoreUnexported(typesv1.LabelPair{})})
		}
	}
}
