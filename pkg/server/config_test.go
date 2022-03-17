package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("/config", func() {
			It("works properly", func() {
				done := make(chan interface{})
				go func() {
					defer GinkgoRecover()

					(*cfg).Server.APIBindAddr = ":10045"
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

					res, err := http.Get(httpServer.URL + "/status/config")
					Expect(err).ToNot(HaveOccurred())
					Expect(res.StatusCode).To(Equal(200))

					b, err := io.ReadAll(res.Body)
					Expect(err).ToNot(HaveOccurred())

					resp := configResponse{}
					err = json.Unmarshal(b, &resp)

					Expect(err).ToNot(HaveOccurred())

					config := config.Server{}
					err = yaml.Unmarshal([]byte(resp.Yaml), &config)

					Expect(err).ToNot(HaveOccurred())
					Expect(config.APIBindAddr).To(Equal((*cfg).Server.APIBindAddr))

					close(done)
				}()
				Eventually(done, 2).Should(BeClosed())
			})
			It("should mask secrets", func() {
				done := make(chan interface{})
				go func() {
					defer GinkgoRecover()

					const fakeSecret = "AB19123d08409123890y"
					(*cfg).Server.Auth.Github.ClientSecret = fakeSecret
					(*cfg).Server.Auth.Gitlab.ClientSecret = fakeSecret
					(*cfg).Server.Auth.Google.ClientSecret = fakeSecret
					(*cfg).Server.Auth.JWTSecret = fakeSecret
					(*cfg).Server.Auth.Internal.AdminUser.Password = fakeSecret
					const sensitive = "<sensitive>"

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

					res, err := http.Get(httpServer.URL + "/status/config")
					Expect(err).ToNot(HaveOccurred())
					Expect(res.StatusCode).To(Equal(200))

					b, err := io.ReadAll(res.Body)
					Expect(err).ToNot(HaveOccurred())

					resp := configResponse{}
					err = json.Unmarshal(b, &resp)

					Expect(err).ToNot(HaveOccurred())

					config := config.Server{}
					err = yaml.Unmarshal([]byte(resp.Yaml), &config)

					Expect(err).ToNot(HaveOccurred())
					Expect(config.Auth.Github.ClientSecret).To(Equal(sensitive))
					Expect(config.Auth.Gitlab.ClientSecret).To(Equal(sensitive))
					Expect(config.Auth.Google.ClientSecret).To(Equal(sensitive))
					Expect(config.Auth.JWTSecret).To(Equal(sensitive))
					Expect(config.Auth.Internal.AdminUser.Password).To(Equal(sensitive))

					close(done)
				}()
				Eventually(done, 2).Should(BeClosed())
			})
		})
	})
})
