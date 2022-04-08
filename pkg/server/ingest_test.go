package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

func readTestdataFile(name string) string {
	f, err := ioutil.ReadFile(name)
	Expect(err).ToNot(HaveOccurred())
	return string(f)
}

func pprofFormFromFile(name string, cfg map[string]*tree.SampleTypeConfig) (*multipart.Writer, *bytes.Buffer) {
	b, err := ioutil.ReadFile(name)
	Expect(err).ToNot(HaveOccurred())
	bw := &bytes.Buffer{}
	w := multipart.NewWriter(bw)
	fw, err := w.CreateFormFile("profile", "profile.pprof")
	Expect(err).ToNot(HaveOccurred())
	_, err = fw.Write(b)
	Expect(err).ToNot(HaveOccurred())
	if cfg != nil {
		jsonb, err := json.Marshal(cfg)
		Expect(err).ToNot(HaveOccurred())
		jw, err := w.CreateFormFile("sample_type_config", "sample_type_config.json")
		_, err = jw.Write(jsonb)
		Expect(err).ToNot(HaveOccurred())
	}
	err = w.Close()
	Expect(err).ToNot(HaveOccurred())
	return w, bw
}

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {
		BeforeEach(func() {
			(*cfg).Server.APIBindAddr = ":10043"
		})

		Describe("/ingest", func() {
			var buf *bytes.Buffer
			var format string
			var contentType string
			var name string
			var sleepDur time.Duration
			var expectedKey string
			headers := map[string]string{}
			expectedTree := "foo;bar 2\nfoo;baz 3\n"

			// this is an example of Shared Example pattern
			//   see https://onsi.github.io/ginkgo/#shared-example-patterns
			ItCorrectlyParsesIncomingData := func() {
				It("correctly parses incoming data", func() {
					done := make(chan interface{})
					go func() {
						defer GinkgoRecover()
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
						defer s.Close()

						st := testing.ParseTime("2020-01-01-01:01:00")
						et := testing.ParseTime("2020-01-01-01:01:10")

						u, _ := url.Parse(httpServer.URL + "/ingest")
						q := u.Query()
						if name == "" {
							name = "test.app{}"
						}
						q.Add("name", name)
						q.Add("from", strconv.Itoa(int(st.Unix())))
						q.Add("until", strconv.Itoa(int(et.Unix())))
						if format != "" {
							q.Add("format", format)
						}
						u.RawQuery = q.Encode()

						fmt.Println(u.String())

						req, err := http.NewRequest("POST", u.String(), buf)
						Expect(err).ToNot(HaveOccurred())
						if contentType == "" {
							contentType = "text/plain"
						}
						for k, v := range headers {
							req.Header.Set(k, v)
						}
						req.Header.Set("Content-Type", contentType)

						res, err := http.DefaultClient.Do(req)
						Expect(err).ToNot(HaveOccurred())
						Expect(res.StatusCode).To(Equal(200))

						if expectedKey == "" {
							expectedKey = name
						}
						sk, _ := segment.ParseKey(expectedKey)
						time.Sleep(sleepDur)
						gOut, err := s.Get(context.TODO(), &storage.GetInput{
							StartTime: st,
							EndTime:   et,
							Key:       sk,
						})
						Expect(gOut).ToNot(BeNil())
						Expect(err).ToNot(HaveOccurred())
						Expect(gOut.Tree).ToNot(BeNil())
						Expect(gOut.Tree.String()).To(Equal(expectedTree))

						close(done)
					}()
					Eventually(done, 2).Should(BeClosed())
				})
			}

			Context("default format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("foo;bar 2\nfoo;baz 3\n"))
					format = ""
					contentType = ""
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("lines format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("foo;bar\nfoo;bar\nfoo;baz\nfoo;baz\nfoo;baz\n"))
					format = "lines"
					contentType = ""
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("trie format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("\x00\x00\x01\x06foo;ba\x00\x02\x01r\x02\x00\x01z\x03\x00"))
					format = "trie"
					contentType = ""
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("tree format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("\x00\x00\x01\x03foo\x00\x02\x03bar\x02\x00\x03baz\x03\x00"))
					format = "tree"
					contentType = ""
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("trie format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("\x00\x00\x01\x06foo;ba\x00\x02\x01r\x02\x00\x01z\x03\x00"))
					format = ""
					contentType = "binary/octet-stream+trie"
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("tree format", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("\x00\x00\x01\x03foo\x00\x02\x03bar\x02\x00\x03baz\x03\x00"))
					format = ""
					contentType = "binary/octet-stream+tree"
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("name with tags", func() {
				BeforeEach(func() {
					buf = bytes.NewBuffer([]byte("foo;bar 2\nfoo;baz 3\n"))
					format = ""
					contentType = ""
					name = "test.app{foo=bar,baz=qux}"
				})

				ItCorrectlyParsesIncomingData()
			})

			Context("pprof", func() {
				BeforeEach(func() {
					format = ""
					sleepDur = 100 * time.Millisecond // prof data is not updated immediately with pprof
					name = "test.app{foo=bar,baz=qux}"
					expectedKey = "test.app.cpu{foo=bar,baz=qux}"
					expectedTree = readTestdataFile("./testdata/pprof-string.txt")
				})

				Context("default sample type config", func() { // this is used in integrations
					BeforeEach(func() {
						var w *multipart.Writer
						w, buf = pprofFormFromFile("../convert/testdata/cpu.pprof", nil)
						contentType = w.FormDataContentType()
					})

					ItCorrectlyParsesIncomingData()
				})

				Context("pprof format instead of content Type", func() { // this is described in docs
					BeforeEach(func() {
						format = "pprof"
						buf = bytes.NewBuffer([]byte(readTestdataFile("../convert/testdata/cpu.pprof")))
					})
					ItCorrectlyParsesIncomingData()
				})

				Context("custom sample type config", func() { // this is also described in docs
					BeforeEach(func() {
						var w *multipart.Writer
						w, buf = pprofFormFromFile("../convert/testdata/cpu.pprof", map[string]*tree.SampleTypeConfig{
							"samples": {
								Units:       "samples",
								DisplayName: "customName",
							},
						})
						contentType = w.FormDataContentType()
						expectedKey = "test.app.customName{foo=bar,baz=qux}"
					})

					ItCorrectlyParsesIncomingData()
				})
			})
		})
	})
})
