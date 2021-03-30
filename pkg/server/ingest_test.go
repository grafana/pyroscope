package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("server", func() {
	testing.WithConfig(func(cfg **config.Config) {

		BeforeEach(func() {
			(*cfg).Server.APIBindAddr = ":51234"
		})

		Describe("/ingest", func() {
			Context("default format", func() {
				It("works as expected", func(done Done) {
					buf := bytes.NewBuffer([]byte("foo;bar 2\nfoo;baz 3\n"))
					s, err := storage.New(*cfg)
					Expect(err).ToNot(HaveOccurred())
					c := New(*cfg, s)
					go c.Start()

					name := "test.app{}"

					st := testing.ParseTime("2020-01-01-01:01:00")
					et := testing.ParseTime("2020-01-01-01:01:10")

					u, _ := url.Parse("http://localhost:51234/ingest")
					q := u.Query()
					q.Add("name", name)
					q.Add("from", strconv.Itoa(int(st.Unix())))
					q.Add("until", strconv.Itoa(int(et.Unix())))
					u.RawQuery = q.Encode()

					fmt.Println(u.String())

					req, err := http.Post(u.String(), "text/plain", buf)
					Expect(err).ToNot(HaveOccurred())
					Expect(req.StatusCode).To(Equal(200))

					sk, _ := storage.ParseKey(name)
					t, _, _, _, _ := s.Get(st, et, sk)
					Expect(t).ToNot(BeNil())
					Expect(t.String()).To(Equal("\"foo;bar\" 2\n\"foo;baz\" 3\n"))

					close(done)
				})
			})
		})
	})
})
