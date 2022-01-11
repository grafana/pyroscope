package api_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Suite")
}

// withRequest returns a function than performs an HTTP request
// with the body specified, and validates the response code and body.
//
// Request and response body ("in" and "out", correspondingly) are
// specified as a file name relative to the "testdata" directory.
// Either of "in" and "out" can be an empty string.
func withRequest(method, url string) func(code int, in, out string) {
	return func(code int, in, out string) {
		var reqBody io.Reader
		if in != "" {
			reqBody = readFile(in)
		}
		req, err := http.NewRequest(method, url, reqBody)
		Expect(err).ToNot(HaveOccurred())
		response, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		Expect(response).ToNot(BeNil())
		Expect(response.StatusCode).To(Equal(code))
		if out == "" {
			Expect(readBody(response).String()).To(BeEmpty())
			return
		}
		// It may also make sense to accept the response as a template
		// and render non-deterministic values.
		Expect(readBody(response)).To(MatchJSON(readFile(out)))
	}
}

func readFile(path string) *bytes.Buffer {
	b, err := os.ReadFile("testdata/" + path)
	Expect(err).ToNot(HaveOccurred())
	return bytes.NewBuffer(b)
}

func readBody(r *http.Response) *bytes.Buffer {
	b, err := io.ReadAll(r.Body)
	Expect(err).ToNot(HaveOccurred())
	_ = r.Body.Close()
	return bytes.NewBuffer(b)
}
