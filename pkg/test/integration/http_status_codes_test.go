package integration

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

type Request struct {
	ProfileType string
}

func TestStatusCodes(t *testing.T) {
	tests := []struct {
		Name        string
		ProfileType string
		Params      url.Values
		Want        int
	}{
		{
			Name:        "valid",
			ProfileType: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
			Params: url.Values{
				"from":  []string{"now-15m"},
				"until": []string{"now"},
			},
			Want: http.StatusOK,
		},
		{
			Name:        "no_time_range",
			ProfileType: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
			Params:      url.Values{},
			Want:        http.StatusOK,
		},
		{
			Name:        "missing_query",
			ProfileType: "",
			Params: url.Values{
				"from":  []string{"now-15m"},
				"until": []string{"now"},
			},
			Want: http.StatusBadRequest,
		},
		{
			Name:        "invalid_query",
			ProfileType: "",
			Params: url.Values{
				"query": []string{"bad_query"},
				"from":  []string{"now-15m"},
				"until": []string{"now"},
			},
			Want: http.StatusBadRequest,
		},
	}

	EachPyroscopeTest(t, func(p *PyroscopeTest, t *testing.T) {
		for _, tt := range tests {
			t.Run(tt.Name, func(t *testing.T) {
				if tt.ProfileType != "" {
					tt.Params.Set("query", createRenderQuery(tt.ProfileType, "pyroscope"))
				}

				path, err := url.JoinPath(p.URL(), "pyroscope", "render")
				require.NoError(t, err, "failed to create base path")

				u, err := url.Parse(path)
				require.NoError(t, err, "failed to parse url")
				u.RawQuery = tt.Params.Encode()

				res, err := http.Get(u.String())
				require.NoError(t, err)
				require.Equal(t, tt.Want, res.StatusCode)
			})
		}
	})
}
