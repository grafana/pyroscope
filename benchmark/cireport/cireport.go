package cireport

import (
	"bytes"
	"text/template"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type ciReport struct {
	q Querier
}
type Querier interface {
	Instant(query string, t time.Time) (model.Value, v1.Warnings, error)
}

func New(q Querier) *ciReport {
	return &ciReport{
		q,
	}
}

//func (r *ciReport) Screenshot() (string, error) {
//	// query grafana api for a dashboard
//	// get all panes
//	// get each pane individually
//	// take screenshot
//	// upload to s3
//}

// Report reports benchmarking results in markdown format
func (r *ciReport) Report() (string, error) {

	query := `rate(pyroscope_http_request_duration_seconds_count{handler="/ingest", code="200"}[5m])`
	now := time.Now()

	result, _, err := r.q.Instant(query, now)
	if err != nil {
		return "", err
	}

	// TODO(eh-am): mount markdown
	var tpl bytes.Buffer

	data := struct {
		Result model.Value
	}{
		Result: result,
	}

	t, err := template.New("tw").Parse(`
# benchmarking results
string {{ .Result }}
	`)
	if err != nil {
		return "", err
	}

	if err := t.Execute(&tpl, data); err != nil {
		return "", err
	}

	return tpl.String(), nil
}
