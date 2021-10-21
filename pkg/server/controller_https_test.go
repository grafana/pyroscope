package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
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

					s, err := storage.New(&(*cfg).Server, logrus.StandardLogger(), prometheus.NewRegistry())
					Expect(err).ToNot(HaveOccurred())

					c, _ := New(&(*cfg).Server, s, s, logrus.New(), prometheus.NewRegistry())
					c.dir = http.Dir(testDataDir)

					go c.Start()
					// TODO: Wait for start .There's possibly a better way of doing this
					time.Sleep(50 * time.Millisecond)
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

					s, err := storage.New(&(*cfg).Server, logrus.StandardLogger(), prometheus.NewRegistry())
					Expect(err).ToNot(HaveOccurred())

					c, _ := New(&(*cfg).Server, s, s, logrus.New(), prometheus.NewRegistry())
					c.dir = http.Dir(testDataDir)

					go c.Start()
					time.Sleep(50 * time.Millisecond)
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
