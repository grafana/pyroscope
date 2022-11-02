package service

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cache", func() {
	It("test", func() {
		cache := newCache(100, time.Minute*5)
		cache.put("test", "hey")
		//x, ok := cache.get("test")
		//Expect(ok).To(BeTrue())
		//Expect(x).To(Equal("hey"))

		_, ok := cache.c.Get("test")
		Expect(ok).To(BeTrue())

		_, ok2 := cache.get("test")
		Expect(ok2).To(BeTrue())
		//		Expect(false).To(BeTrue())
	})
})
