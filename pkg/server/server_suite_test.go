//go:build !windows
// +build !windows

package server_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	//reporterConfig.SlowSpecThreshold = 20 * time.Second
	RunSpecs(t, "Server Suite", suiteConfig, reporterConfig)
}
