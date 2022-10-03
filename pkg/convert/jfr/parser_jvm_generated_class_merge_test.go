package jfr

import (
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("mergeJVMGeneratedClasses", func() {
	ginkgo.It("merges GeneratedMethodAccessor classes", func() {
		src := "jdk/internal/reflect/GeneratedMethodAccessor31"
		res := mergeJVMGeneratedClasses(src)
		Expect(res).To(Equal("jdk/internal/reflect/GeneratedMethodAccessor_"))
	})

	ginkgo.It("does nothing for regular frames", func() {
		src := "java/util/concurrent/Executors$RunnableAdapter"
		res := mergeJVMGeneratedClasses(src)
		Expect(res).To(Equal(src))
	})

	ginkgo.It("merges Lambdas", func() {
		src := "org/example/rideshare/EnclosingClass$$Lambda$8/0x0000000800c01220"
		res := mergeJVMGeneratedClasses(src)
		Expect(res).To(Equal("org/example/rideshare/EnclosingClass$$Lambda$_"))
	})

	ginkgo.It("merges old Lambdas", func() {
		src := "org/example/rideshare/EnclosingClass$$Lambda$4/1283928880"
		res := mergeJVMGeneratedClasses(src)
		Expect(res).To(Equal("org/example/rideshare/EnclosingClass$$Lambda$_"))
	})

	ginkgo.It("merges zstd com_github_luben_zstd", func() {
		src := "libzstd-jni-1.5.1-16931311898282279136.so"
		res := mergeJVMGeneratedClasses(src)
		Expect(res).To(Equal("libzstd-jni-1.5.1-_.so"))
	})

	ginkgo.It("merges amazon correto crypto provider", func() {
		src := "./tmp/libamazonCorrettoCryptoProvider109b39cf33c563eb.so"
		res := mergeJVMGeneratedClasses(src)
		Expect(res).To(Equal("libamazonCorrettoCryptoProvider_.so"))
	})

	ginkgo.When("async-profiler", func() {
		testcases := [][]string{
			{"libasyncProfiler-linux-arm64-17b9a1d8156277a98ccc871afa9a8f69215f92.so",
				"libasyncProfiler-linux-arm64-_.so"},
			{"libasyncProfiler-linux-musl-x64-17b9a1d8156277a98ccc871afa9a8f69215f92.so",
				"libasyncProfiler-linux-musl-x64-_.so"},
			{"libasyncProfiler-linux-x64-17b9a1d8156277a98ccc871afa9a8f69215f92.so",
				"libasyncProfiler-linux-x64-_.so"},
			{"libasyncProfiler-macos-17b9a1d8156277a98ccc871afa9a8f69215f92.so",
				"libasyncProfiler-macos-_.so"},
		}
		for _, testcase := range testcases {
			src := testcase[0]
			expectedRes := testcase[1]
			ginkgo.It("merges different versions "+src, func() {
				res := mergeJVMGeneratedClasses(src)
				Expect(res).To(Equal(expectedRes))
			})
		}
	})
})
