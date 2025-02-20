package metrics

import (
	"bytes"
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/model"
)

func Test_Ruler_missconfigured(t *testing.T) {
	_, err := NewStaticRulerFromEnvVars(log.NewNopLogger())
	assert.EqualError(t, err, "failed to unmarshal recording rules: expected {} for map", "Empty env var should result in error at creating ruler")
}

func Test_Ruler_happyPath(t *testing.T) {
	jsonRecordingRules :=
		`{
					"tenant1": [{
            "metric_name": "metric1",
            "matchers": ["{__profile_type__=\"profile_type\", pod=\"foo\"}"],
            "group_by": ["bar"],
            "external_labels": [{"name":"__name__", "value":"metric1"}]
          }],
          "tenant2": [{
            "metric_name": "metric2",
            "matchers": ["{__profile_type__=\"profile_type\"}"],
            "group_by": [],
            "external_labels": [{"name":"__name__", "value":"should be ignored"}]
          },
          {
            "metric_name": "no_profile_type",
            "matchers": ["{}"],
            "group_by": [],
            "external_labels": []
          },
          {
            "metric_name": "Wrong metric name",
            "matchers": ["{__profile_type__=\"profile_type\"}"],
            "group_by": [],
            "external_labels": []
          },
          {
            "metric_name": "wrong_matcher",
            "matchers": ["{foo==\"bar\"}"],
            "group_by": [],
            "external_labels": []
          }]
	      }`
	_ = os.Setenv(envVarRecordingRules, jsonRecordingRules)
	loggerBuffer := &bytes.Buffer{}
	logger := log.NewLogfmtLogger(loggerBuffer)
	ruler, err := NewStaticRulerFromEnvVars(logger)
	assert.NoError(t, err)

	rules := ruler.RecordingRules("tenant1")
	assert.Equal(t, []*model.RecordingRule{{
		Matchers: []*labels.Matcher{{
			Type:  labels.MatchEqual,
			Name:  "__profile_type__",
			Value: "profile_type",
		}, {
			Type:  labels.MatchEqual,
			Name:  "pod",
			Value: "foo",
		}},
		GroupBy: []string{"bar"},
		ExternalLabels: labels.Labels{
			{Name: "__name__", Value: "metric1"},
		},
	}}, rules)

	rules = ruler.RecordingRules("tenant2")
	assert.Equal(t, []*model.RecordingRule{{
		Matchers: []*labels.Matcher{{
			Type:  labels.MatchEqual,
			Name:  "__profile_type__",
			Value: "profile_type",
		}},
		GroupBy: []string{},
		ExternalLabels: labels.Labels{
			{Name: "__name__", Value: "metric2"},
		},
	}}, rules)

	assert.Equal(t,
		"level=error msg=\"failed to parse recording rule\" rule=\"metric_name:\\\"no_profile_type\\\" matchers:\\\"{}\\\"\" err=\"no __profile_type__ matcher present\"\n"+
			"level=error msg=\"failed to parse recording rule\" rule=\"metric_name:\\\"Wrong metric name\\\" matchers:\\\"{__profile_type__=\\\\\\\"profile_type\\\\\\\"}\\\"\" err=\"invalid metric name: Wrong metric name\"\n"+
			"level=error msg=\"failed to parse recording rule\" rule=\"metric_name:\\\"wrong_matcher\\\" matchers:\\\"{foo==\\\\\\\"bar\\\\\\\"}\\\"\" err=\"failed to parse matchers: 1:6: parse error: unexpected \\\"=\\\" in label matching, expected string\"\n",
		loggerBuffer.String(),
	)

}
