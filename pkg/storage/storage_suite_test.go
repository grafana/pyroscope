package storage_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	testing2 "github.com/petethepig/pyroscope/pkg/testing"
)

func TestStorage(t *testing.T) {
	testing2.SetupLogging()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Storage Suite")
}
