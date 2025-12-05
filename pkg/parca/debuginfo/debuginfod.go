// Copyright 2022-2025 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package debuginfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"path"
	"sync"
	"time"

	debuginfopb "buf.build/gen/go/parca-dev/parca/protocolbuffers/go/parca/debuginfo/v1alpha1"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/thanos-io/objstore"
	"go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/pyroscope/pkg/parca/cache"
)

type DebuginfodClients interface {
	Get(ctx context.Context, server, buildid string) (io.ReadCloser, error)
	GetSource(ctx context.Context, server, buildid, file string) (io.ReadCloser, error)
	Exists(ctx context.Context, buildid string) ([]string, error)
}

type NopDebuginfodClients struct{}

func (NopDebuginfodClients) Get(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), ErrDebuginfoNotFound
}

func (NopDebuginfodClients) GetSource(context.Context, string, string, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), ErrDebuginfoNotFound
}

func (NopDebuginfodClients) Exists(context.Context, string) ([]string, error) {
	return nil, nil
}

type DebuginfodClient interface {
	Get(ctx context.Context, buildid string) (io.ReadCloser, error)
	GetSource(ctx context.Context, buildid, file string) (io.ReadCloser, error)
	Exists(ctx context.Context, buildid string) (bool, error)
}

type NopDebuginfodClient struct{}

func (NopDebuginfodClient) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), ErrDebuginfoNotFound
}

func (NopDebuginfodClient) GetSource(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), ErrDebuginfoNotFound
}

func (NopDebuginfodClient) Exists(context.Context, string) (bool, error) {
	return false, nil
}

type DebuginfodClientConfig struct {
	Host   string
	Client DebuginfodClient
}

type ParallelDebuginfodClients struct {
	clientsMap map[string]DebuginfodClient
	clients    []DebuginfodClientConfig
}

func NewDebuginfodClients(
	logger log.Logger,
	reg prometheus.Registerer,
	tracerProvider trace.TracerProvider,
	upstreamServerHosts []string,
	rt http.RoundTripper,
	timeout time.Duration,
	bucket objstore.Bucket,
) DebuginfodClients {
	clients := make([]DebuginfodClientConfig, 0, len(upstreamServerHosts))
	for _, host := range upstreamServerHosts {
		clients = append(clients, DebuginfodClientConfig{
			Host: host,
			Client: NewDebuginfodTracingClient(
				tracerProvider.Tracer("debuginfod-client"),
				NewDebuginfodExistsClientCache(
					prometheus.WrapRegistererWith(prometheus.Labels{"cache": "debuginfod_exists", "debuginfod_host": host}, reg),
					8*1024,
					NewDebuginfodClientWithObjectStorageCache(
						logger,
						objstore.NewPrefixedBucket(bucket, host),
						NewHTTPDebuginfodClient(
							tracerProvider,
							&http.Client{
								Timeout: timeout,
								Transport: promhttp.InstrumentRoundTripperCounter(
									promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
										Name: "parca_debuginfo_client_requests_total",
										Help: "Total number of requests sent by the debuginfo client.",
										ConstLabels: prometheus.Labels{
											"debuginfod_host": host,
										},
									}, []string{"code", "method"}),
									rt,
								),
							},
							url.URL{Scheme: "https", Host: host},
						),
					),
				),
			),
		})
	}

	return NewParallelDebuginfodClients(clients)
}

func NewParallelDebuginfodClients(clients []DebuginfodClientConfig) *ParallelDebuginfodClients {
	clientsMap := make(map[string]DebuginfodClient, len(clients))
	for _, c := range clients {
		clientsMap[c.Host] = c.Client
	}

	return &ParallelDebuginfodClients{
		clientsMap: clientsMap,
		clients:    clients,
	}
}

func (c *ParallelDebuginfodClients) Get(ctx context.Context, server, buildid string) (io.ReadCloser, error) {
	client, ok := c.clientsMap[server]
	if !ok {
		return nil, fmt.Errorf("no client for server %q", server)
	}

	return client.Get(ctx, buildid)
}

