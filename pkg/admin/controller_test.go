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

	"github.com/pyroscope-io/pyroscope/pkg/admin"
)

type mockAppsGetter struct{}

func (mockAppsGetter) GetAppNames() []string {
	return []string{"app1", "app2"}
}

var _ = Describe("controller", func() {
	Context("/v1/apps", func() {
		It("returns app names", func() {
			file, err := ioutil.TempFile("", "pyroscope.sock")
			if err != nil {
				panic(err)
			}
			socketAddr := file.Name()
			defer os.Remove(file.Name())

			cfg := admin.Config{
				SocketAddr: socketAddr,
			}
			svc := admin.NewService(mockAppsGetter{})
			ctrl, err := admin.NewController(cfg, svc)
			if err != nil {
				panic(err)
			}

			go func() {
				err := ctrl.Start()
				if err != nil {
					panic(err)
				}
			}()

			httpC := newHttpClient(socketAddr)

			// TODO
			// for some reason it takes some until server is ready
			time.Sleep(time.Second * 1)

			req, err := httpC.Get("http://dummy/v1/apps")
			Expect(err).To(BeNil())

			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				panic(err)
			}
			Expect(err).To(BeNil())

			Expect(string(body)).To(Equal(`["app1","app2"]
`))
		})
	})
})

func newHttpClient(socketAddr string) http.Client {
	return http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketAddr)
			},
		},
	}
}
