package gospy

import (
	"log"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
)

var _ = Describe("analytics", func() {
	testing.WithConfig(func(cfg **config.Config) {
		Describe("NewSession", func() {
			It("works as expected", func(done Done) {
				s, err := Start(0, spy.ProfileCPU, 100, false)
				Expect(err).ToNot(HaveOccurred())
				go func() {
					s := time.Now()
					i := 0
					for time.Now().Sub(s) < 300*time.Millisecond {
						i++
						time.Sleep(1 * time.Nanosecond)
					}
				}()

				time.Sleep(50 * time.Millisecond)
				s.Snapshot(func(name []byte, samples uint64, err error) {
					log.Println("name", string(name))
				})
				close(done)
			})
		})
	})
})
