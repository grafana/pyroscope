package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

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

const assetAtCompressionThreshold, assetLtCompressionThreshold = "AssetAtCompressionThreshold", "AssetLTCompressionThreshold"

var tempAssetDir *testing.TmpDirectory

var _ = BeforeSuite(func() {
	tempAssetDir = testing.TmpDirSync()
	os.Mkdir(filepath.Join(tempAssetDir.Path, "assets"), 0755)
	os.WriteFile(filepath.Join(tempAssetDir.Path, "assets", assetLtCompressionThreshold), make([]byte, gzHTTPCompressionThreshold-1), 0644)
	os.WriteFile(filepath.Join(tempAssetDir.Path, "assets", assetAtCompressionThreshold), make([]byte, gzHTTPCompressionThreshold), 0644)
})

var _ = AfterSuite(func() {
	tempAssetDir.Close()
})

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		DescribeTable("compress assets",
			func(filename string, uncompressed bool) {
				done := make(chan interface{})
				go func(filename string, uncompressed bool) {
					defer GinkgoRecover()

					(*cfg).Server.APIBindAddr = ":10045"
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
					c.dir = http.Dir(tempAssetDir.Path)
					h, _ := c.getHandler()
					httpServer := httptest.NewServer(h)
					defer httpServer.Close()

					res, err := http.Get(fmt.Sprintf("%s/assets/%s", httpServer.URL, filename))
					Expect(err).ToNot(HaveOccurred())
					Expect(res.StatusCode).To(Equal(http.StatusOK))
					Expect(res.Uncompressed).To(Equal(uncompressed))

					close(done)
				}(filename, uncompressed)
				Eventually(done, 2).Should(BeClosed())
			},
			Entry("Should compress assets greater than or equal to threshold", assetAtCompressionThreshold, true),
			Entry("Should not compress assets less than threshold", assetLtCompressionThreshold, false),
		)
	})
})
