package cireport

import (
	"context"
	"encoding/json"
	"fmt"
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
	PanelID      int
	From         int64
	To           int64
	Width        int
	Height       int
}

// ScreenshotPanel takes screenshot of a grafana panel
func (gs *GrafanaScreenshotter) ScreenshotPanel(ctx context.Context, cfg ScreenshotPanelConfig) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(gs.TimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gs.GrafanaURL+"/render/d-solo/"+cfg.DashboardUID, nil)
	if err != nil {
		return []byte{}, err
	}

	q := req.URL.Query()
	q.Add("from", strconv.FormatInt(cfg.From, 10))
	q.Add("to", strconv.FormatInt(cfg.To, 10))
	q.Add("panelId", strconv.Itoa(cfg.PanelID))

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
	ID    int    `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Data  []byte
}

type Row struct {
	Panels []Panel `json:"panels"`
}

// GetAllPaneIds retrieves all panes for a given dashboard
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
			Rows   []Row   `json:"rows"`
		} `json:"dashboard"`
	}

	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return []Panel{}, err
	}

	panelsFromRows := make([]Panel, 0, 0)
	for _, r := range j.Dashboard.Rows {
		for _, p := range r.Panels {
			panelsFromRows = append(panelsFromRows, p)
		}
	}

	return append(j.Dashboard.Panels, panelsFromRows...), nil
}

// AllPanes take a screenshot of all panes in a dashboard
func (gs *GrafanaScreenshotter) AllPanels(ctx context.Context, dashboardUID string, from int64, to int64) ([]Panel, error) {
	logrus.Debug("getting all ids from dashboard ", dashboardUID)
	panels, err := gs.getAllPanes(ctx, dashboardUID)
	if err != nil {
		return []Panel{}, err
	}

	// we don't want to take screenshots of row panels
	panels = removeRowPanels(panels)
	if len(panels) <= 0 {
		return []Panel{}, fmt.Errorf("at least 1 panel is required")
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
					PanelID:      id,
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

func removeRowPanels(panels []Panel) []Panel {
	newPanels := make([]Panel, 0, 0)

	for _, p := range panels {
		if p.Type != "row" {
			newPanels = append(newPanels, p)
		}
	}

	return newPanels
}
