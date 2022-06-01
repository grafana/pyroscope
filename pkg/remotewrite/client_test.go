package remotewrite_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/remotewrite"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/sirupsen/logrus"
)

var _ = Describe("TrafficShadower", func() {
	var logger *logrus.Logger
	var remoteHandler http.HandlerFunc
	var wg sync.WaitGroup
	var cfg remotewrite.RemoteWriteConfig
	var pi parser.PutInput

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetOutput(ioutil.Discard)

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
		client.Put(context.TODO(), pi)
		wg.Wait()
	}

	It("sends request to remote", func() {
		pi = parser.PutInput{
			Key: segment.NewKey(map[string]string{
				"__name__": "myapp",
			}),
		}

		assertRequest := func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()

			Expect(r.URL.Query().Get("name")).To(Equal("myapp{}"))
		}

		remoteHandler = assertRequest

		run()
	})

})
