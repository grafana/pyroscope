// Copyright 2022 The Parca Authors
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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/client"
	"golang.org/x/net/context"
	"gopkg.in/yaml.v2"
)

type DebugInfodClient interface {
	GetDebugInfo(ctx context.Context, buildid string) (io.ReadCloser, error)
}

type NopDebugInfodClient struct{}

func (NopDebugInfodClient) GetDebugInfo(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), errDebugInfoNotFound
}

type HTTPDebugInfodClient struct {
	logger          log.Logger
	UpstreamServers []*url.URL
	timeoutDuration time.Duration
}

type DebugInfodClientObjectStorageCache struct {
	logger log.Logger

	client DebugInfodClient
	bucket objstore.Bucket
}

// NewHTTPDebugInfodClient returns a new HTTP debug info client.
func NewHTTPDebugInfodClient(logger log.Logger, serverURLs []string, timeoutDuration time.Duration) (*HTTPDebugInfodClient, error) {
	logger = log.With(logger, "component", "debuginfod")
	parsedURLs := make([]*url.URL, 0, len(serverURLs))
	for _, serverURL := range serverURLs {
		u, err := url.Parse(serverURL)
		if err != nil {
			return nil, err
		}

		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
		}
	}
	return &HTTPDebugInfodClient{
		logger:          logger,
		UpstreamServers: parsedURLs,
		timeoutDuration: timeoutDuration,
	}, nil
}

// NewDebugInfodClientWithObjectStorageCache creates a new DebugInfodClient that caches the debug information in the object storage.
func NewDebugInfodClientWithObjectStorageCache(logger log.Logger, config *Config, h DebugInfodClient) (DebugInfodClient, error) {
	logger = log.With(logger, "component", "debuginfod")
	cfg, err := yaml.Marshal(config.Bucket)
	if err != nil {
		return nil, fmt.Errorf("marshal content of debuginfod object storage configuration: %w", err)
	}

	bucket, err := client.NewBucket(logger, cfg, nil, "parca/debuginfod")
	if err != nil {
		return nil, fmt.Errorf("instantiate debuginfod object storage: %w", err)
	}

	return &DebugInfodClientObjectStorageCache{
		logger: logger,
		client: h,
		bucket: bucket,
	}, nil
}

type closer func() error

func (f closer) Close() error { return f() }

type readCloser struct {
	io.Reader
	closer
}

// GetDebugInfo returns debug info for given buildid while caching it in object storage.
func (c *DebugInfodClientObjectStorageCache) GetDebugInfo(ctx context.Context, buildID string) (io.ReadCloser, error) {
	logger := log.With(c.logger, "buildid", buildID)
	debugInfo, err := c.client.GetDebugInfo(ctx, buildID)
	if err != nil {
		return nil, err
	}

	r, w := io.Pipe()
	go func() {
		defer w.Close()
		defer debugInfo.Close()

		if err := c.bucket.Upload(ctx, objectPath(buildID), r); err != nil {
			level.Error(logger).Log("msg", "failed to upload downloaded debuginfod file", "err", err)
		}
	}()

	return readCloser{
		Reader: io.TeeReader(debugInfo, w),
		closer: closer(func() error {
			defer debugInfo.Close()

			if err := w.Close(); err != nil {
				return err
			}
			return nil
		}),
	}, nil
}

// GetDebugInfo returns debug information file for given buildID by downloading it from upstream servers.
func (c *HTTPDebugInfodClient) GetDebugInfo(ctx context.Context, buildID string) (io.ReadCloser, error) {
	logger := log.With(c.logger, "buildid", buildID)

	// e.g:
	//"https://debuginfod.elfutils.org/"
	//"https://debuginfod.systemtap.org/"
	//"https://debuginfod.opensuse.org/"
	//"https://debuginfod.s.voidlinux.org/"
	//"https://debuginfod.debian.net/"
	//"https://debuginfod.fedoraproject.org/"
	//"https://debuginfod.altlinux.org/"
	//"https://debuginfod.archlinux.org/"
	//"https://debuginfod.centos.org/"
	for _, u := range c.UpstreamServers {
		ctx, cancel := context.WithTimeout(ctx, c.timeoutDuration)
		defer cancel()

		serverURL := *u
		rc, err := c.request(ctx, serverURL, buildID)
		if err != nil {
			level.Warn(logger).Log(
				"msg", "failed to download debug info file from upstream debuginfod server, trying next one (if exists)",
				"server", serverURL, "err", err,
			)
			continue
		}
		return rc, nil
	}
	return nil, errDebugInfoNotFound
}

func (c *HTTPDebugInfodClient) request(ctx context.Context, u url.URL, buildID string) (io.ReadCloser, error) {
	// https://www.mankier.com/8/debuginfod#Webapi
	// Endpoint: /buildid/BUILDID/debuginfo
	// If the given buildid is known to the server,
	// this request will result in a binary object that contains the customary .*debug_* sections.
	u.Path = path.Join(u.Path, "buildid", buildID, "debuginfo")

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	switch resp.StatusCode / 100 {
	case 2:
		return resp.Body, nil
	case 4:
		if resp.StatusCode == http.StatusNotFound {
			return nil, errDebugInfoNotFound
		}
		return nil, fmt.Errorf("client error: %s", resp.Status)
	case 5:
		return nil, fmt.Errorf("server error: %s", resp.Status)
	default:
		return nil, fmt.Errorf("unexpected status code: %s", resp.Status)
	}
}