func (c *ParallelDebuginfodClients) GetSource(ctx context.Context, server, buildid, file string) (io.ReadCloser, error) {
	client, ok := c.clientsMap[server]
	if !ok {
		return nil, fmt.Errorf("no client for server %q", server)
	}

	return client.GetSource(ctx, buildid, file)
}

func (c *ParallelDebuginfodClients) Exists(ctx context.Context, buildid string) ([]string, error) {
	availability := make([]bool, len(c.clients))
	availabilityCount := 0

	var g sync.WaitGroup
	for i, cfg := range c.clients {
		g.Add(1)
		go func(i int, cfg DebuginfodClientConfig) {
			defer g.Done()

			exists, err := cfg.Client.Exists(ctx, buildid)
			if err != nil {
				// Error is already recorded in the debuginfod client tracing.
				return
			}

			if exists {
				availability[i] = true
				availabilityCount++
			}
		}(i, cfg)
	}
	g.Wait()

	// We do this to preserve the order of servers as we want the order to
	// preserve the precedence.
	res := make([]string, 0, availabilityCount)
	for i, cfg := range c.clients {
		if availability[i] {
			res = append(res, cfg.Host)
		}
	}

	return res, nil
}

type HTTPDebuginfodClient struct {
	tp     trace.TracerProvider
	tracer trace.Tracer

	client *http.Client

	upstreamServer url.URL
}

type DebuginfodClientObjectStorageCache struct {
	logger log.Logger

	client DebuginfodClient
	bucket objstore.Bucket
}

// NewHTTPDebuginfodClient returns a new HTTP debug info client.
func NewHTTPDebuginfodClient(
	tp trace.TracerProvider,
	client *http.Client,
	url url.URL,
) *HTTPDebuginfodClient {
	return &HTTPDebuginfodClient{
		tracer:         tp.Tracer("debuginfod-http-client"),
		tp:             tp,
		upstreamServer: url,
		client:         client,
	}
}

// NewDebuginfodClientWithObjectStorageCache creates a new DebuginfodClient that caches the debug information in the object storage.
func NewDebuginfodClientWithObjectStorageCache(
	logger log.Logger,
	bucket objstore.Bucket,
	client DebuginfodClient,
) DebuginfodClient {
	return &DebuginfodClientObjectStorageCache{
		client: client,
		bucket: bucket,
		logger: logger,
	}
}

// Get returns debuginfo for given buildid while caching it in object storage.
func (c *DebuginfodClientObjectStorageCache) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	rc, err := c.bucket.Get(ctx, objectPath(buildID, debuginfopb.DebuginfoType_DEBUGINFO_TYPE_DEBUGINFO_UNSPECIFIED))
	if err != nil {
		if c.bucket.IsObjNotFoundErr(err) {
			return c.getAndCache(ctx, buildID)
		}

		return nil, err
	}

	return rc, nil
}

// GetSource returns source file for given buildid and file while caching it in object storage.
func (c *DebuginfodClientObjectStorageCache) GetSource(ctx context.Context, buildID, file string) (io.ReadCloser, error) {
	rc, err := c.bucket.Get(ctx, debuginfodSourcePath(buildID, file))
	if err != nil {
		if c.bucket.IsObjNotFoundErr(err) {
			return c.getSourceAndCache(ctx, buildID, file)
		}

		return nil, err
	}

	return rc, nil
}

func (c *DebuginfodClientObjectStorageCache) getAndCache(ctx context.Context, buildID string) (io.ReadCloser, error) {
	r, err := c.client.Get(ctx, buildID)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	if err := c.bucket.Upload(ctx, objectPath(buildID, debuginfopb.DebuginfoType_DEBUGINFO_TYPE_DEBUGINFO_UNSPECIFIED), r); err != nil {
		level.Error(c.logger).Log("msg", "failed to upload downloaded debuginfod file", "err", err, "build_id", buildID)
	}

	r, err = c.bucket.Get(ctx, objectPath(buildID, debuginfopb.DebuginfoType_DEBUGINFO_TYPE_DEBUGINFO_UNSPECIFIED))
	if err != nil {
		return nil, err
	}

	return r, nil
}

