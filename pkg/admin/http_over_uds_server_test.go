package admin_test

import (
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/admin"
)

type mockHandler struct{}

func (m mockHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

var _ = Describe("HTTP Over UDS", func() {
	var (
		socketAddr string
		dir        string
		cleanup    func()
	)

	When("passed an empty socket address", func() {
		It("should give an error", func() {
			_, err := admin.NewUdsHTTPServer("")

			Expect(err).To(MatchError(admin.ErrInvalidSocketPathname))
		})
	})

	When("passed a non existing socket address", func() {
		It("should give an error", func() {
			// if user is root, s/he can create the socket anywhere
			if os.Getuid() == 0 {
				Skip("test is invalid when running as root")
			}

			_, err := admin.NewUdsHTTPServer("/non_existing_path")

			// TODO how to test for wrapped errors?
			// Expect(err).To(MatchError(fmt.Errorf("could not bind to socket")))
			Expect(err).To(HaveOccurred())
		})
	})

	When("passed an already bound socket address", func() {
		BeforeEach(func() {
			cleanup, dir = genRandomDir()
			socketAddr = dir + "/pyroscope.sock"
		})
		AfterEach(func() {
			cleanup()
		})

		When("that socket does not respond", func() {
			It("should take over that socket", func() {
				// create server 1
				By("creating server 1 that's not running")
				_, err := admin.NewUdsHTTPServer(socketAddr)
				Expect(err).ToNot(HaveOccurred())

				By("creating server 2")
				// create server 2
				_, err = admin.NewUdsHTTPServer(socketAddr)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("that socket is still responding", func() {
			It("should error", func() {
				By("creating server 1 and running it")
				server, err := admin.NewUdsHTTPServer(socketAddr)
				Expect(err).ToNot(HaveOccurred())

				go func() {
					server.Start(http.NewServeMux())
				}()

				waitUntilServerIsReady(socketAddr)

				// create server 2
				By("creating server 2")
				_, err = admin.NewUdsHTTPServer(socketAddr)
				Expect(err).To(MatchError(admin.ErrSocketStillResponding))
			})
		})
	})

	When("server is closed", func() {
		It("shutsdown properly", func() {
			cleanup, dir = genRandomDir()
			socketAddr = dir + "/pyroscope.sock"
			defer cleanup()

			// start the server
			server, err := admin.NewUdsHTTPServer(socketAddr)
			Expect(err).ToNot(HaveOccurred())
			go func() {
				server.Start(http.NewServeMux())
			}()

			waitUntilServerIsReady(socketAddr)

			server.Stop()

			Expect(socketAddr).ToNot(BeAnExistingFile())
		})
	})
})

func waitUntilServerIsReady(socketAddr string) error {
	const MaxReadinessRetries = 5
	client, err := admin.NewHTTPOverUDSClient(socketAddr)
	if err != nil {
		return err
	}
	retries := 0

	for {
		_, err := client.Get(admin.HealthAddress)

		// all good?
		if err == nil {
			return nil
		}
		if retries >= MaxReadinessRetries {
			break
		}

		time.Sleep(time.Millisecond * 300)
		retries++
	}

	return fmt.Errorf("maximum retries exceeded ('%d') waiting for server ('%s') to respond", retries, admin.HealthAddress)
}
