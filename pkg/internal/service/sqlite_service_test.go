package service_test

import (
	"os"

	. "github.com/onsi/gomega"
	"github.com/pyroscope-io/pyroscope/pkg/sqlstore"
)

// testSuite supposed to be DB-specific.
type testSuite struct {
	*sqlstore.SQLStore
	path string
}

func (s *testSuite) BeforeEach() {
	c := &sqlstore.Config{
		Logger: nil, // TODO
		Type:   "sqlite3",
		URL:    "file::memory:?cache=shared",
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
