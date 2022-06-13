package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/parca-dev/parca/pkg/scrape"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
)

type apiFuncResult struct {
	data interface{}
	err  *error
}

// DiscoveredTargets has all the targets discovered by the agent.
type TargetDiscovery struct {
	ActiveTargets  []*APITarget        `json:"activeTargets"`
	DroppedTargets []*APIDroppedTarget `json:"droppedTargets"`
}

// Target has the information for one target.
type APITarget struct {
	// TODO: (callum) do we have these labels?
	// Labels before any processing.
	// DiscoveredLabels map[string]string `json:"discoveredLabels"`
	// Any labels that are added to this target and its profiles.
	Labels map[string]string `json:"labels"`

	ScrapePool string `json:"scrapePool"`
	ScrapeURL  string `json:"scrapeUrl"`

	LastError          string              `json:"lastError"`
	LastScrape         time.Time           `json:"lastScrape"`
	LastScrapeDuration float64             `json:"lastScrapeDuration"`
	Health             scrape.TargetHealth `json:"health"`

	ScrapeInterval string `json:"scrapeInterval"`
	ScrapeTimeout  string `json:"scrapeTimeout"`
}

type APIDroppedTarget struct {
	// Any labels that are added to this target and its profiles.
	Labels    map[string]string `json:"labels"`
	ScrapeURL string            `json:"scrapeUrl"`
}

// targets serves the targets page.
func (a *Agent) TargetsHandler(rw http.ResponseWriter, req *http.Request) {
	// Caller can request only active or dropped targets if they want.
	state := strings.ToLower(req.URL.Query().Get("state"))
	showActive := state == "" || state == "any" || state == "active"
	showDropped := state == "" || state == "any" || state == "dropped"

	res := &TargetDiscovery{}

	if showActive {
		targetsActive := a.ActiveTargets()
		// activeKeys, numTargets := sortKeys(targetsActive)
		res.ActiveTargets = make([]*APITarget, 0, len(targetsActive))
		for group, tg := range targetsActive {
			for _, target := range tg {
				lastErrStr := ""
				lastErr := target.LastError()
				if lastErr != nil {
					lastErrStr = lastErr.Error()
				}

				var err error
				res.ActiveTargets = append(res.ActiveTargets, &APITarget{
					Labels:     target.Labels().Map(),
					ScrapePool: group,
					ScrapeURL:  target.URL().String(),
					LastError: func() string {
						if err == nil && lastErrStr == "" {
							return ""
						} else if err != nil {
							return errors.Wrapf(err, lastErrStr).Error()
						}
						return lastErrStr
					}(),
					LastScrape:         target.LastScrape(),
					LastScrapeDuration: target.LastScrapeDuration().Seconds(),
					Health:             target.Health(),
					ScrapeInterval:     target.GetValue(model.ScrapeIntervalLabel),
					ScrapeTimeout:      target.GetValue(model.ScrapeTimeoutLabel),
				})
			}
		}
	}
	if showDropped {
		// tDropped := flatten(api.targetRetriever(r.Context()).TargetsDropped())
		tDropped := a.DroppedTargets()
		res.DroppedTargets = make([]*APIDroppedTarget, 0, len(tDropped))
		for _, t := range tDropped {
			res.DroppedTargets = append(res.DroppedTargets, &APIDroppedTarget{
				ScrapeURL: t.URL().String(),
				Labels:    t.Labels().Map(),
			})
		}
	}
	rw.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(res)
	if err != nil {
		level.Error(a.logger).Log("msg", "error marshaling json response", "err", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Write(b)
}
