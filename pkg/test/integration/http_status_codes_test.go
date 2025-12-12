package integration

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

type Request struct {
	ProfileType string
}

func TestStatusCodes(t *testing.T) {
	const profileTypeProcessCPU = "process_cpu:cpu:nanoseconds:cpu:nanoseconds"

	type Test struct {
		Name   string
		Method string
		Path   string
		Params url.Values
		Header http.Header
		Body   string
		Want   int
	}

	renderTests := []Test{
		{
			Name:   "valid",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":  []string{"now-15m"},
				"until": []string{"now"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "no_time_range",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "all_zero_time_range",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":  []string{"0"},
				"until": []string{"0"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "from_zero_time_range",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":  []string{"0"},
				"until": []string{"now"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "until_zero_time_range",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query": []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":  []string{"now-15m"},
				"until": []string{"0"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "missing_query",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"from":  []string{"now-15m"},
				"until": []string{"now"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "invalid_query_syntax",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query": []string{"bad_query"},
				"from":  []string{"now-15m"},
				"until": []string{"now"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "query_without_profile_type",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query": []string{`{service_name="test"}`},
				"from":  []string{"now-15m"},
				"until": []string{"now"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "invalid_profile_type_format",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query": []string{`invalid_format{service_name="test"}`},
				"from":  []string{"now-15m"},
				"until": []string{"now"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "format_dot_valid",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":   []string{"now-15m"},
				"until":  []string{"now"},
				"format": []string{"dot"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "format_dot_no_time_range",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"format": []string{"dot"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "format_dot_with_max_nodes",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":    []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":     []string{"now-15m"},
				"until":    []string{"now"},
				"format":   []string{"dot"},
				"maxNodes": []string{"50"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "format_dot_with_max_nodes_hyphen",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":     []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":      []string{"now-15m"},
				"until":     []string{"now"},
				"format":    []string{"dot"},
				"max-nodes": []string{"50"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "with_group_by",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":   []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":    []string{"now-15m"},
				"until":   []string{"now"},
				"groupBy": []string{"service_name"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "with_group_by_multiple",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":   []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":    []string{"now-15m"},
				"until":   []string{"now"},
				"groupBy": []string{"service_name", "region"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "with_aggregation_sum",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":       []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":        []string{"now-15m"},
				"until":       []string{"now"},
				"aggregation": []string{"sum"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "with_aggregation_avg",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":       []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":        []string{"now-15m"},
				"until":       []string{"now"},
				"aggregation": []string{"avg"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "with_aggregation_invalid",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":       []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":        []string{"now-15m"},
				"until":       []string{"now"},
				"aggregation": []string{"invalid"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "with_max_nodes",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":    []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":     []string{"now-15m"},
				"until":    []string{"now"},
				"maxNodes": []string{"1024"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "with_max_nodes_hyphen",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":     []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":      []string{"now-15m"},
				"until":     []string{"now"},
				"max-nodes": []string{"1024"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "with_max_nodes_zero",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":    []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":     []string{"now-15m"},
				"until":    []string{"now"},
				"maxNodes": []string{"0"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "with_max_nodes_invalid",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":    []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":     []string{"now-15m"},
				"until":    []string{"now"},
				"maxNodes": []string{"invalid"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "all_params_combined",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":       []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":        []string{"now-15m"},
				"until":       []string{"now"},
				"groupBy":     []string{"service_name"},
				"aggregation": []string{"sum"},
				"maxNodes":    []string{"512"},
			},
			Want: http.StatusOK,
		},
		{
			Name:   "empty_query_param",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query": []string{""},
				"from":  []string{"now-15m"},
				"until": []string{"now"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:   "from_and_until_0_with_dot_format",
			Method: http.MethodGet,
			Path:   "/pyroscope/render",
			Params: url.Values{
				"query":  []string{createRenderQuery(profileTypeProcessCPU, "pyroscope")},
				"from":   []string{"0"},
				"until":  []string{"0"},
				"format": []string{"dot"},
			},
			Want: http.StatusBadRequest,
		},
	}

	renderDiffTests := []Test{}

	allTests := map[string][]Test{
		"/render":     renderTests,
		"/renderDiff": renderDiffTests,
		// TODO add other public endpoints
	}

	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {
		client := http.DefaultClient

		for endpoint, tests := range allTests {
			for _, tt := range tests {
				t.Run(fmt.Sprintf("%s/%s", endpoint, tt.Name), func(t *testing.T) {
					t.Parallel()

					path, err := url.JoinPath(p.URL(), tt.Path)
					require.NoError(t, err, "failed to create path")

					u, err := url.Parse(path)
					require.NoError(t, err, "failed to parse url")
					u.RawQuery = tt.Params.Encode()

					var reqBody io.Reader
					if tt.Body != "" {
						reqBody = bytes.NewReader([]byte(tt.Body))
					}

					req, err := http.NewRequest(tt.Method, u.String(), reqBody)
					require.NoError(t, err)

					for key, vals := range tt.Header {
						for _, val := range vals {
							req.Header.Add(key, val)
						}
					}

					res, err := client.Do(req)
					require.NoError(t, err)
					require.Equal(t, tt.Want, res.StatusCode)
				})
			}
		}
	})
}
