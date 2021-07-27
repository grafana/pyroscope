package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Context("/render", func() {
			It("supports name and query parameters", func() {
				var httpServer *httptest.Server
				(*cfg).Server.APIBindAddr = ":10044"
				s, err := storage.New(&(*cfg).Server)
				Expect(err).ToNot(HaveOccurred())
				c, _ := New(&(*cfg).Server, s, s, logrus.New())
				h, _ := c.mux()
				httpServer = httptest.NewServer(h)
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
		})
	})
})
