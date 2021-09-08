package cireport

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io/ioutil"
	"math"
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

type Query struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description" default:""`

	Base         string `yaml:"base"`
	BaseResult   float64
	Target       string `yaml:"target"`
	TargetResult float64
	// TODO(eh-am): implement a default value
	DiffThreshold float64 `yaml:"diffThresholdPercent"`
	DiffPercent   float64
	// Absolute value
	DiffPercentAbs float64

	// Indicates whether a bigger value is better or not
	BiggerIsBetter bool `yaml:"biggerIsBetter"`

	mu sync.Mutex
}

type QueriesConfig struct {
	BaseName   string `yaml:"baseName"`
	TargetName string `yaml:"targetName"`

	Queries []Query `yaml:"queries"`
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
	for index, queries := range r.qCfg.Queries {
		i := index
		q := queries
		g.Go(func() error {
			baseResult, err := r.q.Instant(q.Base, now)
			if err != nil {
				return err
			}
			targetResult, err := r.q.Instant(q.Target, now)
			if err != nil {
				return err
			}

			diffPercent := ((targetResult - baseResult) / (targetResult + baseResult)) * 100

			// TODO(eh-am): should I lock the whole array?
			q.mu.Lock()
			r.qCfg.Queries[i].BaseResult = baseResult
			r.qCfg.Queries[i].TargetResult = targetResult

			// compute as much as possible beforehand
			// so that the template code is cleaner
			r.qCfg.Queries[i].DiffPercent = diffPercent
			r.qCfg.Queries[i].DiffPercentAbs = math.Abs(diffPercent)
			q.mu.Unlock()
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

	t, err := template.New("pr.gotpl").
		Funcs(template.FuncMap{
			"formatDiff": formatDiff,
		}).
		ParseFS(resources, "resources/pr.gotpl")
	if err != nil {
		return "", err
	}

	if err := t.Execute(&tpl, data); err != nil {
		return "", err
	}

	return tpl.String(), nil
}

// formatDiff formats diff in a markdown intended format
func formatDiff(q Query) string {
	diffPercent := ((q.TargetResult - q.BaseResult) / ((q.TargetResult + q.BaseResult) / 2)) * 100.0

	res := fmt.Sprintf("%.2f (%.2f%%)", q.TargetResult-q.BaseResult, diffPercent)

	// TODO: use something friendlier to colourblind people?
	goodEmoji := ":green_square:"
	badEmoji := ":red_square:"

	// is threshold relevant?
	if math.Abs(diffPercent) > q.DiffThreshold {
		if q.BiggerIsBetter { // higher is better
			if q.TargetResult > q.BaseResult {
				return fmt.Sprintf("%s %s", goodEmoji, res)
			} else {
				return fmt.Sprintf("%s %s", badEmoji, res)
			}
		} else { // lower is better
			if q.TargetResult < q.BaseResult {
				return fmt.Sprintf("%s %s", goodEmoji, res)
			} else {
				return fmt.Sprintf("%s %s", badEmoji, res)
			}
		}
	}

	return res
}
