package cireport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type ScreenshotPaneConfig struct {
	GrafanaURL     string
	DashboardUid   string
	PanelId        int
	Dest           string
	From           int
	To             int
	Width          int
	Height         int
	TimeoutSeconds int
}

// ScreenshotPane takes screenshot of a grafana pane
func ScreenshotPane(ctx context.Context, cfg ScreenshotPaneConfig) error {
	logrus.Debug("screenshoting pane", cfg.PanelId)
	if _, err := os.Stat(cfg.Dest); err == nil {
		// File exists
		return fmt.Errorf("file exists %s, won't overwrite", cfg.Dest)
	} else if os.IsNotExist(err) {
		// file exists
	} else {
		// unkown error
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.GrafanaURL+"/render/d-solo/"+cfg.DashboardUid, nil)
	if err != nil {
		return err
	}

	q := req.URL.Query()
	q.Add("from", strconv.Itoa(cfg.From))
	q.Add("to", strconv.Itoa(cfg.To))
	q.Add("panelId", strconv.Itoa(cfg.PanelId))

	req.URL.RawQuery = q.Encode()

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	logrus.Debugf("writing pane %d to %s\n", cfg.PanelId, cfg.Dest)
	err = os.WriteFile(cfg.Dest, data, 0666)
	if err != nil {
		return err
	}

	return nil
}

type GetAllPaneIdsConfig struct {
	GrafanaURL     string
	DashboardUid   string
	TimeoutSeconds int
}

// GetAllPaneIds retrieves all panes id for a given dashboard
// It assumes there are no rows in the dashboard
func GetAllPaneIds(ctx context.Context, cfg GetAllPaneIdsConfig) ([]int, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.GrafanaURL+"/api/dashboards/uid/"+cfg.DashboardUid, nil)
	if err != nil {
		return []int{}, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return []int{}, err
	}

	// bare minimum we need from the endpoint
	var j struct {
		Dashboard struct {
			Panels []struct {
				Id int `json:"id"`
			} `json:"panels"`
		} `json:"dashboard"`
	}

	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return []int{}, err
	}

	var ids []int
	for _, v := range j.Dashboard.Panels {
		ids = append(ids, v.Id)
	}

	return ids, nil
}

type ScreenshotAllPanesConfig struct {
	GrafanaURL            string
	DashboardUid          string
	Dest                  string
	From                  int
	To                    int
	TimeoutSecondsPerPane int
}

// ScreenshotAllPanes take a screenshot of every single pane
// It assumes there are no rows
func ScreenshotAllPanes(ctx context.Context, cfg ScreenshotAllPanesConfig) ([]int, error) {
	logrus.Debug("getting all ids from dashboard ", cfg.DashboardUid)
	ids, err := GetAllPaneIds(ctx, GetAllPaneIdsConfig{
		GrafanaURL:     cfg.GrafanaURL,
		DashboardUid:   cfg.DashboardUid,
		TimeoutSeconds: cfg.TimeoutSecondsPerPane,
	})
	if err != nil {
		return ids, err
	}

	g, ctx := errgroup.WithContext(ctx)

	logrus.Debug("taking screenshot of panes ", ids)
	for _, v := range ids {
		i := v // // https://golang.org/doc/faq#closures_and_goroutines
		g.Go(func() error {
			return ScreenshotPane(ctx,
				ScreenshotPaneConfig{
					Dest: path.Join(cfg.Dest, strconv.Itoa(i)+".png"),

					GrafanaURL:     cfg.GrafanaURL,
					DashboardUid:   cfg.DashboardUid,
					PanelId:        i,
					Width:          500,
					Height:         500,
					From:           cfg.From,
					To:             cfg.To,
					TimeoutSeconds: cfg.TimeoutSecondsPerPane,
				})
		})
	}
	//wg.Wait()
	if err := g.Wait(); err != nil {
		return []int{}, err
	}

	return ids, err
}
