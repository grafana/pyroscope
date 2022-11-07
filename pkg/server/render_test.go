package server

import (
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
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {
	var httpServer *httptest.Server
	var s *storage.Storage

	testing.WithConfig(func(cfg **config.Config) {
		BeforeEach(func() {
			(*cfg).Server.APIBindAddr = ":10044"
			var err error
			s, err = storage.New(
				storage.NewConfig(&(*cfg).Server),
				logrus.StandardLogger(),
				prometheus.NewRegistry(),
				new(health.Controller),
				storage.NoopApplicationMetadataService{},
			)
			Expect(err).ToNot(HaveOccurred())
			e, _ := exporter.NewExporter(nil, nil)
			c, _ := New(Config{
				Configuration:           &(*cfg).Server,
				Storage:                 s,
				Ingester:                parser.New(logrus.StandardLogger(), s, e),
				Logger:                  logrus.New(),
				MetricsRegisterer:       prometheus.NewRegistry(),
				ExportedMetricsRegistry: prometheus.NewRegistry(),
				Notifier:                mockNotifier{},
			})
			h, _ := c.serverMux()
			httpServer = httptest.NewServer(h)
		})
		JustAfterEach(func() {
			s.Close()
			httpServer.Close()
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
