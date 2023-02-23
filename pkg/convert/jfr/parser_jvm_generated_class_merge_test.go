package jfr

import (
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("mergeJVMGeneratedClasses", func() {
	ginkgo.When("we parse jfr", func() {
		ginkgo.When("we process jvm generated classes", func() {
			testcases := [][]string{
				{
					"org/example/rideshare/EnclosingClass$$Lambda$4/1283928880",
					"org/example/rideshare/EnclosingClass$$Lambda$_",
				},
				{
					"org/example/rideshare/EnclosingClass$$Lambda$8/0x0000000800c01220",
					"org/example/rideshare/EnclosingClass$$Lambda$_",
				},
				{
					"java/util/concurrent/Executors$RunnableAdapter",
					"java/util/concurrent/Executors$RunnableAdapter",
				},
				{
					"jdk/internal/reflect/GeneratedMethodAccessor31",
					"jdk/internal/reflect/GeneratedMethodAccessor_",
				},
			}
			for _, testcase := range testcases {
				src := testcase[0]
				expectedRes := testcase[1]
				ginkgo.It("merges so names - "+src, func() {
					res := mergeJVMGeneratedClasses(src)
					Expect(res).To(Equal(expectedRes))
				})
			}
		})
		ginkgo.When("we process shared libraries names", func() {
			testcases := [][]string{
				{
					"libasyncProfiler-linux-arm64-17b9a1d8156277a98ccc871afa9a8f69215f92.so",
					"libasyncProfiler-_.so",
				},
				{
					"libasyncProfiler-linux-musl-x64-17b9a1d8156277a98ccc871afa9a8f69215f92.so",
					"libasyncProfiler-_.so",
				},
				{
					"libasyncProfiler-linux-x64-17b9a1d8156277a98ccc871afa9a8f69215f92.so",
					"libasyncProfiler-_.so",
				},
				{
					"libasyncProfiler-macos-17b9a1d8156277a98ccc871afa9a8f69215f92.so",
					"libasyncProfiler-_.so",
				},
				{
					"libamazonCorrettoCryptoProvider109b39cf33c563eb.so",
					"libamazonCorrettoCryptoProvider_.so",
				},
				{
					"amazonCorrettoCryptoProviderNativeLibraries.7382c2f79097f415/libcrypto.so",
					"libamazonCorrettoCryptoProvider_.so",
				},
				{
					"libzstd-jni-1.5.1-16931311898282279136.so",
					"libzstd-jni-_.so",
				},
			}
			for _, testcase := range testcases {
				src := testcase[0]
				expectedRes := testcase[1]
				ginkgo.It("merges generated so names - "+src, func() {
					res := mergeJVMGeneratedClasses(src)
					Expect(res).To(Equal(expectedRes))
				})
				ginkgo.It("merges generated so names even deleted - "+src, func() {
					res := mergeJVMGeneratedClasses(src + " (deleted)")
					Expect(res).To(Equal(expectedRes))
				})
				ginkgo.It("merges generated so names even deleted inside with /tmp prefix - "+src, func() {
					res := mergeJVMGeneratedClasses("/tmp/" + src + " (deleted)")
					Expect(res).To(Equal(expectedRes))
				})
				ginkgo.It("merges generated so names even deleted inside with ./tmp prefix - "+src, func() {
					res := mergeJVMGeneratedClasses("./tmp/" + src + " (deleted)")
					Expect(res).To(Equal(expectedRes))
				})
			}
		})
	})
})
