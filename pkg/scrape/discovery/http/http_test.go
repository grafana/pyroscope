// Copyright 2021 The Prometheus Authors
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

package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/targetgroup"
	pmodel "github.com/pyroscope-io/pyroscope/pkg/scrape/model"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestHTTPValidRefresh(t *testing.T) {
	ts := httptest.NewServer(http.FileServer(http.Dir("./fixtures")))
	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL + "/http_sd.good.json",
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, logrus.New())
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	expectedTargets := []*targetgroup.Group{
		{
			Targets: []pmodel.LabelSet{
				{
					pmodel.AppNameLabel: pmodel.LabelValue("test"),
					pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1:9090"),
				},
			},
			Labels: pmodel.LabelSet{
				pmodel.LabelName("__meta_datacenter"): pmodel.LabelValue("bru1"),
				pmodel.LabelName("__meta_url"):        pmodel.LabelValue(ts.URL + "/http_sd.good.json"),
			},
			Source: urlSource(ts.URL+"/http_sd.good.json", 0),
		},
	}
	require.Equal(t, tgs, expectedTargets)
}

func TestHTTPInvalidCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, logrus.New())
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.EqualError(t, err, "server returned HTTP status 400 Bad Request")
}

func TestHTTPInvalidFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "{}")
	}))

	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, logrus.New())
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.EqualError(t, err, `unsupported content type "text/plain; charset=utf-8"`)
}

func TestContentTypeRegex(t *testing.T) {
	cases := []struct {
		header string
		match  bool
	}{
		{
			header: "application/json;charset=utf-8",
			match:  true,
		},
		{
			header: "application/json;charset=UTF-8",
			match:  true,
		},
		{
			header: "Application/JSON;Charset=\"utf-8\"",
			match:  true,
		},
		{
			header: "application/json; charset=\"utf-8\"",
			match:  true,
		},
		{
			header: "application/json",
			match:  true,
		},
		{
			header: "application/jsonl; charset=\"utf-8\"",
			match:  false,
		},
		{
			header: "application/json;charset=UTF-9",
			match:  false,
		},
		{
			header: "application /json;charset=UTF-8",
			match:  false,
		},
		{
			header: "application/ json;charset=UTF-8",
			match:  false,
		},
		{
			header: "application/json;",
			match:  false,
		},
		{
			header: "charset=UTF-8",
			match:  false,
		},
	}

	for _, test := range cases {
		t.Run(test.header, func(t *testing.T) {
			require.Equal(t, test.match, matchContentType.MatchString(test.header))
		})
	}
}

func TestSourceDisappeared(t *testing.T) {
	var stubResponse string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, stubResponse)
	}))
	t.Cleanup(ts.Close)

	cases := []struct {
		responses       []string
		expectedTargets [][]*targetgroup.Group
	}{
		{
			responses: []string{
				`[]`,
				`[]`,
			},
			expectedTargets: [][]*targetgroup.Group{{}, {}},
		},
		{
			responses: []string{
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"],"application":"test"}]`,
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"],"application":"test"}, 
				 {"labels": {"k": "2"}, "targets": ["127.0.0.1"],"application":"test1"}]`,
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AppNameLabel: pmodel.LabelValue("test"),
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("1"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
				},
				{
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AppNameLabel: pmodel.LabelValue("test"),
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("1"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AppNameLabel: pmodel.LabelValue("test1"),
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("2"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 1),
					},
				},
			},
		},
		{
			responses: []string{
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"],"application":"test1"},
				 {"labels": {"k": "2"}, "targets": ["127.0.0.1"],"application":"test2"}]`,
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"],"application":"test1"}]`,
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AppNameLabel: pmodel.LabelValue("test1"),
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("1"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AppNameLabel: pmodel.LabelValue("test2"),
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("2"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 1),
					},
				},
				{
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AppNameLabel: pmodel.LabelValue("test1"),
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("1"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: nil,
						Labels:  nil,
						Source:  urlSource(ts.URL, 1),
					},
				},
			},
		},
		{
			responses: []string{
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"], "application":"test1"}, {"labels": {"k": "2"}, "targets": ["127.0.0.1"],"application":"test2"}, {"labels": {"k": "3"}, "targets": ["127.0.0.1"],"application":"test3"}]`,
				`[{"labels": {"k": "1"}, "targets": ["127.0.0.1"],"application":"test1"}]`,
				`[{"labels": {"k": "v"}, "targets": ["127.0.0.2"],"application":"testv"}, {"labels": {"k": "vv"}, "targets": ["127.0.0.3"],"application":"testvv"}]`,
			},
			expectedTargets: [][]*targetgroup.Group{
				{
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
								pmodel.AppNameLabel: "test1",
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("1"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
								pmodel.AppNameLabel: "test2",
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("2"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 1),
					},
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
								pmodel.AppNameLabel: "test3",
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("3"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 2),
					},
				},
				{
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AppNameLabel: "test1",
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.1"),
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("1"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: nil,
						Labels:  nil,
						Source:  urlSource(ts.URL, 1),
					},
					{
						Targets: nil,
						Labels:  nil,
						Source:  urlSource(ts.URL, 2),
					},
				},
				{
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.2"),
								pmodel.AppNameLabel: "testv",
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("v"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 0),
					},
					{
						Targets: []pmodel.LabelSet{
							{
								pmodel.AddressLabel: pmodel.LabelValue("127.0.0.3"),
								pmodel.AppNameLabel: "testvv",
							},
						},
						Labels: pmodel.LabelSet{
							pmodel.LabelName("k"):          pmodel.LabelValue("vv"),
							pmodel.LabelName("__meta_url"): pmodel.LabelValue(ts.URL),
						},
						Source: urlSource(ts.URL, 1),
					},
				},
			},
		},
	}

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(1 * time.Second),
	}
	d, err := NewDiscovery(&cfg, logrus.New())
	require.NoError(t, err)
	for _, test := range cases {
		ctx := context.Background()
		for i, res := range test.responses {
			stubResponse = res
			tgs, err := d.refresh(ctx)
			require.NoError(t, err)
			require.Equal(t, test.expectedTargets[i], tgs)
		}
	}
}
