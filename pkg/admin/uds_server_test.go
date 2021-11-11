package admin_test

import (
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/admin"
)

type mockHandler struct{}

func (m mockHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

var _ = Describe("UDS Server", func() {
	var (
		socketAddr string
	)

	When("passed an invalid socket address", func() {
		It("should give an error", func() {
			_, err := admin.NewUdsHTTPServer("")

			// TODO
			// check which kind of errors we got
			Expect(err).ToNot(BeNil())

			_ = socketAddr
		})
	})

	When("passed an already binded socket address", func() {
		When("that socket responds", func() {
			It("should return an error", func() {
				cleanup, dir := genRandomDir()
				defer cleanup()

				socketAddr = dir + "/pyroscope.sock"

				// create server 1
				_, err := admin.NewUdsHTTPServer(socketAddr)
				must(err)

				// create server 2
				_, err = admin.NewUdsHTTPServer(socketAddr)
				Expect(err).ToNot(BeNil())
			})
		})

		When("that socket does not respond", func() {
			It("should take over that socket", func() {
				cleanup, dir := genRandomDir()
				defer cleanup()

				socketAddr = dir + "/pyroscope.sock"

				// create server 1
				server, err := admin.NewUdsHTTPServer(socketAddr)
				must(err)

				go func() {
					server.Start(http.NewServeMux())
				}()

				// create server 2
				_, err = admin.NewUdsHTTPServer(socketAddr)
				Expect(err).ToNot(BeNil())
			})
		})
	})
})
