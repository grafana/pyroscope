package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

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
				Ingester:                parser.New(logrus.StandardLogger(), s, e),
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
