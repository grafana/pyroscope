package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/golang/protobuf/proto"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {
	var httpServer *httptest.Server

	testing.WithConfig(func(cfg **config.Config) {
		BeforeEach(func() {
			(*cfg).Server.APIBindAddr = ":10044"
			s, err := storage.New(storage.NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller))
			Expect(err).ToNot(HaveOccurred())
			e, _ := exporter.NewExporter(nil, nil)
			c, _ := New(Config{
				Configuration:           &(*cfg).Server,
				Storage:                 s,
				MetricsExporter:         e,
				Logger:                  logrus.New(),
				MetricsRegisterer:       prometheus.NewRegistry(),
				ExportedMetricsRegistry: prometheus.NewRegistry(),
				Notifier:                mockNotifier{},
				Adhoc:                   mockAdhocServer{},
			})
			h, _ := c.serverMux()
			httpServer = httptest.NewServer(h)
		})
		Context("/render", func() {
			It("supports name and query parameters", func() {
				defer httpServer.Close()

				resp, err := http.Get(fmt.Sprintf("%s/render?name=%s", httpServer.URL, url.QueryEscape(`app`)))
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				resp, err = http.Get(fmt.Sprintf("%s/render?query=%s", httpServer.URL, url.QueryEscape(`app{foo="bar"}`)))
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				resp, err = http.Get(fmt.Sprintf("%s/render?query=%s", httpServer.URL, url.QueryEscape(`app{foo"bar"}`)))
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

				resp, err = http.Get(fmt.Sprintf("%s/render", httpServer.URL))
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})
			It("supports pprof", func() {
				defer httpServer.Close()

				resp, err := http.Get(fmt.Sprintf("%s/render?query=%s&format=%s", httpServer.URL, url.QueryEscape(`app{foo="bar"}`), "pprof"))
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Disposition")).To(MatchRegexp(
					"^attachment; filename=.+\\.pprof$",
				))
				body, _ := io.ReadAll(resp.Body)
				profile := &tree.Profile{}
				err = proto.Unmarshal(body, profile)
				Expect(err).ToNot(HaveOccurred())
			})
			It("supports collapsed format", func() {
				defer httpServer.Close()

				resp, err := http.Get(fmt.Sprintf("%s/render?query=%s&format=%s", httpServer.URL, url.QueryEscape(`app{foo="bar"}`), "collapsed"))
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(resp.Header.Get("Content-Disposition")).To(MatchRegexp(
					"^attachment; filename.+\\.collapsed.txt$",
				))
			})
		})
	})
})

var _ = Describe("render merge test", func() {
	var httpServer *httptest.Server
	testing.WithConfig(func(cfg **config.Config) {
		BeforeEach(func() {
			(*cfg).Server.APIBindAddr = ":10044"
			s, err := storage.New(storage.NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller))
			Expect(err).ToNot(HaveOccurred())
			e, _ := exporter.NewExporter(nil, nil)
			c, _ := New(Config{
				Configuration:           &(*cfg).Server,
				Storage:                 s,
				MetricsExporter:         e,
				Logger:                  logrus.New(),
				MetricsRegisterer:       prometheus.NewRegistry(),
				ExportedMetricsRegistry: prometheus.NewRegistry(),
				Notifier:                mockNotifier{},
				Adhoc:                   mockAdhocServer{},
			})
			h, _ := c.serverMux()
			httpServer = httptest.NewServer(h)
		})

		Context("/render", func() {
			It("handles merge requests", func() {
				defer httpServer.Close()

				resp, err := http.Post(httpServer.URL+"/merge", "application/json", reqBody(mergeRequest{
					AppName:  "app.cpu",
					Profiles: []string{"a", "b"},
				}))

				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var merged mergeResponse
				Expect(json.NewDecoder(resp.Body).Decode(&merged)).ToNot(HaveOccurred())
				Expect(merged.Validate()).ToNot(HaveOccurred())
			})
		})
	})
})

func reqBody(v interface{}) io.Reader {
	var b bytes.Buffer
	Expect(json.NewEncoder(&b).Encode(v)).ToNot(HaveOccurred())
	return &b
}