func (c *DebuginfodClientObjectStorageCache) getSourceAndCache(ctx context.Context, buildID, file string) (io.ReadCloser, error) {
	r, err := c.client.GetSource(ctx, buildID, file)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	if err := c.bucket.Upload(ctx, debuginfodSourcePath(buildID, file), r); err != nil {
		level.Error(c.logger).Log("msg", "failed to upload downloaded debuginfod file", "err", err, "build_id", buildID, "file", file)
	}

	r, err = c.bucket.Get(ctx, debuginfodSourcePath(buildID, file))
	if err != nil {
		return nil, err
	}

	return r, nil
}

// Exists returns true if debuginfo for given buildid exists.
func (c *DebuginfodClientObjectStorageCache) Exists(ctx context.Context, buildID string) (bool, error) {
	exists, err := c.bucket.Exists(ctx, objectPath(buildID, debuginfopb.DebuginfoType_DEBUGINFO_TYPE_DEBUGINFO_UNSPECIFIED))
	if err != nil {
		return false, err
	}

	if exists {
		return true, nil
	}

	return c.client.Exists(ctx, buildID)
}

// Get returns debug information file for given buildID by downloading it from upstream servers.
func (c *HTTPDebuginfodClient) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	return c.debuginfoRequest(ctx, buildID)
}

func (c *HTTPDebuginfodClient) debuginfoRequest(ctx context.Context, buildID string) (io.ReadCloser, error) {
	// https://www.mankier.com/8/debuginfod#Webapi
	// Endpoint: /buildid/BUILDID/debuginfo
	// If the given buildid is known to the server,
	// this request will result in a binary object that contains the customary .*debug_* sections.
	u := c.upstreamServer
	u.Path = path.Join(u.Path, "buildid", buildID, "debuginfo")

	return c.request(ctx, u.String())
}

// GetSource returns source file for given buildID and file by downloading it from upstream servers.
func (c *HTTPDebuginfodClient) GetSource(ctx context.Context, buildID, file string) (io.ReadCloser, error) {
	// https://www.mankier.com/8/debuginfod#Webapi
	// Endpoint: /buildid/BUILDID/source/FILE
	// If the given buildid and file combination is known to the server,
	// this request will result in a text file that contains the source code.
	u := c.upstreamServer
	u.Path = path.Join(u.Path, "buildid", buildID, "source", file)

	return c.request(ctx, u.String())
}

func (c *HTTPDebuginfodClient) request(ctx context.Context, fullUrl string) (io.ReadCloser, error) {
	ctx = httptrace.WithClientTrace(ctx, otelhttptrace.NewClientTrace(ctx, otelhttptrace.WithTracerProvider(c.tp)))

	resp, err := c.doRequest(ctx, fullUrl)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return c.handleResponse(ctx, resp)
}

func (c *HTTPDebuginfodClient) Exists(ctx context.Context, buildID string) (bool, error) {
	r, err := c.Get(ctx, buildID)
	if err != nil {
		if err == ErrDebuginfoNotFound {
			return false, nil
		}
		return false, err
	}

	return true, r.Close()
}

func (c *HTTPDebuginfodClient) doRequest(ctx context.Context, url string) (*http.Response, error) {
	ctx, span := c.tracer.Start(ctx, "debuginfod-http-request")
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	span.SetAttributes(attribute.String("http.url.host", req.URL.Host))
	span.SetAttributes(attribute.String("http.url", req.URL.String()))

	resp, err := c.client.Do(req)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	return resp, nil
}

