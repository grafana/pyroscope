package remotewrite_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/convert/profile"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/remotewrite"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

// TODO(eh-am): clean up these bunch of putInput

var _ = Describe("TrafficShadower", func() {
	var logger *logrus.Logger

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetOutput(ioutil.Discard)
	})

	Context("happy path", func() {
		var remoteHandler http.HandlerFunc
		var wg sync.WaitGroup
		var cfg config.RemoteWriteTarget
		var in ingestion.IngestInput

		BeforeEach(func() {
			remoteHandler = func(w http.ResponseWriter, r *http.Request) {}

			cfg.Address = ""
			cfg.AuthToken = ""
			cfg.Tags = make(map[string]string)
		})

		run := func() {
			remoteServer := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					remoteHandler(w, r)
					wg.Done()
				}),
			)

			cfg.Address = remoteServer.URL
			client := remotewrite.NewClient(logger, prometheus.NewRegistry(), "targetName", cfg)

			wg.Add(1)
			client.Ingest(context.TODO(), &in)
			wg.Wait()
		}

		It("sends request to remote", func() {
			in = ingestion.IngestInput{
				Metadata: ingestion.Metadata{
					Key: segment.NewKey(map[string]string{
						"__name__": "myapp",
					}),

					StartTime:       attime.Parse("1654110240"),
					EndTime:         attime.Parse("1654110250"),
					SampleRate:      100,
					SpyName:         "gospy",
					Units:           metadata.SamplesUnits,
					AggregationType: metadata.SumAggregationType,
				},

				Profile: new(profile.RawProfile),
				Format:  ingestion.FormatGroups,
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
				Expect(r.URL.Query().Get("format")).To(Equal(string(ingestion.FormatGroups)))
			}

			run()
		})

		When("auth is configured", func() {
			BeforeEach(func() {
				cfg.AuthToken = "myauthtoken"
			})

			It("sets the Authorization header", func() {
				in = ingestion.IngestInput{
					Metadata: ingestion.Metadata{
						Key: segment.NewKey(map[string]string{
							"__name__": "myapp",
							"my":       "tag",
						}),

						StartTime:       attime.Parse("1654110240"),
						EndTime:         attime.Parse("1654110250"),
						SampleRate:      100,
						SpyName:         "gospy",
						Units:           metadata.SamplesUnits,
						AggregationType: metadata.SumAggregationType,
					},

					Profile: new(profile.RawProfile),
				}

				remoteHandler = func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()

					Expect(r.Header.Get("Authorization")).To(Equal("Bearer myauthtoken"))
				}

				run()
			})
		})

		When("tags are configured", func() {
			BeforeEach(func() {
				cfg.Tags = make(map[string]string)

				cfg.Tags["minha"] = "tag"
				cfg.Tags["nuestra"] = "etiqueta"
			})

			It("enhances the app name with tags", func() {
				in = ingestion.IngestInput{
					Metadata: ingestion.Metadata{
						Key: segment.NewKey(map[string]string{
							"__name__": "myapp",
							"my":       "tag",
						}),

						StartTime:       attime.Parse("1654110240"),
						EndTime:         attime.Parse("1654110250"),
						SampleRate:      100,
						SpyName:         "gospy",
						Units:           metadata.SamplesUnits,
						AggregationType: metadata.SumAggregationType,
					},

					Profile: new(profile.RawProfile),
				}

				remoteHandler = func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()

					key, err := segment.ParseKey(r.URL.Query().Get("name"))
					Expect(err).NotTo(HaveOccurred())

					Expect(key).To(Equal(
						segment.NewKey(map[string]string{
							"__name__": "myapp",
							"my":       "tag",
							"minha":    "tag",
							"nuestra":  "etiqueta",
						}),
					))
				}

				run()
			})
		})
	})

	Context("sad path", func() {
		When("it can't convert PutInput into a http.Request", func() {
			It("fails with ErrConvertPutInputToRequest", func() {
				client := remotewrite.NewClient(logger, prometheus.NewRegistry(), "targetName",
					config.RemoteWriteTarget{
						Address: "%%",
					})
				in := ingestion.IngestInput{
					Metadata: ingestion.Metadata{
						Key: segment.NewKey(map[string]string{
							"__name__": "myapp",
						}),
					},
					Profile: new(profile.RawProfile),
				}

				err := client.Ingest(context.TODO(), &in)
				Expect(err).To(MatchError(remotewrite.ErrConvertPutInputToRequest))
			})
		})

		When("it can't send to remote", func() {
			It("fails with ErrMakingRequest", func() {
				client := remotewrite.NewClient(logger, prometheus.NewRegistry(), "targetName",
					config.RemoteWriteTarget{
						Address: "//inexistent-url",
					})
				in := ingestion.IngestInput{
					Metadata: ingestion.Metadata{
						Key: segment.NewKey(map[string]string{
							"__name__": "myapp",
							"my":       "tag",
						}),

						StartTime:       attime.Parse("1654110240"),
						EndTime:         attime.Parse("1654110250"),
						SampleRate:      100,
						SpyName:         "gospy",
						Units:           metadata.SamplesUnits,
						AggregationType: metadata.SumAggregationType,
					},

					Profile: new(profile.RawProfile),
				}

				err := client.Ingest(context.TODO(), &in)
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

				client := remotewrite.NewClient(logger, prometheus.NewRegistry(), "targetName",
					config.RemoteWriteTarget{
						Address: remoteServer.URL,
					})
				in := ingestion.IngestInput{
					Metadata: ingestion.Metadata{
						Key: segment.NewKey(map[string]string{
							"__name__": "myapp",
							"my":       "tag",
						}),

						StartTime:       attime.Parse("1654110240"),
						EndTime:         attime.Parse("1654110250"),
						SampleRate:      100,
						SpyName:         "gospy",
						Units:           metadata.SamplesUnits,
						AggregationType: metadata.SumAggregationType,
					},

					Profile: new(profile.RawProfile),
				}

				err := client.Ingest(context.TODO(), &in)
				Expect(err).To(MatchError(remotewrite.ErrNotOkResponse))
			})
		})
	})
})
