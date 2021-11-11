package admin_test

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
)

// The idea for these tests is to have client and server
// communicating over the unix socket
var _ = Describe("integration", func() {
	var server *admin.Server
	var httpC http.Client
	var socketAddr string
	var cleanup func()

	BeforeEach(func() {
		logger, _ := test.NewNullLogger()

		// the bind syscall will create the socket file
		// so we first create a temporary directory
		// and pass a well-known file name
		// that way tests can be run concurrently
		clean, dir := genRandomDir()
		cleanup = clean
		socketAddr = dir + "/pyroscope.tmp.sock"

		httpServer, err := admin.NewUdsHTTPServer(socketAddr)
		must(err)

		svc := admin.NewService(mockAppsGetter{})
		ctrl := admin.NewController(logger, svc)
		s, err := admin.NewServer(logger, ctrl, httpServer)
		must(err)
		server = s

		httpC = newHttpClient(socketAddr)
	})

	AfterEach(func() {
		cleanup()
	})

	It("works", func() {
		go func() {
			defer GinkgoRecover()

			err := server.Start()
			if err != nil {
				Expect(err).To(BeNil())
			}
		}()

		waitUntilServerIsReady(socketAddr)

		resp, err := httpC.Get("http://dummy/health")
		Expect(err).To(BeNil())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})

// TODO:
// for all effects this client can be reused
func newHttpClient(socketAddr string) http.Client {
	return http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketAddr)
			},
		},
	}
}

func genRandomDir() (func(), string) {
	// the bind syscall will create the socket file
	// so we first create a temporary directory
	// and pass a well-known file name
	// that way tests can be run concurrently
	dir, err := ioutil.TempDir("", "")
	must(err)

	return func() {
		os.RemoveAll(dir)
	}, dir
}
