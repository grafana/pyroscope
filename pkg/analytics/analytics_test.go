package analytics

import (
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

const durThreshold = 30 * time.Millisecond

var _ = Describe("analytics", func() {
	// TODO: make port configurable
	url = "http://localhost:50000/api/events"
	gracePeriod = 100 * time.Millisecond
	uploadFrequency = 200 * time.Millisecond

	testing.WithConfig(func(cfg **config.Config) {
		Describe("NewService", func() {
			It("works as expected", func(done Done) {
				wg := sync.WaitGroup{}
				wg.Add(3)
				timestamps := []time.Time{}
				myHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					timestamps = append(timestamps, time.Now())
					bytes, err := ioutil.ReadAll(r.Body)
					Expect(err).ToNot(HaveOccurred())

					v := make(map[string]interface{})
					err = json.Unmarshal(bytes, &v)
					Expect(err).ToNot(HaveOccurred())

					fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
					wg.Done()
				})

				mockServer := &http.Server{
					Addr:           ":50000",
					Handler:        myHandler,
					ReadTimeout:    10 * time.Second,
					WriteTimeout:   10 * time.Second,
					MaxHeaderBytes: 1 << 20,
				}
				go mockServer.ListenAndServe()

				s, err := storage.New(&(*cfg).Server)
				Expect(err).ToNot(HaveOccurred())

				c, _ := server.New(&(*cfg).Server, s)
				analytics := NewService(&(*cfg).Server, s, c)

				startTime := time.Now()
				go analytics.Start()
				wg.Wait()
				mockServer.Close()
				analytics.Stop()
				Expect(timestamps).To(ConsistOf(
					BeTemporally("~", startTime.Add(100*time.Millisecond), durThreshold),
					BeTemporally("~", startTime.Add(300*time.Millisecond), durThreshold),
					BeTemporally("~", startTime.Add(500*time.Millisecond), durThreshold),
				))
				close(done)
			}, 2)
		})
	})
})
