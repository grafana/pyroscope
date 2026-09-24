package anomalyapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TenantHeader is the header this client sends the tenant ID on, matching the convention
// this repo's own querier clients already use for tenant propagation.
const TenantHeader = "X-Scope-OrgID"

// Anomaly is a single anomaly returned by the configured anomaly source.
type Anomaly struct {
	ProfileUUID string
	Score       float64
	ObservedAt  time.Time
	ModelID     string
}

// Client fetches anomalies from an externally configured anomaly source.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a new Client. Returns nil if cfg.URL is empty -- callers should check for a
// nil Client and treat it as "this anomaly source isn't configured" rather than calling it.
func New(cfg Config, httpClient *http.Client) *Client {
	if cfg.URL == "" {
		return nil
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(cfg.URL, "/"),
		httpClient: httpClient,
	}
}

type listAnomaliesResponse struct {
	Anomalies []struct {
		ProfileUUID string    `json:"profile_uuid"`
		Score       float64   `json:"score"`
		ObservedAt  time.Time `json:"observed_at"`
		ModelID     string    `json:"model_id"`
	} `json:"anomalies"`
}

// ListAnomalies returns the anomalies the configured source has recorded for the given
// tenant/service, observed within [start, end].
func (c *Client) ListAnomalies(ctx context.Context, tenantID, serviceName string, start, end time.Time) ([]Anomaly, error) {
	v := url.Values{
		"service_name": {serviceName},
		"start":        {strconv.FormatInt(start.UnixMilli(), 10)},
		"end":          {strconv.FormatInt(end.UnixMilli(), 10)},
	}
	reqURL := c.baseURL + "/api/v1/anomalydetection/anomalies?" + v.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set(TenantHeader, tenantID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling anomaly source: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anomaly source returned status %d", resp.StatusCode)
	}

	var parsed listAnomaliesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding anomaly source response: %w", err)
	}

	anomalies := make([]Anomaly, len(parsed.Anomalies))
	for i, a := range parsed.Anomalies {
		anomalies[i] = Anomaly{
			ProfileUUID: a.ProfileUUID,
			Score:       a.Score,
			ObservedAt:  a.ObservedAt,
			ModelID:     a.ModelID,
		}
	}
	return anomalies, nil
}
