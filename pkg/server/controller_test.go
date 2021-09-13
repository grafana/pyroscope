package server

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

const assetAtCompressionThreshold, assetLtCompressionThreshold = "AssetAtCompressionThreshold", "AssetLTCompressionThreshold"

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		var temp_asset_dir *testing.TmpDirectory
		BeforeSuite(func() {
			temp_asset_dir = testing.TmpDirSync()
			ioutil.WriteFile(filepath.Join(temp_asset_dir.Path, assetLtCompressionThreshold), make([]byte, gzHttpCompressionThreshold-1), 0644)
			ioutil.WriteFile(filepath.Join(temp_asset_dir.Path, assetAtCompressionThreshold), make([]byte, gzHttpCompressionThreshold), 0644)
		})
		AfterSuite(func() {
			temp_asset_dir.Close()
		})
		DescribeTable("compress assets",
			func(filename string, uncompressed bool) {
				done := make(chan interface{})
				go func(filename string, uncompressed bool) {
					defer GinkgoRecover()

					(*cfg).Server.APIBindAddr = ":10045"
					s, err := storage.New(&(*cfg).Server, prometheus.NewRegistry())
					Expect(err).ToNot(HaveOccurred())
					c, _ := New(&(*cfg).Server, s, s, logrus.New(), prometheus.NewRegistry())
					c.dir = http.Dir(temp_asset_dir.Path)
					h, _ := c.getHandler()
					httpServer := httptest.NewServer(h)
					defer httpServer.Close()

					res, err := http.Get(fmt.Sprintf("%s/%s", httpServer.URL, filename))
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
