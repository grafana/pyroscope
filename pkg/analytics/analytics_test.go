package analytics

import (
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

const durThreshold = 30 * time.Millisecond

type mockStatsProvider struct{}

func (mockStatsProvider) Stats() map[string]int { return map[string]int{} }

func (mockStatsProvider) AppsCount() int { return 0 }

var _ = Describe("analytics", func() {
	gracePeriod = 100 * time.Millisecond
	uploadFrequency = 200 * time.Millisecond

	testing.WithConfig(func(cfg **config.Config) {
		Describe("NewService", func() {
			It("works as expected", func() {
				done := make(chan interface{})
				go func() {
					defer GinkgoRecover()

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

					httpServer := httptest.NewServer(myHandler)
					defer httpServer.Close()
					url = httpServer.URL + "/api/events"

					s, err := storage.New(&(*cfg).Server, prometheus.NewRegistry())
					Expect(err).ToNot(HaveOccurred())

					analytics := NewService(&(*cfg).Server, s, mockStatsProvider{})

					startTime := time.Now()
					go analytics.Start()
					wg.Wait()
					analytics.Stop()
					Expect(timestamps).To(ConsistOf(
						BeTemporally("~", startTime.Add(100*time.Millisecond), durThreshold),
						BeTemporally("~", startTime.Add(300*time.Millisecond), durThreshold),
						BeTemporally("~", startTime.Add(500*time.Millisecond), durThreshold),
					))
					close(done)
				}()
				Eventually(done, 2).Should(BeClosed())
			})
		})
	})
})
