package attime

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("attime", func() {
	Describe("Parse", func() {
		Context("simple cases", func() {
			It("works correctly", func() {
				now := time.Unix(1577836800, 0)
				timeNow = func() time.Time { return now }
				defer func() { timeNow = time.Now }()

				Expect(Parse("now")).To(Equal(now))
				Expect(Parse("now-1s")).To(Equal(now.Add(-1 * time.Second)))
				Expect(Parse("now+1s")).To(Equal(now.Add(1 * time.Second)))
				Expect(Parse("now-1min")).To(Equal(now.Add(-1 * time.Minute)))
				Expect(Parse("now-1h")).To(Equal(now.Add(-1 * time.Hour)))
				Expect(Parse("now-1d")).To(Equal(now.Add(-1 * time.Hour * 24)))
				Expect(Parse("now-1w")).To(Equal(now.Add(-1 * time.Hour * 24 * 7)))
				Expect(Parse("now-1mon")).To(Equal(now.Add(-1 * time.Hour * 24 * 30)))
				Expect(Parse("now-1M")).To(Equal(now.Add(-1 * time.Hour * 24 * 30)))
				Expect(Parse("now-1y")).To(Equal(now.Add(-1 * time.Hour * 24 * 365)))
				Expect(Parse("now-1")).To(Equal(now))
				Expect(Parse("20200101")).To(Equal(time.Unix(1577836800, 0).UTC()))
				Expect(Parse("1577836800")).To(Equal(time.Unix(1577836800, 0)))
				Expect(Parse("1577836800001")).To(Equal(time.Unix(1577836800, 1000000)))
				Expect(Parse("1577836800000001")).To(Equal(time.Unix(1577836800, 1000)))
				Expect(Parse("1577836800000000001")).To(Equal(time.Unix(1577836800, 1)))
			})
		})
	})
})
