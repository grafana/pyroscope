//go:build !windows
// +build !windows

package admin_test

import (
	"io/ioutil"
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
	var server *admin.Server
	var httpC *http.Client
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

		// create the server
		http, err := admin.NewHTTPOverUDSClient(socketAddr, admin.WithTimeout(time.Millisecond*10))
		Expect(err).ToNot(HaveOccurred())

		httpServer, err := admin.NewUdsHTTPServer(socketAddr, http)
		Expect(err).ToNot(HaveOccurred())
		svc := admin.NewService(mockStorage{})
		ctrl := admin.NewController(logger, svc)
		s, err := admin.NewServer(logger, ctrl, httpServer)
		Expect(err).ToNot(HaveOccurred())
		server = s

		// create the client
		httpClient, err := admin.NewHTTPOverUDSClient(socketAddr)
		Expect(err).ToNot(HaveOccurred())
		httpC = httpClient

		go (func() {
			defer GinkgoRecover()
			// we don't care if the server is closed
			_ = server.Start()
		})()
		waitUntilServerIsReady(socketAddr)
	})

	AfterEach(func() {
		server.Stop()
		cleanup()
	})

	It("works", func() {
		//		go func() {
		//			defer GinkgoRecover()
		//
		//			err := server.Start()
		//			Expect(err).ToNot(HaveOccurred())
		//		}()

		resp, err := httpC.Get("http://dummy/health")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})

func genRandomDir() (func(), string) {
	// the bind syscall will create the socket file
	// so we first create a temporary directory
	// and pass a well-known file name
	// that way tests can be run concurrently
	dir, err := ioutil.TempDir("", "")
	Expect(err).ToNot(HaveOccurred())

	return func() {
		os.RemoveAll(dir)
	}, dir
}
