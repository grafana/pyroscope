package admin_test

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
)

// TODO fazer todos esses testes
var _ = Describe("client", func() {
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
		s, err := admin.NewUdsHTTPServer(socketAddr)
		Expect(err).ToNot(HaveOccurred())
		server = s
		go server.Start(handler)
		waitUntilServerIsReady(socketAddr)
		// TODO wait for server to be up?
	})

	AfterEach(func() {
		server.Stop()
		cleanup()
	})

	Context("when socket address is empty", func() {
		It("fails", func() {
			_, err := admin.NewClient("")
			Expect(err).To(HaveOccurred())
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
				client, err := admin.NewClient(socketAddr)
				Expect(err).ToNot(HaveOccurred())

				_, err = client.GetAppsNames()
				Expect(err).ToNot(HaveOccurred())
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
				client, err := admin.NewClient(socketAddr)

				_, err = client.GetAppsNames()
				// TODO
				// how to test for specific errors
				Expect(err).To(HaveOccurred())
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
				client, err := admin.NewClient(socketAddr)
				Expect(err).ToNot(HaveOccurred())

				_, err = client.GetAppsNames()
				// TODO
				// how to test for specific errors
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when can't talk to server", func() {
			It("fails", func() {
				Expect(true).To(Equal(true))
				client, err := admin.NewClient("foobar.sock")
				Expect(err).ToNot(HaveOccurred())

				_, err = client.GetAppsNames()
				//// TODO
				//// how to test for specific errors
				Expect(err).To(HaveOccurred())
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
				client, err := admin.NewClient(socketAddr)
				Expect(err).ToNot(HaveOccurred())
				Expect(err).ToNot(HaveOccurred())

				err = client.DeleteApp("appname")
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when can't talk to server", func() {
			It("fails", func() {
				Expect(true).To(Equal(true))
				client, err := admin.NewClient("foobar.sock")
				Expect(err).ToNot(HaveOccurred())

				err = client.DeleteApp("appname")
				//// TODO
				//// how to test for specific errors
				Expect(err).To(HaveOccurred())
			})
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
			client, err := admin.NewClient(socketAddr)
			Expect(err).ToNot(HaveOccurred())

			err = client.DeleteApp("appname")
			// TODO
			// how to test for specific errors
			Expect(err).To(HaveOccurred())
		})
	})
})
