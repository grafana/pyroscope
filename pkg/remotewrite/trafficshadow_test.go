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

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetOutput(ioutil.Discard)

		remoteHandler = nil
		localHandler = nil
		payload = nil
		endpoint = ""
	})

	run := func() {
		originalHandler := mockHandler{handler: localHandler}
		remoteServer := httptest.NewServer(http.HandlerFunc(remoteHandler))

		handler := remotewrite.NewTrafficShadower(logger, originalHandler, config.RemoteWrite{
			Enabled:   true,
			Address:   remoteServer.URL,
			AuthToken: "",
		})

		request, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(payload))
		response := httptest.NewRecorder()

		wg.Add(2)
		handler.ServeHTTP(response, request)
		wg.Wait()
	}

	It("sends same payload to both remote server and local handler", func() {
		payload = []byte("test")
		endpoint = "/?test=123"

		assertRequest := func(w http.ResponseWriter, r *http.Request) {
			body, err := ioutil.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(Equal(payload))

			wg.Done()
		}

		remoteHandler = assertRequest
		localHandler = assertRequest

		run()
	})

	It("sends same query params to both remote server and local handler", func() {
		payload = []byte("")
		endpoint = "/?test=123"

		assertRequest := func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Query().Get("test")).To(Equal("123"))

			wg.Done()
		}

		remoteHandler = assertRequest
		localHandler = assertRequest

		run()
	})

	//	It("sends AuthKey to remote server", func() {
	//
	//	})
})
