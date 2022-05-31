package remotewrite_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/remotewrite"
	"github.com/sirupsen/logrus"
)

type mockHandler struct {
	handler http.HandlerFunc
}

func (m mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.handler(w, r)
}

var _ = Describe("TrafficShadower", func() {
	var logger *logrus.Logger
	var remoteHandler http.HandlerFunc
	var localHandler http.HandlerFunc
	var payload []byte
	var endpoint string
	var wg sync.WaitGroup
	var authToken string

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetOutput(ioutil.Discard)

		noopHandler := func(w http.ResponseWriter, r *http.Request) {}

		remoteHandler = noopHandler
		localHandler = noopHandler
		payload = []byte("")
		endpoint = ""
		authToken = ""
	})

	run := func() {
		originalHandler := mockHandler{handler: func(w http.ResponseWriter, r *http.Request) {
			localHandler(w, r)
			wg.Done()
		}}

		remoteServer := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				remoteHandler(w, r)
				wg.Done()
			}),
		)

		handler := remotewrite.NewTrafficShadower(logger, originalHandler, config.RemoteWriteCfg{
			Address:   remoteServer.URL,
			AuthToken: authToken,
		})

		request, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(payload))
		response := httptest.NewRecorder()

		wg.Add(2)
		handler.ServeHTTP(response, request)
		wg.Wait()
	}

	It("sends same payload to both remote server and local handler", func() {
		payload = []byte("test")

		assertRequest := func(w http.ResponseWriter, r *http.Request) {
			body, err := ioutil.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(Equal(payload))
		}

		remoteHandler = assertRequest
		localHandler = assertRequest

		run()
	})

	It("sends same query params to both remote server and local handler", func() {
		endpoint = "/?test=123"

		assertRequest := func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Query().Get("test")).To(Equal("123"))
		}

		remoteHandler = assertRequest
		localHandler = assertRequest

		run()
	})

	Context("When authKey is present", func() {
		It("sends AuthKey to remote server", func() {
			authToken = "MY_KEY"

			remoteHandler = func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer " + authToken))
			}

			run()
		})
	})

	Context("When authKey is not present", func() {
		It("doesnt send to remote server", func() {
			authToken = ""

			remoteHandler = func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Authorization")).To(Equal(""))
			}

			run()
		})
	})
})
