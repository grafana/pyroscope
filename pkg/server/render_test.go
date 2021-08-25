package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {

	query := "pyroscope.server.alloc_objects{}"
	formedBody := &RenderDiffParams{
		Query: &query,
		From:  "now-1h",
		Until: "now",
		Left: RenderTreeParams{
			From:  "now-1h",
			Until: "now",
		},

		Right: RenderTreeParams{
			From:  "now-30m",
			Until: "now",
		},
		Format: "json",
	}

	formedBodyJSON, err := json.Marshal(formedBody)
	if err != nil {
		panic(err)
	}

	malFormedBody := &RenderDiffParams{

		Until: "now",
		Left: RenderTreeParams{
			From:  "now-1h",
			Until: "now",
		},

		Right: RenderTreeParams{
			From:  "",
			Until: "now",
		},
	}

	malFormedBodyJSON, err := json.Marshal(malFormedBody)
	if err != nil {
		panic(err)
	}

	testing.WithConfig(func(cfg **config.Config) {
		Context("/render", func() {
			It("supports name and query parameters", func() {
				var httpServer *httptest.Server
				(*cfg).Server.APIBindAddr = ":10044"
				s, err := storage.New(&(*cfg).Server, prometheus.NewRegistry())
				Expect(err).ToNot(HaveOccurred())
				c, _ := New(&(*cfg).Server, s, s, logrus.New(), prometheus.NewRegistry())
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

				// POST Method
				resp, err = http.Post(fmt.Sprintf("%s/render-diff", httpServer.URL), "application/json", bytes.NewBuffer(formedBodyJSON))
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				resp, err = http.Post(fmt.Sprintf("%s/render-diff", httpServer.URL), "application/json", bytes.NewBuffer(malFormedBodyJSON))
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			})
		})
	})
})
