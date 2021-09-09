package cireport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type GrafanaScreenshotter struct {
	GrafanaURL     string
	TimeoutSeconds int
}

type ScreenshotPanelConfig struct {
	DashboardUID string
	PanelId      int
	From         int64
	To           int64
	Width        int
	Height       int
}

// ScreenshotPanel takes screenshot of a grafana panel
func (gs *GrafanaScreenshotter) ScreenshotPanel(ctx context.Context, cfg ScreenshotPanelConfig) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(gs.TimeoutSeconds)*time.Second)
	defer cancel()
	// TODO(eh-am): pass width/height
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gs.GrafanaURL+"/render/d-solo/"+cfg.DashboardUID, nil)
	if err != nil {
		return []byte{}, err
	}

	q := req.URL.Query()
	q.Add("from", strconv.FormatInt(cfg.From, 10))
	q.Add("to", strconv.FormatInt(cfg.To, 10))
	q.Add("panelId", strconv.Itoa(cfg.PanelId))

	req.URL.RawQuery = q.Encode()

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return []byte{}, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}

	return data, nil
}

type Panel struct {
	ID    int    `json: "id"`
	Title string `json: "title"`
	Data  []byte
}

// GetAllPaneIds retrieves all panes id for a given dashboard
// It assumes there are no rows in the dashboard
func (gs *GrafanaScreenshotter) getAllPanes(ctx context.Context, uid string) ([]Panel, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(gs.TimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gs.GrafanaURL+"/api/dashboards/uid/"+uid, nil)
	if err != nil {
		return []Panel{}, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return []Panel{}, err
	}

	// bare minimum we need from the endpoint
	var j struct {
		Dashboard struct {
			Panels []Panel `json:"panels"`
		} `json:"dashboard"`
	}

	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return []Panel{}, err
	}

	return j.Dashboard.Panels, nil
}

// AllPanes take a screenshot of every single pane
// IMPORTANT! It assumes there are no rows
// TODO: handle rows too
func (gs *GrafanaScreenshotter) AllPanels(ctx context.Context, dashboardUID string, from int64, to int64) ([]Panel, error) {
	logrus.Debug("getting all ids from dashboard ", dashboardUID)
	panels, err := gs.getAllPanes(ctx, dashboardUID)
	if err != nil {
		return []Panel{}, err
	}

	res := make([]Panel, len(panels), len(panels))
	g, ctx := errgroup.WithContext(ctx)

	logrus.Debugf("taking screenshot of panes %+v\n", panels)
	for i, v := range panels {
		i := i
		id := v.ID // https://golang.org/doc/faq#closures_and_goroutines
		g.Go(func() error {
			d, err := gs.ScreenshotPanel(ctx,
				ScreenshotPanelConfig{
					DashboardUID: dashboardUID,
					PanelId:      id,
					Width:        500,
					Height:       500,
					From:         from,
					To:           to,
				})

			res[i] = panels[i]
			// TODO: do we need to lock this?
			res[i].Data = d

			return err
		})
	}

	if err := g.Wait(); err != nil {
		return []Panel{}, err
	}

	return res, err
}
