package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/config"
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
					s, err := storage.New(&(*cfg).Server)
					Expect(err).ToNot(HaveOccurred())
					c, _ := New(&(*cfg).Server, s)
					go func() {
						defer GinkgoRecover()
						c.Start()
					}()

					retryUntilServerIsUp("http://localhost:10045/")

					res, err := http.Get("http://localhost:10045/config")
					Expect(err).ToNot(HaveOccurred())
					Expect(res.StatusCode).To(Equal(200))

					b, err := ioutil.ReadAll(res.Body)
					Expect(err).ToNot(HaveOccurred())

					actual := make(map[string]interface{})
					err = json.Unmarshal(b, &actual)

					Expect(err).ToNot(HaveOccurred())
					Expect(actual["APIBindAddr"]).To(Equal((*cfg).Server.APIBindAddr))

					c.Stop()
					close(done)
				}()
				Eventually(done, 2).Should(BeClosed())
			})
		})
	})
})
