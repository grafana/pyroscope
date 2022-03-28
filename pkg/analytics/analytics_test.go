//go:build !windows && !race

package analytics

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

const durThreshold = 30 * time.Millisecond

type mockStatsProvider struct {
	stats map[string]int
}

func (m *mockStatsProvider) Stats() map[string]int {
	if m.stats != nil {
		return m.stats
	}
	return map[string]int{}
}

func (*mockStatsProvider) AppsCount() int { return 0 }

var _ = Describe("analytics", func() {
	gracePeriod = 100 * time.Millisecond
	uploadFrequency = 200 * time.Millisecond
	snapshotFrequency = 200 * time.Millisecond

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
						bytes, err := io.ReadAll(r.Body)
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

					s, err := storage.New(storage.NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller))
					Expect(err).ToNot(HaveOccurred())

					analytics := NewService(&(*cfg).Server, s, &mockStatsProvider{})

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
			It("cumilative metrics should persist on service stop", func() {
				done := make(chan interface{})
				go func() {
					defer GinkgoRecover()

					wg := sync.WaitGroup{}
					v := make(map[string]interface{})
					myHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						bytes, err := io.ReadAll(r.Body)
						Expect(err).ToNot(HaveOccurred())
						err = json.Unmarshal(bytes, &v)
						Expect(err).ToNot(HaveOccurred())
						w.WriteHeader(http.StatusOK)
						wg.Done()
					})

					httpServer := httptest.NewServer(myHandler)
					defer httpServer.Close()
					url = httpServer.URL + "/api/events"

					s, err := storage.New(storage.NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller))
					Expect(err).ToNot(HaveOccurred())

					stats := map[string]int{
						"diff":       1,
						"ingest":     1,
						"comparison": 1,
					}

					mockProvider := mockStatsProvider{stats: stats}

					for i := 0; i < 2; i = i + 1 {
						wg.Add(1)
						analytics := NewService(&(*cfg).Server, s, &mockProvider)
						go analytics.Start()
						wg.Wait()
						analytics.Stop()
					}

					Expect(v["controller_diff"]).To(BeEquivalentTo(2))
					Expect(v["controller_ingest"]).To(BeEquivalentTo(2))
					Expect(v["controller_comparison"]).To(BeEquivalentTo(2))
					Expect(v["analytics_persistence"]).To(BeTrue())

					close(done)
				}()
				Eventually(done, 2).Should(BeClosed())
			})
		})
	})
})
