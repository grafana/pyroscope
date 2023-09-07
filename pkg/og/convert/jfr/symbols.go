package jfr

import (
	"regexp"

	"github.com/grafana/jfr-parser/parser/types"
)

// jdk/internal/reflect/GeneratedMethodAccessor31
var generatedMethodAccessor = regexp.MustCompile("^(jdk/internal/reflect/GeneratedMethodAccessor)(\\d+)$")

// org/example/rideshare/OrderService$$Lambda$669.0x0000000800fd7318.run
var lambdaGeneratedEnclosingClass = regexp.MustCompile("^(.+\\$\\$Lambda\\$)\\d+[./](0x[\\da-f]+|\\d+)$")

// libzstd-jni-1.5.1-16931311898282279136.so.Java_com_github_luben_zstd_ZstdInputStreamNoFinalizer_decompressStream
var zstdJniSoLibName = regexp.MustCompile("^(\\.?/tmp/)?(libzstd-jni-\\d+\\.\\d+\\.\\d+-)(\\d+)(\\.so)( \\(deleted\\))?$")

// ./tmp/libamazonCorrettoCryptoProvider109b39cf33c563eb.so
// ./tmp/amazonCorrettoCryptoProviderNativeLibraries.7382c2f79097f415/libcrypto.so (deleted)
var amazonCorrettoCryptoProvider = regexp.MustCompile("^(\\.?/tmp/)?(lib)?(amazonCorrettoCryptoProvider)(NativeLibraries\\.)?([0-9a-f]{16})" +
	"(/libcrypto|/libamazonCorrettoCryptoProvider)?(\\.so)( \\(deleted\\))?$")

// libasyncProfiler-linux-arm64-17b9a1d8156277a98ccc871afa9a8f69215f92.so
var pyroscopeAsyncProfiler = regexp.MustCompile(
	"^(\\.?/tmp/)?(libasyncProfiler)-(linux-arm64|linux-musl-x64|linux-x64|macos)-(17b9a1d8156277a98ccc871afa9a8f69215f92)(\\.so)( \\(deleted\\))?$")

// TODO
// ./tmp/snappy-1.1.8-6fb9393a-3093-4706-a7e4-837efe01d078-libsnappyjava.so
func mergeJVMGeneratedClasses(frame string) string {
	frame = generatedMethodAccessor.ReplaceAllString(frame, "${1}_")
	frame = lambdaGeneratedEnclosingClass.ReplaceAllString(frame, "${1}_")
	frame = zstdJniSoLibName.ReplaceAllString(frame, "libzstd-jni-_.so")
	frame = amazonCorrettoCryptoProvider.ReplaceAllString(frame, "libamazonCorrettoCryptoProvider_.so")
	frame = pyroscopeAsyncProfiler.ReplaceAllString(frame, "libasyncProfiler-_.so")
	return frame
}

func processSymbols(ref *types.SymbolList) {
	for i := range ref.Symbol { //todo regex replace inplace
		ref.Symbol[i].String = mergeJVMGeneratedClasses(ref.Symbol[i].String)
	}
}
