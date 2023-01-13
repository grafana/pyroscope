package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

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

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		const testDataDir string = "testdata"
		const tlsCertificateFile string = "cert.pem"
		const tlsKeyFile string = "key.pem"
		Describe("HTTPS", func() {
			It("Should serve HTTPS when TLSCertificateFile and TLSKeyFile is defined",
				func() {
					defer GinkgoRecover()
					const addr = ":10046"
					(*cfg).Server.APIBindAddr = addr
					(*cfg).Server.TLSCertificateFile = filepath.Join(testDataDir, tlsCertificateFile)
					(*cfg).Server.TLSKeyFile = filepath.Join(testDataDir, tlsKeyFile)

					s, err := storage.New(
						storage.NewConfig(&(*cfg).Server),
						logrus.StandardLogger(),
						prometheus.NewRegistry(),
						new(health.Controller),
						storage.NoopApplicationMetadataService{},
					)
					Expect(err).ToNot(HaveOccurred())
					defer s.Close()
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
					c.dir = http.Dir(testDataDir)

					startController(c, "https", addr)

					tr := &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					}

					client := &http.Client{Transport: tr}
					resHTTPS, _ := client.Get(fmt.Sprintf("https://localhost%s", addr))
					resHTTP, _ := client.Get(fmt.Sprintf("http://localhost%s", addr))

					defer resHTTPS.Body.Close()
					defer resHTTP.Body.Close()
					defer c.Stop()

					Expect(resHTTPS.StatusCode).To(Equal(http.StatusOK))
					Expect(resHTTP.StatusCode).To(Equal(http.StatusBadRequest))
				},
			)
			It("Should serve HTTP when TLSCertificateFile & TLSKeyFile is undefined",
				func() {
					defer GinkgoRecover()
					const addr = ":10046"
					(*cfg).Server.APIBindAddr = addr

					s, err := storage.New(
						storage.NewConfig(&(*cfg).Server),
						logrus.StandardLogger(),
						prometheus.NewRegistry(),
						new(health.Controller),
						storage.NoopApplicationMetadataService{},
					)
					Expect(err).ToNot(HaveOccurred())
					defer s.Close()
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
					c.dir = http.Dir(testDataDir)

					startController(c, "http", addr)

					tr := &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					}

					client := &http.Client{Transport: tr}
					resHTTP, _ := client.Get(fmt.Sprintf("http://localhost%s", addr))
					_, errHTTPS := client.Get(fmt.Sprintf("https://localhost%s", addr))

					defer resHTTP.Body.Close()
					defer c.Stop()

					Expect(resHTTP.StatusCode).To(Equal(http.StatusOK))
					Expect(errHTTPS).To(HaveOccurred())
				},
			)
		})
	})
})

func startController(c *Controller, protocol string, addr string) {
	startSync := make(chan struct{})
	go func() {
		defer GinkgoRecover()
		err := c.StartSync(startSync)
		Expect(err).ToNot(HaveOccurred())
	}()
	<-startSync
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	httpClient := &http.Client{Transport: tr}
	var err error
	var res *http.Response
	for i := 0; i < 100; i++ {
		res, err = httpClient.Get(fmt.Sprintf("%s://localhost%s", protocol, addr))
		if err == nil && res.StatusCode == http.StatusOK {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	err = fmt.Errorf("failed to wait for server startup %v %v", err, res)
	Expect(err).ToNot(HaveOccurred())
}
