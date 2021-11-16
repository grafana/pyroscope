//go:build !windows
// +build !windows

package admin_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
)

var _ = Describe("client", func() {
	fastTimeout := time.Millisecond * 100
	var (
		cleanup    func()
		socketAddr string
		server     *admin.UdsHTTPServer
		handler    *http.ServeMux
	)

	BeforeEach(func() {
		c, dir := genRandomDir()
		socketAddr = dir + "/pyroscope.sock"
		handler = http.NewServeMux()

		cleanup = c
	})

	JustBeforeEach(func() {
		s, err := admin.NewUdsHTTPServer(socketAddr, createHttpClientWithFastTimeout(socketAddr))
		Expect(err).ToNot(HaveOccurred())
		server = s
		go server.Start(handler)
		// waitUntilServerIsReady(socketAddr)
	})

	AfterEach(func() {
		server.Stop()
		cleanup()
	})

	// this test isn't super useful since this is already tested in the actual http client
	// but I wanted to test instantiaton error
	// without requiring dependency injection
	Context("when socket address is empty", func() {
		It("fails", func() {
			_, err := admin.NewClient("", fastTimeout)
			Expect(err).To(MatchError(admin.ErrHTTPClientCreation))
		})
	})

	Describe("GetAppNames", func() {
		Context("when server returns just fine", func() {
			BeforeEach(func() {
				handler = http.NewServeMux()
				handler.HandleFunc("/v1/apps", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
					_, _ = fmt.Fprintf(w, `["name1", "name2"]`)
				})
			})

			It("works", func() {
				client, err := admin.NewClient(socketAddr, fastTimeout)
				Expect(err).ToNot(HaveOccurred())

				_, err = client.GetAppsNames()
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when server responds with 500 error", func() {
			BeforeEach(func() {
				handler = http.NewServeMux()
				handler.HandleFunc("/v1/apps", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(500)
				})
			})

			It("fails", func() {
				client, err := admin.NewClient(socketAddr, fastTimeout)

				_, err = client.GetAppsNames()
				Expect(err).To(MatchError(admin.ErrStatusCodeNotOK))
			})
		})

		Context("when server returns invalid json", func() {
			BeforeEach(func() {
				handler = http.NewServeMux()
				handler.HandleFunc("/v1/apps", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)

					_, _ = fmt.Fprintf(w, "broken_json")
					_, _ = fmt.Fprintln(w)
				})
			})

			It("fails", func() {
				client, err := admin.NewClient(socketAddr, fastTimeout)
				Expect(err).ToNot(HaveOccurred())

				_, err = client.GetAppsNames()
				Expect(err).To(MatchError(admin.ErrDecodingResponse))
			})
		})

		Context("when can't talk to server", func() {
			It("fails", func() {
				Expect(true).To(Equal(true))
				client, err := admin.NewClient("foobar.sock", fastTimeout)
				Expect(err).ToNot(HaveOccurred())

				_, err = client.GetAppsNames()
				Expect(err).To(MatchError(admin.ErrMakingRequest))
			})
		})
	})

	Describe("DeleteApp", func() {
		Context("when server returns just fine", func() {
			BeforeEach(func() {
				handler = http.NewServeMux()
				handler.HandleFunc("/v1/apps", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
				})
			})

			It("works", func() {
				client, err := admin.NewClient(socketAddr, fastTimeout)
				Expect(err).ToNot(HaveOccurred())

				err = client.DeleteApp("appname")
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when can't talk to server", func() {
			It("fails", func() {
				Expect(true).To(Equal(true))
				client, err := admin.NewClient("foobar.sock", fastTimeout)
				Expect(err).ToNot(HaveOccurred())

				err = client.DeleteApp("appname")
				Expect(err).To(MatchError(admin.ErrMakingRequest))
			})
		})

		Context("when server responds with error", func() {
			BeforeEach(func() {
				handler = http.NewServeMux()
				handler.HandleFunc("/v1/apps", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(500)
				})
			})

			It("fails", func() {
				client, err := admin.NewClient(socketAddr, fastTimeout)
				Expect(err).ToNot(HaveOccurred())

				err = client.DeleteApp("appname")
				Expect(err).To(MatchError(admin.ErrStatusCodeNotOK))
			})
		})
	})

})
