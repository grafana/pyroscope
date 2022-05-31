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
	var cfg config.RemoteWrite

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetOutput(ioutil.Discard)

		noopHandler := func(w http.ResponseWriter, r *http.Request) {}

		remoteHandler = noopHandler
		localHandler = noopHandler
		payload = []byte("")
		endpoint = ""

		cfg.Address = ""
		cfg.AuthToken = ""
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

		cfg.Address = remoteServer.URL
		handler := remotewrite.NewTrafficShadower(logger, originalHandler, cfg)

		request, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(payload))
		response := httptest.NewRecorder()

		wg.Add(2)
		handler.ServeHTTP(response, request)
		wg.Wait()
	}

	It("sends same payload to both remote server and local handler", func() {
		payload = []byte("test")

		assertRequest := func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
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
			defer GinkgoRecover()
			Expect(r.URL.Query().Get("test")).To(Equal("123"))
		}

		remoteHandler = assertRequest
		localHandler = assertRequest

		run()
	})

	When("authKey is present", func() {
		BeforeEach(func() {
			cfg.AuthToken = "MY_KEY"
		})

		It("sends AuthKey to remote server", func() {
			remoteHandler = func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer " + cfg.AuthToken))
			}

			run()
		})
	})

	When("authKey is not present", func() {
		BeforeEach(func() {
			cfg.AuthToken = ""
		})

		It("doesnt send to remote server", func() {
			remoteHandler = func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				Expect(r.Header.Get("Authorization")).To(Equal(""))
			}

			run()
		})
	})

	When("when remote labels are added", func() {
		BeforeEach(func() {
			endpoint = "/?name=myapp"
			cfg.Tags = map[string]string{
				"my":      "tag",
				"nuestra": "tag",
			}
		})

		It("enhances the remote request with the labels", func() {
			assertRequest := func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				Expect(r.URL.Query().Get("name")).To(Equal(`myapp{my=tag,nuestra=tag}`))
			}

			remoteHandler = assertRequest
			run()
		})
	})
})
