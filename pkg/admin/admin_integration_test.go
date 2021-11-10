package admin_test

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
)

// The idea for these tests is to have client and server
// communicating over the unix socket
var _ = Describe("integration", func() {
	var server *admin.AdminServer
	var httpC http.Client
	var socketAddr string

	BeforeEach(func() {
		logger, _ := test.NewNullLogger()

		// the bind syscall will create the socket file
		// so we first create a temporary directory
		// and pass a well-known file name
		// that way tests can be run concurrently
		dir, err := ioutil.TempDir("", "")
		socketAddr = dir + "/pyroscope.tmp.sock"
		must(err)

		cfg := admin.Config{SocketAddr: socketAddr, Log: logger}

		svc := admin.NewService(mockAppsGetter{})
		ctrl := admin.NewController(logger, svc)
		server, err = admin.NewServer(cfg, ctrl)
		must(err)

		httpC = newHttpClient(socketAddr)
	})

	AfterEach(func() {
		os.Remove(socketAddr)
	})

	It("works", func() {
		go func() {
			defer GinkgoRecover()

			err := server.Start()
			if err != nil {
				Expect(err).To(BeNil())
			}
		}()

		// TODO
		// for some reason it takes some time until server is ready
		// we could add retries?
		time.Sleep(time.Millisecond * 10)

		resp, err := httpC.Get("http://dummy/check")
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
