package tree

import (
	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
)

var (
	loc1               = &Location{Id: 1}
	loc2               = &Location{Id: 2}
	loc3               = &Location{Id: 3}
	fun1               = &Function{Id: 1}
	fun2               = &Function{Id: 2}
	fun3               = &Function{Id: 3}
	sortedLocs         = &Profile{Location: []*Location{loc1, loc2, loc3}}
	unsortedLocs       = &Profile{Location: []*Location{loc2, loc3, loc1}}
	nonconsecutiveLocs = &Profile{Location: []*Location{loc1, loc3}}
	sortedFuns         = &Profile{Function: []*Function{fun1, fun2, fun3}}
	unsortedFuns       = &Profile{Function: []*Function{fun2, fun3, fun1}}
	nonconsecutiveFuns = &Profile{Function: []*Function{fun1, fun3}}
)

var _ = Describe("profile finder", func() {
	Describe("Sorted consecutive locations", func() {
		It("returns correct results", func() {
			finder := NewFinder(sortedLocs)
			loc, ok := finder.FindLocation(1)
			Expect(ok).To(BeTrue())
			Expect(loc).To(BeIdenticalTo(loc1))
			loc, ok = finder.FindLocation(2)
			Expect(ok).To(BeTrue())
			Expect(loc).To(BeIdenticalTo(loc2))
			loc, ok = finder.FindLocation(3)
			Expect(ok).To(BeTrue())
			Expect(loc).To(BeIdenticalTo(loc3))
			loc, ok = finder.FindLocation(0)
			Expect(ok).To(BeFalse())
			loc, ok = finder.FindLocation(4)
			Expect(ok).To(BeFalse())
		})
	})
	Describe("Unsorted consecutive locations", func() {
		It("returns correct results", func() {
			finder := NewFinder(unsortedLocs)
			loc, ok := finder.FindLocation(1)
			Expect(ok).To(BeTrue())
			Expect(loc).To(BeIdenticalTo(loc1))
			loc, ok = finder.FindLocation(2)
			Expect(ok).To(BeTrue())
			Expect(loc).To(BeIdenticalTo(loc2))
			loc, ok = finder.FindLocation(3)
			Expect(ok).To(BeTrue())
			Expect(loc).To(BeIdenticalTo(loc3))
			loc, ok = finder.FindLocation(0)
			Expect(ok).To(BeFalse())
			loc, ok = finder.FindLocation(4)
			Expect(ok).To(BeFalse())
		})
	})
	Describe("Non-consecutive locations", func() {
		It("returns correct results", func() {
			finder := NewFinder(nonconsecutiveLocs)
			loc, ok := finder.FindLocation(1)
			Expect(ok).To(BeTrue())
			Expect(loc).To(BeIdenticalTo(loc1))
			loc, ok = finder.FindLocation(2)
			Expect(ok).To(BeFalse())
			loc, ok = finder.FindLocation(3)
			Expect(ok).To(BeTrue())
			Expect(loc).To(BeIdenticalTo(loc3))
			loc, ok = finder.FindLocation(0)
			Expect(ok).To(BeFalse())
			loc, ok = finder.FindLocation(4)
			Expect(ok).To(BeFalse())
		})
	})
	Describe("Sorted consecutive functions", func() {
		It("returns correct results", func() {
			finder := NewFinder(sortedFuns)
			fun, ok := finder.FindFunction(1)
			Expect(ok).To(BeTrue())
			Expect(fun).To(BeIdenticalTo(fun1))
			fun, ok = finder.FindFunction(2)
			Expect(ok).To(BeTrue())
			Expect(fun).To(BeIdenticalTo(fun2))
			fun, ok = finder.FindFunction(3)
			Expect(ok).To(BeTrue())
			Expect(fun).To(BeIdenticalTo(fun3))
			fun, ok = finder.FindFunction(0)
			Expect(ok).To(BeFalse())
			fun, ok = finder.FindFunction(4)
			Expect(ok).To(BeFalse())
		})
	})
	Describe("Unsorted consecutive functions", func() {
		It("returns correct results", func() {
			finder := NewFinder(unsortedFuns)
			fun, ok := finder.FindFunction(1)
			Expect(ok).To(BeTrue())
			Expect(fun).To(BeIdenticalTo(fun1))
			fun, ok = finder.FindFunction(2)
			Expect(ok).To(BeTrue())
			Expect(fun).To(BeIdenticalTo(fun2))
			fun, ok = finder.FindFunction(3)
			Expect(ok).To(BeTrue())
			Expect(fun).To(BeIdenticalTo(fun3))
			fun, ok = finder.FindFunction(0)
			Expect(ok).To(BeFalse())
			fun, ok = finder.FindFunction(4)
			Expect(ok).To(BeFalse())
		})
	})
	Describe("Non-consecutive functions", func() {
		It("returns correct results", func() {
			finder := NewFinder(nonconsecutiveFuns)
			fun, ok := finder.FindFunction(1)
			Expect(ok).To(BeTrue())
			Expect(fun).To(BeIdenticalTo(fun1))
			fun, ok = finder.FindFunction(2)
			Expect(ok).To(BeFalse())
			fun, ok = finder.FindFunction(3)
			Expect(ok).To(BeTrue())
			Expect(fun).To(BeIdenticalTo(fun3))
			fun, ok = finder.FindFunction(0)
			Expect(ok).To(BeFalse())
			fun, ok = finder.FindFunction(4)
			Expect(ok).To(BeFalse())
		})
	})
})
