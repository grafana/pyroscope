package attime

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("attime", func() {
	Describe("Parse", func() {
		Context("simple cases", func() {
			It("works correctly", func() {
				Expect(Parse("now")).To(BeTemporally("~", time.Now()))
				Expect(Parse("now-1s")).To(BeTemporally("~", time.Now().Add(-1*time.Second)))
				Expect(Parse("now+1s")).To(BeTemporally("~", time.Now().Add(1*time.Second)))
				Expect(Parse("now-1min")).To(BeTemporally("~", time.Now().Add(-1*time.Minute)))
				Expect(Parse("now-1h")).To(BeTemporally("~", time.Now().Add(-1*time.Hour)))
				Expect(Parse("now-1d")).To(BeTemporally("~", time.Now().Add(-1*time.Hour*24)))
				Expect(Parse("now-1w")).To(BeTemporally("~", time.Now().Add(-1*time.Hour*24*7)))
				Expect(Parse("now-1mon")).To(BeTemporally("~", time.Now().Add(-1*time.Hour*24*30)))
				Expect(Parse("now-1M")).To(BeTemporally("~", time.Now().Add(-1*time.Hour*24*30)))
				Expect(Parse("now-1y")).To(BeTemporally("~", time.Now().Add(-1*time.Hour*24*365)))
				Expect(Parse("now-1")).To(BeTemporally("~", time.Now()))
				Expect(Parse("20200101")).To(BeTemporally("~", time.Unix(1577836800, 0)))
				Expect(Parse("1577836800")).To(BeTemporally("~", time.Unix(1577836800, 0)))
			})
		})
	})
})
