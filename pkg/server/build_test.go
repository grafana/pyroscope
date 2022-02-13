package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("/build", func() {
			It("works properly", func() {
				done := make(chan interface{})
				go func() {
					defer GinkgoRecover()

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
					httpServer := httptest.NewServer(h)
					defer httpServer.Close()

					res, err := http.Get(httpServer.URL + "/build")
					Expect(err).ToNot(HaveOccurred())
					Expect(res.StatusCode).To(Equal(200))

					b, err := io.ReadAll(res.Body)
					Expect(err).ToNot(HaveOccurred())

					actual := make(map[string]interface{})
					err = json.Unmarshal(b, &actual)

					Expect(err).ToNot(HaveOccurred())
					Expect(actual["goos"]).To(Equal(runtime.GOOS))

					close(done)
				}()
				Eventually(done, 2).Should(BeClosed())
			})
		})
	})
})
