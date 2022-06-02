package remotewrite_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/remotewrite"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
	"github.com/sirupsen/logrus"
)

var _ = Describe("TrafficShadower", func() {
	var logger *logrus.Logger

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetOutput(ioutil.Discard)
	})

	Context("happy path", func() {
		var remoteHandler http.HandlerFunc
		var wg sync.WaitGroup
		var cfg config.RemoteWrite
		var pi parser.PutInput

		BeforeEach(func() {
			remoteHandler = func(w http.ResponseWriter, r *http.Request) {}

			cfg.Address = ""
			cfg.AuthToken = ""
		})

		run := func() {
			remoteServer := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					remoteHandler(w, r)
					wg.Done()
				}),
			)

			cfg.Address = remoteServer.URL
			client := remotewrite.NewClient(logger, cfg)

			wg.Add(1)
			client.Put(context.TODO(), &pi)
			wg.Wait()
		}

		It("sends request to remote", func() {
			pi = parser.PutInput{
				Key: segment.NewKey(map[string]string{
					"__name__": "myapp",
				}),

				StartTime:       attime.Parse("1654110240"),
				EndTime:         attime.Parse("1654110250"),
				SampleRate:      100,
				SpyName:         "gospy",
				Units:           metadata.SamplesUnits,
				AggregationType: metadata.SumAggregationType,
			}

			remoteHandler = func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()

				Expect(r.URL.Query().Get("name")).To(Equal("myapp{}"))
				Expect(r.URL.Query().Get("from")).To(Equal("1654110240"))
				Expect(r.URL.Query().Get("until")).To(Equal("1654110250"))
				Expect(r.URL.Query().Get("sampleRate")).To(Equal("100"))
				Expect(r.URL.Query().Get("spyName")).To(Equal("gospy"))
				Expect(r.URL.Query().Get("units")).To(Equal("samples"))
				Expect(r.URL.Query().Get("aggregationType")).To(Equal("sum"))
			}

			run()
		})
	})

	Context("sad path", func() {
		When("it can't convert PutInput a http.Request", func() {
			It("fails with ErrConvertPutInputToRequest", func() {
				client := remotewrite.NewClient(logger, config.RemoteWrite{
					Address: "%%",
				})
				pi := parser.PutInput{}

				err := client.Put(context.TODO(), &pi)
				Expect(err).To(MatchError(remotewrite.ErrConvertPutInputToRequest))
			})
		})

		When("it can't send to remote", func() {
			It("fails with ErrMakingRequest", func() {
				client := remotewrite.NewClient(logger, config.RemoteWrite{
					Address: "//inexistent-url",
				})
				pi := parser.PutInput{
					Key: segment.NewKey(map[string]string{
						"__name__": "myapp",
					}),

					StartTime:       attime.Parse("1654110240"),
					EndTime:         attime.Parse("1654110250"),
					SampleRate:      100,
					SpyName:         "gospy",
					Units:           metadata.SamplesUnits,
					AggregationType: metadata.SumAggregationType,
				}

				err := client.Put(context.TODO(), &pi)
				Expect(err).To(MatchError(remotewrite.ErrMakingRequest))
			})
		})

		When("response is not within the 2xx range", func() {
			It("fails with ErrNotOkResponse", func() {
				remoteServer := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(500)
					}),
				)

				client := remotewrite.NewClient(logger, config.RemoteWrite{
					Address: remoteServer.URL,
				})
				pi := parser.PutInput{
					Key: segment.NewKey(map[string]string{
						"__name__": "myapp",
					}),

					StartTime:       attime.Parse("1654110240"),
					EndTime:         attime.Parse("1654110250"),
					SampleRate:      100,
					SpyName:         "gospy",
					Units:           metadata.SamplesUnits,
					AggregationType: metadata.SumAggregationType,
				}

				err := client.Put(context.TODO(), &pi)
				Expect(err).To(MatchError(remotewrite.ErrNotOkResponse))
			})
		})
	})
})
