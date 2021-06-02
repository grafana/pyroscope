// +build dotnetspy

package dotnetspy

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("agent.DotnetSpy", func() {
	Describe("Does not panic if a session has not been established", func() {
		s := newSession(31337)
		s.timeout = time.Millisecond * 10
		Expect(s.start()).To(HaveOccurred())
		spy := &DotnetSpy{session: s}

		It("On Snapshot before Reset", func() {
			spy.Snapshot(func(name []byte, samples uint64, err error) {
				Fail("Snapshot callback must not be called")
			})
		})

		It("On Snapshot after Reset", func() {
			spy.Reset()
			spy.Snapshot(func(name []byte, samples uint64, err error) {
				Fail("Snapshot callback must not be called")
			})
		})

		It("On Stop", func() {
			Expect(spy.Stop()).ToNot(HaveOccurred())
		})
	})
})
