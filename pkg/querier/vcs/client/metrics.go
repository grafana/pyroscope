package client

import (
	"fmt"
	"time"

	"github.com/google/go-github/v58/github"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics can record events surrounding various client actions, like logging in
// or fetching a file.
type Metrics interface {
	LoginObserve(elapsed time.Duration, err error)
	RefreshObserve(elapsed time.Duration, err error)
	GetCommitObserve(elapsed time.Duration, res *github.Response, err error)
	GetFileObserve(elapsed time.Duration, res *github.Response, err error)
}

func NewMetrics(reg prometheus.Registerer) Metrics {
	return &githubMetrics{
		APIDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pyroscope",
				Name:      "vcs_github_request_duration",
				Help:      "Duration of GitHub API requests in seconds",
				Buckets:   prometheus.ExponentialBucketsRange(0.1, 10, 8),
			},
			[]string{"path", "status"},
		),
		APIRateLimit: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: "pyroscope",
			Name:      "vcs_github_rate_limit",
			Help:      "Remaining GitHub API requests before rate limiting occurs",
		}),
	}
}

type githubMetrics struct {
	APIDuration  *prometheus.HistogramVec
	APIRateLimit prometheus.Gauge
}

func (m *githubMetrics) LoginObserve(elapsed time.Duration, err error) {
	// We technically don't know the true status codes of the OAuth login flow,
	// but we'll assume "no error" means 200 and "an error" means 400. A 400 is
	// chosen because that's what we report to the user.
	status := "200"
	if err != nil {
		status = "400"
	}

	m.APIDuration.
		WithLabelValues("/login/oauth/authorize", status).
		Observe(elapsed.Seconds())
}

func (m *githubMetrics) RefreshObserve(elapsed time.Duration, err error) {
	// We technically don't know the true status codes of the OAuth login flow,
	// but we'll assume "no error" means 200 and "an error" means 400. A 400 is
	// chosen because that's what we report to the user.
	status := "200"
	if err != nil {
		status = "400"
	}

	m.APIDuration.
		WithLabelValues("/login/oauth/access_token", status).
		Observe(elapsed.Seconds())
}

func (m *githubMetrics) GetCommitObserve(elapsed time.Duration, res *github.Response, err error) {
	var status string
	if res != nil {
		status = fmt.Sprintf("%d", res.StatusCode)
		m.APIRateLimit.Set(float64(res.Rate.Remaining))
	}

	if err != nil {
		status = "500"
	}

	m.APIDuration.
		WithLabelValues("/repos/{owner}/{repo}/commits/{ref}", status).
		Observe(elapsed.Seconds())
}

func (m *githubMetrics) GetFileObserve(elapsed time.Duration, res *github.Response, err error) {
	var status string
	if res != nil {
		status = fmt.Sprintf("%d", res.StatusCode)
		m.APIRateLimit.Set(float64(res.Rate.Remaining))
	}

	if err != nil {
		status = "500"
	}

	m.APIDuration.
		WithLabelValues("/repos/{owner}/{repo}/contents/{path}", status).
		Observe(elapsed.Seconds())
}
