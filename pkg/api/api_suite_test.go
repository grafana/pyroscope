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
