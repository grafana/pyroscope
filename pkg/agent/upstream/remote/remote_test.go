package remote

import (
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/agent/upstream"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
	"github.com/sirupsen/logrus"
)

var _ = Describe("remote.Remote", func() {
	Describe("Upload", func() {
		It("uploads data to an http server", func() {
			done := make(chan interface{})
			func() {
				wg := sync.WaitGroup{}
				wg.Add(3)
				var timestampsMutex sync.Mutex
				timestamps := []time.Time{}
				myHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()

					timestampsMutex.Lock()
					timestamps = append(timestamps, time.Now())
					timestampsMutex.Unlock()
					_, err := ioutil.ReadAll(r.Body)
					Expect(err).ToNot(HaveOccurred())

					fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
					wg.Done()
				})

				mockServer := &http.Server{
					Addr:           ":50001",
					Handler:        myHandler,
					ReadTimeout:    10 * time.Second,
					WriteTimeout:   10 * time.Second,
					MaxHeaderBytes: 1 << 20,
				}
				go mockServer.ListenAndServe()

				cfg := RemoteConfig{
					AuthToken:              "",
					UpstreamThreads:        4,
					UpstreamAddress:        "http://localhost:50001",
					UpstreamRequestTimeout: 3 * time.Second,
				}
				r, err := New(cfg, logrus.New())

				t := transporttrie.New()
				for i := 0; i < 3; i++ {
					r.Upload(&upstream.UploadJob{
						Name:       "test{}",
						StartTime:  testing.SimpleTime(0),
						EndTime:    testing.SimpleTime(10),
						SpyName:    "debugspy",
						SampleRate: 100,
						Units:      "samples",
						Trie:       t,
					})
				}

				Expect(err).To(BeNil())
				wg.Wait()
				r.Stop()
				close(done)
			}()
			Eventually(done, 5).Should(BeClosed())
		})
	})
})
