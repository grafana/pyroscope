package client

import (
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/util"
)

var (
	githubRouteMatchers = map[string]*regexp.Regexp{
		// Get repository contents.
		// https://docs.github.com/en/rest/repos/contents?apiVersion=2022-11-28#get-repository-content
		"/repos/{owner}/{repo}/contents/{path}": regexp.MustCompile(`^\/repos\/\S+\/\S+\/contents\/\S+$`),

		// Get a commit.
		// https://docs.github.com/en/rest/commits/commits?apiVersion=2022-11-28#get-a-commit
		"/repos/{owner}/{repo}/commits/{ref}": regexp.MustCompile(`^\/repos\/\S+\/\S+\/commits\/\S+$`),

		// Refresh auth token.
		// https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/refreshing-user-access-tokens#refreshing-a-user-access-token-with-a-refresh-token
		"/login/oauth/access_token": regexp.MustCompile(`^\/login\/oauth\/access_token$`),
	}
)

func InstrumentedHTTPClient(logger log.Logger, reg prometheus.Registerer) *http.Client {
	apiDuration := util.RegisterOrGet(reg, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "pyroscope",
			Name:      "vcs_github_request_duration",
			Help:      "Duration of GitHub API requests in seconds",
			Buckets:   prometheus.ExponentialBucketsRange(0.1, 10, 8),
		},
		[]string{"method", "route", "status_code"},
	))

	defaultClient := &http.Client{
		Timeout:   10 * time.Second,
		Transport: http.DefaultTransport,
	}
	client := util.InstrumentedHTTPClient(defaultClient, withGitHubMetricsTransport(logger, apiDuration))
	return client
}

// withGitHubMetricsTransport wraps a transport with a client to track GitHub
// API usage.
func withGitHubMetricsTransport(logger log.Logger, hv *prometheus.HistogramVec) util.RoundTripperInstrumentFunc {
	return func(next http.RoundTripper) http.RoundTripper {
		return util.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			route := matchGitHubAPIRoute(req.URL.Path)
			statusCode := ""
			start := time.Now()

			res, err := next.RoundTrip(req)
			if err == nil {
				statusCode = fmt.Sprintf("%d", res.StatusCode)
			}

			if route == "unknown_route" {
				level.Warn(logger).Log("path", req.URL.Path, "msg", "unknown GitHub API route")
			}
			hv.WithLabelValues(req.Method, route, statusCode).Observe(time.Since(start).Seconds())

			return res, err
		})
	}
}

func matchGitHubAPIRoute(path string) string {
	for route, regex := range githubRouteMatchers {
		if regex.MatchString(path) {
			return route
		}
	}

	return "unknown_route"
}
