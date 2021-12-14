package cireport

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"math"
	"os"
	"sync"
	"text/template"
	"time"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
)

var (
	//go:embed resources/*
	resources embed.FS
)

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

	BiggerIsBetter bool `yaml:"biggerIsBetter"`

	mu sync.Mutex
}

type QueriesConfig struct {
	BaseName   string `yaml:"baseName"`
	TargetName string `yaml:"targetName"`

	Queries []Query `yaml:"queries"`
}
type TableReport struct {
	q Querier
}

func NewTableReport(q Querier) *TableReport {
	return &TableReport{
		q,
	}
}

func TableReportCli(q Querier, queriesFile string) (string, error) {
	var qCfg QueriesConfig

	// read the file
	yamlFile, err := os.ReadFile(queriesFile)
	if err != nil {
		return "", err
	}
	err = yaml.Unmarshal(yamlFile, &qCfg)
	if err != nil {
		return "", err
	}

	t := NewTableReport(q)

	return t.Report(context.Background(), &qCfg)
}

// TableReport reports query results from prometheus in markdown format
func (r *TableReport) Report(ctx context.Context, qCfg *QueriesConfig) (string, error) {
	// TODO: treat each error individually?
	g, ctx := errgroup.WithContext(ctx)

	now := time.Now()
	for index, queries := range qCfg.Queries {
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
			qCfg.Queries[i].BaseResult = baseResult
			qCfg.Queries[i].TargetResult = targetResult

			// compute as much as possible beforehand
			// so that the template code is cleaner
			qCfg.Queries[i].DiffPercent = diffPercent
			q.mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return "", err
	}

	var tpl bytes.Buffer

	data := struct {
		QC *QueriesConfig
	}{
		QC: qCfg,
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
			emoji := badEmoji
			if q.TargetResult > q.BaseResult {
				emoji = goodEmoji
			}

			return fmt.Sprintf("%s %s", emoji, res)
		}

		// lower is better
		emoji := badEmoji
		if q.TargetResult < q.BaseResult {
			emoji = goodEmoji
		}
		return fmt.Sprintf("%s %s", emoji, res)
	}

	return res
}