func (c *HTTPDebuginfodClient) handleResponse(ctx context.Context, resp *http.Response) (io.ReadCloser, error) {
	var err error
	// Follow at most 2 redirects.
	for i := 0; i < 2; i++ {
		switch resp.StatusCode / 100 {
		case 2:
			return resp.Body, nil
		case 3:
			resp, err = c.doRequest(ctx, resp.Header.Get("Location"))
			if err != nil {
				return nil, fmt.Errorf("request failed: %w", err)
			}

			continue
		case 4:
			if resp.StatusCode == http.StatusNotFound {
				return nil, ErrDebuginfoNotFound
			}
			return nil, fmt.Errorf("client error: %s", resp.Status)
		case 5:
			return nil, fmt.Errorf("server error: %s", resp.Status)
		default:
			return nil, fmt.Errorf("unexpected status code: %s", resp.Status)
		}
	}

	return nil, errors.New("too many redirects")
}

type debuginfodResponse struct {
	lastResponseTime  time.Time
	lastResponseError error
	lastResponse      bool
}

type DebuginfodExistsClientCache struct {
	lruCache *cache.LRUCache[string, debuginfodResponse]

	client DebuginfodClient
}

func NewDebuginfodExistsClientCache(
	reg prometheus.Registerer,
	cacheSize int,
	client DebuginfodClient,
) *DebuginfodExistsClientCache {
	return &DebuginfodExistsClientCache{
		lruCache: cache.NewLRUCache[string, debuginfodResponse](reg, cacheSize),
		client:   client,
	}
}

func (c *DebuginfodExistsClientCache) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	return c.client.Get(ctx, buildID)
}

func (c *DebuginfodExistsClientCache) GetSource(ctx context.Context, buildID, file string) (io.ReadCloser, error) {
	return c.client.GetSource(ctx, buildID, file)
}

func (c *DebuginfodExistsClientCache) Exists(ctx context.Context, buildID string) (bool, error) {
	if v, ok := c.lruCache.Get(buildID); ok {
		if v.lastResponseError == nil || time.Since(v.lastResponseTime) < 10*time.Minute {
			// If there was no error in the last response then we can safely
			// return the cached value. That means we definitively know whether
			// the build ID exists or not. If there was an error in the last
			// response then we use this as a backoff mechanism to only try the
			// same build ID once every 10 minutes.
			return v.lastResponse, v.lastResponseError
		}

		// This means we saw an error last time trying and the 10 minute back
		// off has expired.
	}

	exists, err := c.client.Exists(ctx, buildID)
	c.lruCache.Add(buildID, debuginfodResponse{
		lastResponseTime:  time.Now(),
		lastResponseError: err,
		lastResponse:      exists,
	})
	return exists, err
}

type DebuginfodTracingClient struct {
	tracer trace.Tracer
	client DebuginfodClient
}

func NewDebuginfodTracingClient(
	tracer trace.Tracer,
	client DebuginfodClient,
) *DebuginfodTracingClient {
	return &DebuginfodTracingClient{
		tracer: tracer,
		client: client,
	}
}

func (c *DebuginfodTracingClient) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	ctx, span := c.tracer.Start(ctx, "DebuginfodClient.Get")
	defer span.End()

	span.SetAttributes(attribute.String("buildid", buildID))

	return c.client.Get(ctx, buildID)
}

func (c *DebuginfodTracingClient) GetSource(ctx context.Context, buildID, file string) (io.ReadCloser, error) {
	ctx, span := c.tracer.Start(ctx, "DebuginfodClient.GetSource")
	defer span.End()

	span.SetAttributes(attribute.String("buildid", buildID))
	span.SetAttributes(attribute.String("file", file))

	return c.client.GetSource(ctx, buildID, file)
}

func (c *DebuginfodTracingClient) Exists(ctx context.Context, buildID string) (bool, error) {
	ctx, span := c.tracer.Start(ctx, "DebuginfodClient.Exists")
	defer span.End()

	span.SetAttributes(attribute.String("buildid", buildID))

	return c.client.Exists(ctx, buildID)
}
