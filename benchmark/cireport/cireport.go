package cireport

import (
	"bytes"
	"context"
	"embed"
	"io/ioutil"
	"sync"
	"text/template"
	"time"

	"github.com/pyroscope-io/pyroscope/benchmark/config"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
)

var (
	//go:embed resources/*
	resources embed.FS
)

type ciReport struct {
	q    Querier
	cfg  *config.CIReport
	qCfg *QueriesConfig
}
type Querier interface {
	Instant(query string, t time.Time) (float64, error)
}

type QueriesConfig struct {
	BaseName   string `yaml:"baseName"`
	TargetName string `yaml:"targetName"`

	Queries []struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description" default:""`

		Base          string `yaml:"base"`
		BaseResult    float64
		Target        string `yaml:"target"`
		TargetResult  float64
		DiffThreshold int `yaml:"diffThreshold" default:"5"` // TODO(eh-am): what type to use here
		DiffPercent   float64

		mu sync.Mutex
	} `yaml:"queries"`
}

func New(q Querier, cfg *config.CIReport) (*ciReport, error) {
	var qCfg QueriesConfig

	// read the file
	yamlFile, err := ioutil.ReadFile(cfg.QueriesFile)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlFile, &qCfg)
	if err != nil {
		return nil, err
	}

	return &ciReport{
		q,
		cfg,
		&qCfg,
	}, nil
}

//func (r *ciReport) Screenshot() (string, error) {
//	// query grafana api for a dashboard
//	// get all panes
//	// get each pane individually
//	// take screenshot
//	// upload to s3
//}

// Report reports benchmarking results from prometheus in markdown format
func (r *ciReport) Report(ctx context.Context) (string, error) {
	// TODO: treat each error individually?
	g, ctx := errgroup.WithContext(context.Background())

	now := time.Now()
	for i, queries := range r.qCfg.Queries {
		i := i
		g.Go(func() error {
			baseResult, err := r.q.Instant(queries.Base, now)
			if err != nil {
				return err
			}

			targetResult, err := r.q.Instant(queries.Target, now)
			if err != nil {
				return err
			}

			diffPercent := ((targetResult - baseResult) / (targetResult + baseResult)) * 100

			// TODO(eh-am): should I lock the whole array?
			queries.mu.Lock()
			r.qCfg.Queries[i].BaseResult = baseResult
			r.qCfg.Queries[i].TargetResult = targetResult
			r.qCfg.Queries[i].DiffPercent = diffPercent
			queries.mu.Unlock()
			return nil
		})
	}

	g.Wait()

	var tpl bytes.Buffer

	data := struct {
		QC *QueriesConfig
	}{
		QC: r.qCfg,
	}

	t, err := template.ParseFS(resources, "resources/pr.gotpl")
	if err != nil {
		return "", err
	}

	if err := t.Execute(&tpl, data); err != nil {
		return "", err
	}

	return tpl.String(), nil
}
