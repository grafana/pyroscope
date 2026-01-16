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
//

package debuginfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"go.opentelemetry.io/otel/trace/noop"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

func TestHTTPDebugInfodClient_request(t *testing.T) {
	r, err := recorder.New("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		t.Log(r.Stop())
	})

	type fields struct {
		UpstreamServers []*url.URL
		timeoutDuration time.Duration
	}
	type args struct {
		u       url.URL
		buildID string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "success",
			fields: fields{
				timeoutDuration: 30 * time.Second,
			},
			args: args{
				u: url.URL{
					Scheme: "http",
					Host:   "debuginfod.elfutils.org",
				},
				buildID: "d278249792061c6b74d1693ca59513be1def13f2",
			},
			want:    `ELF 64-bit LSB shared object, x86-64, version 1 (GNU/Linux), dynamically linked, interpreter , BuildID[sha1]=d278249792061c6b74d1693ca59513be1def13f2, for GNU/Linux 3.2.0, with debug_info, not stripped`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewHTTPDebuginfodClient(
				noop.NewTracerProvider(),
				&http.Client{
					// Recorder transport passed, so the recorded data from testdata/fixtures will ber passed.
					// Use make go/test-clean to remove the recorded data.
					Transport: r,
				},
				tt.args.u,
			)
			ctx, cancel := context.WithTimeout(context.Background(), tt.fields.timeoutDuration)
			t.Cleanup(cancel)

			r, err := c.debuginfoRequest(ctx, tt.args.buildID)
			if (err != nil) != tt.wantErr {
				t.Errorf("debuginfoRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			t.Cleanup(func() {
				r.Close()
			})

			tmpfile, err := os.CreateTemp("", "debuginfod-download-*")
			require.NoError(t, err)

			t.Cleanup(func() {
				os.Remove(tmpfile.Name())
			})

			downloadAndCompare(t, r, tt.want)
		})
	}
}

func downloadAndCompare(t *testing.T, r io.ReadCloser, want string) {
	t.Helper()

	tmpfile, err := os.CreateTemp("", "debuginfod-download-*")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.Remove(tmpfile.Name())
	})

	_, err = io.Copy(tmpfile, r)
	require.NoError(t, err)

	require.NoError(t, tmpfile.Close())

	cmd := exec.Command("file", tmpfile.Name())

	stdout, err := cmd.Output()
	require.NoError(t, err)

	got := strings.TrimSpace(strings.Split(string(stdout), ":")[1])

	// For some reason the output of the `file` command is not always
	// consistent across architectures, and in the amd64 case even
	// inserts an escaped tab causing the string to contain `\011`. So
	// we remove the inconsistencies and ten compare output strings.
	got = strings.ReplaceAll(got, "\t", "")
	got = strings.ReplaceAll(got, "\\011", "")
	require.Equal(t, want, got)
}

func TestHTTPDebugInfodClientRedirect(t *testing.T) {
	ds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "test")
	}))
	defer ds.Close()

	rs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ds.URL+r.URL.Path, http.StatusFound)
	}))
	defer rs.Close()

	u, err := url.Parse(rs.URL)
	require.NoError(t, err)

	c := NewHTTPDebuginfodClient(noop.NewTracerProvider(), &http.Client{
		Timeout: 30 * time.Second,
	}, *u)

	ctx := context.Background()
	r, err := c.Get(ctx, "d278249792061c6b74d1693ca59513be1def13f2")
	require.NoError(t, err)
	require.NotNil(t, r)

	content, err := io.ReadAll(r)
	require.NoError(t, err)

	require.Equal(t, "test", string(content))
}

type fakeDebuginfodClient struct {
	get       func(ctx context.Context, buildID string) (io.ReadCloser, error)
	getSource func(ctx context.Context, buildID string) (io.ReadCloser, error)
	exists    func(ctx context.Context, buildID string) (bool, error)
}

func (f *fakeDebuginfodClient) Get(ctx context.Context, buildID string) (io.ReadCloser, error) {
	return f.get(ctx, buildID)
}

func (f *fakeDebuginfodClient) GetSource(ctx context.Context, buildID, file string) (io.ReadCloser, error) {
	return f.getSource(ctx, buildID)
}

func (f *fakeDebuginfodClient) Exists(ctx context.Context, buildID string) (bool, error) {
	return f.exists(ctx, buildID)
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func TestHTTPDebugInfodCache(t *testing.T) {
	c := &fakeDebuginfodClient{
		get: func(ctx context.Context, buildID string) (io.ReadCloser, error) {
			return nopCloser{bytes.NewBuffer([]byte("test"))}, nil
		},
	}

	cache := NewDebuginfodClientWithObjectStorageCache(
		log.NewNopLogger(),
		objstore.NewInMemBucket(),
		c,
	)

	ctx := context.Background()
	r, err := cache.Get(ctx, "test")
	require.NoError(t, err)
	content, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "test", string(content))

	// Test that the cache works.
	c.get = func(ctx context.Context, buildID string) (io.ReadCloser, error) {
		return nil, errors.New("should not be called")
	}

	r, err = cache.Get(ctx, "test")
	require.NoError(t, err)
	content, err = io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "test", string(content))
}
