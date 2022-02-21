package storage_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	testutils "github.com/pyroscope-io/pyroscope/pkg/testing"
)

func TestStorage(t *testing.T) {
	testutils.SetupLogging()

	RegisterFailHandler(Fail)
	RunSpecs(t, "Storage Suite")
}
