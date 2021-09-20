package server

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

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
const tlsCert = 
`-----BEGIN CERTIFICATE-----
MIICpzCCAi6gAwIBAgIUVyJRQWSwWAdra4ndkDYGQ36wnBUwCgYIKoZIzj0EAwIw
gYoxCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJEQzELMAkGA1UEBwwCREMxFTATBgNV
BAoMDHB5cm9zY29wZS5pbzEVMBMGA1UECwwMcHlyb3Njb3BlLmlvMRIwEAYDVQQD
DAlsb2NhbGhvc3QxHzAdBgkqhkiG9w0BCQEWEGRldkBweXJvc2NvcGUuaW8wHhcN
MjEwOTIwMTMwMDI0WhcNMzEwOTE4MTMwMDI0WjCBijELMAkGA1UEBhMCVVMxCzAJ
BgNVBAgMAkRDMQswCQYDVQQHDAJEQzEVMBMGA1UECgwMcHlyb3Njb3BlLmlvMRUw
EwYDVQQLDAxweXJvc2NvcGUuaW8xEjAQBgNVBAMMCWxvY2FsaG9zdDEfMB0GCSqG
SIb3DQEJARYQZGV2QHB5cm9zY29wZS5pbzB2MBAGByqGSM49AgEGBSuBBAAiA2IA
BBiXAzwxT5591cwYgExG5WTO4LkNua9fexZ595sHgoTs+l13hlmJxdFDaVaasf7W
S/A+8hQO8CLEvAHVNPhXiSE7yQyLRc7kZNxdKXVFH/6RyErWL32XwD/kOKhtzr9T
CqNTMFEwHQYDVR0OBBYEFB8iItOC7tw8Vt4WX8AWIWoTTmTlMB8GA1UdIwQYMBaA
FB8iItOC7tw8Vt4WX8AWIWoTTmTlMA8GA1UdEwEB/wQFMAMBAf8wCgYIKoZIzj0E
AwIDZwAwZAIwQ6kqjamnb73dM1ARyA1VfE0ZMKHPP/bX7t1GnjT9bQI5YtsL6txT
Tsw7f3UVBQ9YAjBEy/MSnZ1TEyMb2jr2ItkaBRImuuko4Ksc1u5APiLncOmUJm+2
KanShH9fNMKOs8w=
-----END CERTIFICATE-----
`
const tlsKey = 
`-----BEGIN EC PARAMETERS-----
BgUrgQQAIg==
-----END EC PARAMETERS-----
-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDCtJtoa5ezMGl5vOm7F2wJfBnceiLIapTZEZcdMBdp3nuWXyZ3ENbIF
wbbvIrioZnagBwYFK4EEACKhZANiAAQYlwM8MU+efdXMGIBMRuVkzuC5DbmvX3sW
efebB4KE7Ppdd4ZZicXRQ2lWmrH+1kvwPvIUDvAixLwB1TT4V4khO8kMi0XO5GTc
XSl1RR/+kchK1i99l8A/5Diobc6/Uwo=
-----END EC PRIVATE KEY-----
`

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		var tlsCertificateFile string
		var tlsCertificateKeyFile string
		var temp_asset_dir *testing.TmpDirectory

		BeforeSuite(func() {
			temp_asset_dir = testing.TmpDirSync()
			ioutil.WriteFile(filepath.Join(temp_asset_dir.Path, assetLtCompressionThreshold), make([]byte, gzHttpCompressionThreshold-1), 0644)
			ioutil.WriteFile(filepath.Join(temp_asset_dir.Path, assetAtCompressionThreshold), make([]byte, gzHttpCompressionThreshold), 0644)
			tlsCertificateFile = filepath.Join(temp_asset_dir.Path, "tlsCert")
			tlsCertificateKeyFile = filepath.Join(temp_asset_dir.Path, "tlsKey")
			ioutil.WriteFile(filepath.Join(temp_asset_dir.Path, "tlsCert"), []byte(tlsCert), 0644)
			ioutil.WriteFile(filepath.Join(temp_asset_dir.Path, "tlsKey"), []byte(tlsKey), 0644)
			ioutil.WriteFile(filepath.Join(temp_asset_dir.Path, "index.html"), []byte("<!DOCTYPE html>"), 0644)
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
		Describe("HTTPS", func() {
			It("Should serve HTTPS when TLSCertificateFile and TLSCertificateKeyFile is defined",
				func() {
					defer GinkgoRecover()
					const addr = ":10046"
					(*cfg).Server.APIBindAddr = addr
					(*cfg).Server.TLSCertificateFile = tlsCertificateFile
					(*cfg).Server.TLSCertificateKeyFile = tlsCertificateKeyFile

					s, err := storage.New(&(*cfg).Server, prometheus.NewRegistry())
					Expect(err).ToNot(HaveOccurred())

					c, _ := New(&(*cfg).Server, s, s, logrus.New(), prometheus.NewRegistry())
					c.dir = http.Dir(temp_asset_dir.Path)

					go c.Start()
					time.Sleep(50 * time.Millisecond) // TODO: There's possibly a better way of doing this
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
			It("Should serve HTTP when TLSCertificateFile & TLSCertificateKeyFile is undefined",
				func() {
					defer GinkgoRecover()
					const addr = ":10046"
					(*cfg).Server.APIBindAddr = addr

					s, err := storage.New(&(*cfg).Server, prometheus.NewRegistry())
					Expect(err).ToNot(HaveOccurred())

					c, _ := New(&(*cfg).Server, s, s, logrus.New(), prometheus.NewRegistry())
					c.dir = http.Dir(temp_asset_dir.Path)

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
