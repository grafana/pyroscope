package service_test

import (
	"os"

	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/sqlstore"
)

// testSuite is supposed to be DB-specific: once we add support for other
// SQL databases, each of them should have its own one; build tags are to
// be used in order to run tests with a particular SQL driver.
type testSuite struct {
	*sqlstore.SQLStore
	path string
}

func (s *testSuite) BeforeEach() {
	c := &sqlstore.Config{
		Type: "sqlite3",
		URL:  "file::memory:?cache=shared",
	}
	if s.path != "" {
		c.URL = s.path
	}
	var err error
	s.SQLStore, err = sqlstore.Open(c)
	Expect(err).ToNot(HaveOccurred())
}

func (s *testSuite) AfterEach() {
	defer func() {
		if s.path != "" {
			Expect(os.RemoveAll(s.path)).ToNot(HaveOccurred())
		}
	}()
	Expect(s.Close()).ToNot(HaveOccurred())
}
