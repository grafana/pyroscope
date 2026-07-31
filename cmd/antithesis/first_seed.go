package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/v2/pkg/api/connect"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

const (
	defaultWriteURL = "http://pyroscope-distributor:4040"
	defaultReadURL  = "http://pyroscope-query-frontend:4040"

	seedJob         = "antithesis"
	seedServiceName = "antithesis-seed"
	seedMarkerLabel = "antithesis_seed"
	seedIDLabel     = "antithesis_seed_id"
)

var seeds = []seed{
	{tenantID: "tenant-a", marker: "tenant-a"},
	{tenantID: "tenant-b", marker: "tenant-b"},
}

type seed struct {
	tenantID string
	marker   string
}

type firstSeedConfig struct {
	writeURL          string
	readURL           string
	httpClient        *http.Client
	now               func() time.Time
	visibilityPoll    time.Duration
	visibilityTimeout time.Duration
}

// FirstSeed pushes a known profile for each test tenant and waits until every
// profile is visible through the query frontend.
func FirstSeed(ctx context.Context) error {
	return firstSeed(ctx, firstSeedConfig{
		writeURL:          envOrDefault("PYROSCOPE_WRITE_URL", defaultWriteURL),
		readURL:           envOrDefault("PYROSCOPE_READ_URL", defaultReadURL),
		httpClient:        &http.Client{Timeout: 10 * time.Second},
		now:               time.Now,
		visibilityPoll:    500 * time.Millisecond,
		visibilityTimeout: time.Minute,
	})
}

func firstSeed(ctx context.Context, cfg firstSeedConfig) error {
	seedTime := cfg.now().Add(-time.Second).Truncate(time.Millisecond)
	seedID := strconv.FormatInt(seedTime.UnixNano(), 10)
	rawProfile, err := buildSmokeProfile(seedTime)
	if err != nil {
		return fmt.Errorf("prepare embedded smoke profile: %w", err)
	}

	pusher := pushv1connect.NewPusherServiceClient(
		cfg.httpClient,
		cfg.writeURL,
		connectapi.DefaultClientOptions()...,
	)
	querier := querierv1connect.NewQuerierServiceClient(
		cfg.httpClient,
		cfg.readURL,
		connectapi.DefaultClientOptions()...,
	)

	for _, s := range seeds {
		if err := pushSeed(ctx, pusher, s, seedID, rawProfile); err != nil {
			return fmt.Errorf("push seed for tenant %q: %w", s.tenantID, err)
		}
		_ = level.Info(logger).Log("msg", "seed profile pushed", "tenant", s.tenantID, "marker", s.marker, "seed_id", seedID)
	}

	for _, s := range seeds {
		if err := waitForSeed(ctx, querier, cfg, s, seedID, seedTime); err != nil {
			return fmt.Errorf("wait for seed for tenant %q: %w", s.tenantID, err)
		}
		_ = level.Info(logger).Log("msg", "seed profile visible", "tenant", s.tenantID, "marker", s.marker, "seed_id", seedID)
	}

	return nil
}

func pushSeed(
	ctx context.Context,
	pusher pushv1connect.PusherServiceClient,
	s seed,
	seedID string,
	rawProfile []byte,
) error {
	req := connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{{
			Labels: model.LabelsFromStrings(
				"__name__", "process_cpu",
				"job", seedJob,
				"service_name", seedServiceName,
				seedMarkerLabel, s.marker,
				seedIDLabel, seedID,
			),
			Samples: []*pushv1.RawSample{{
				ID:         uuid.NewSHA1(uuid.NameSpaceOID, []byte(seedID+":"+s.tenantID)).String(),
				RawProfile: rawProfile,
			}},
		}},
	})
	req.Header().Set("X-Scope-OrgID", s.tenantID)
	_, err := pusher.Push(ctx, req)
	return err
}

func waitForSeed(
	ctx context.Context,
	querier querierv1connect.QuerierServiceClient,
	cfg firstSeedConfig,
	s seed,
	seedID string,
	seedTime time.Time,
) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.visibilityTimeout)
	defer cancel()

	ticker := time.NewTicker(cfg.visibilityPoll)
	defer ticker.Stop()

	var lastErr error
	for {
		req := connect.NewRequest(&typesv1.LabelValuesRequest{
			Name:     seedIDLabel,
			Matchers: []string{fmt.Sprintf(`{service_name=%q,%s=%q}`, seedServiceName, seedMarkerLabel, s.marker)},
			Start:    seedTime.Add(-time.Minute).UnixMilli(),
			End:      seedTime.Add(time.Hour).UnixMilli(),
		})
		req.Header().Set("X-Scope-OrgID", s.tenantID)
		resp, err := querier.LabelValues(ctx, req)
		if err == nil && slices.Contains(resp.Msg.Names, seedID) {
			return nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("seed %q did not become visible: %w", seedID, lastErr)
			}
			return fmt.Errorf("seed %q did not become visible: %w", seedID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
