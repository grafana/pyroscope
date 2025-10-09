package queryfrontend

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

func Test_matchersToLabelSelector(t *testing.T) {
	tests := []struct {
		name     string
		matchers []*labels.Matcher
		expected string
	}{
		{
			name:     "empty matchers",
			matchers: []*labels.Matcher{},
			expected: "{}",
		},
		{
			name: "single standard label name with equals",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "my-service"),
			},
			expected: `{service_name="my-service"}`,
		},
		{
			name: "label name with dots",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service.name", "test"),
			},
			expected: `{"service.name"="test"}`,
		},
		{
			name: "label name with hyphen",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service-name", "value"),
			},
			expected: `{"service-name"="value"}`,
		},
		{
			name: "label name with special character (√)",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "my√label", "value"),
			},
			expected: `{"my√label"="value"}`,
		},
		{
			name: "label name with unicode (世界)",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "世界", "value"),
			},
			expected: `{"世界"="value"}`,
		},
		{
			name: "label name starting with number",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "123label", "value"),
			},
			expected: `{"123label"="value"}`,
		},
		{
			name: "not equal matcher",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotEqual, "service_name", "test"),
			},
			expected: `{service_name!="test"}`,
		},
		{
			name: "regex match matcher",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, "service_name", ".*test.*"),
			},
			expected: `{service_name=~".*test.*"}`,
		},
		{
			name: "regex not match matcher",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchNotRegexp, "service_name", "test.*"),
			},
			expected: `{service_name!~"test.*"}`,
		},
		{
			name: "multiple standard matchers",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "my-service"),
				labels.MustNewMatcher(labels.MatchEqual, "environment", "production"),
				labels.MustNewMatcher(labels.MatchEqual, "region", "us-west-2"),
			},
			expected: `{service_name="my-service",environment="production",region="us-west-2"}`,
		},
		{
			name: "multiple matchers with special characters",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service.name", "my-service"),
				labels.MustNewMatcher(labels.MatchEqual, "app-version", "v1.0"),
				labels.MustNewMatcher(labels.MatchEqual, "my√label", "value"),
			},
			expected: `{"service.name"="my-service","app-version"="v1.0","my√label"="value"}`,
		},
		{
			name: "mixed matcher types",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "test"),
				labels.MustNewMatcher(labels.MatchNotEqual, "environment", "dev"),
				labels.MustNewMatcher(labels.MatchRegexp, "region", "us-.*"),
			},
			expected: `{service_name="test",environment!="dev",region=~"us-.*"}`,
		},
		{
			name: "value with quotes needs escaping",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "label", `value"with"quotes`),
			},
			expected: `{label="value\"with\"quotes"}`,
		},
		{
			name: "label name and value with special characters",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "label-name.special", "value/with\\special"),
			},
			expected: `{"label-name.special"="value/with\\special"}`,
		},
		{
			name: "standard alphanumeric labels don't get quoted",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"),
				labels.MustNewMatcher(labels.MatchEqual, "foo123", "bar456"),
				labels.MustNewMatcher(labels.MatchEqual, "__internal__", "value"),
			},
			expected: `{foo="bar",foo123="bar456",__internal__="value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchersToLabelSelector(tt.matchers)
			require.Equal(t, tt.expected, result, "matchersToLabelSelector output mismatch")
		})
	}
}

func Test_buildLabelSelectorFromMatchers(t *testing.T) {
	tests := []struct {
		name     string
		matchers []string
		expected string
		wantErr  bool
	}{
		{
			name:     "single matcher",
			matchers: []string{`{service_name="test"}`},
			expected: `{service_name="test"}`,
			wantErr:  false,
		},
		{
			name:     "matcher with special characters in label name",
			matchers: []string{`{"service.name"="test"}`},
			expected: `{"service.name"="test"}`,
			wantErr:  false,
		},
		{
			name:     "multiple matchers",
			matchers: []string{`{service_name="test"}`, `{environment="prod"}`},
			expected: `{service_name="test",environment="prod"}`,
			wantErr:  false,
		},
		{
			name:     "invalid matcher syntax",
			matchers: []string{`{unclosed`},
			expected: "",
			wantErr:  true,
		},
		{
			name:     "empty matchers",
			matchers: []string{},
			expected: `{}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildLabelSelectorFromMatchers(tt.matchers)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}
