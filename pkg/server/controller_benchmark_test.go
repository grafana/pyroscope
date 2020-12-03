package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/davecgh/go-spew/spew"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/petethepig/pyroscope/pkg/config"
	"github.com/petethepig/pyroscope/pkg/storage"
	"github.com/petethepig/pyroscope/pkg/testing"
	"github.com/petethepig/pyroscope/pkg/timing"
	"github.com/petethepig/pyroscope/pkg/util/bytesize"
	// . "github.com/onsi/gomega"
)

type benchmarkResult struct {
	name        string
	totalTime   time.Duration
	avgTime     time.Duration
	totalSize   bytesize.ByteSize
	directories int
	files       int
	fileCount   int
}

type profile struct {
	bytes []byte
}

func cachedFile(path string, cb func() []byte) []byte {
	defer GinkgoRecover()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		f, err := os.Open(path)
		Expect(err).ToNot(HaveOccurred())
		b, err := ioutil.ReadAll(f)
		Expect(err).ToNot(HaveOccurred())
		return b
	}
	b := cb()
	f, err := os.Create(path)
	Expect(err).ToNot(HaveOccurred())
	f.Write(b)
	return b
}

func generateProfile(seed int) *profile {
	path := fmt.Sprintf("/tmp/profile-cache-%d.txt", seed)
	return &profile{
		bytes: cachedFile(path, func() []byte {
			cmd := exec.Command("ruby", "../../scripts/generate-fake-profile.rb", strconv.Itoa(seed))
			stdout, err := cmd.StdoutPipe()
			Expect(err).ToNot(HaveOccurred())

			cmd.Start()

			b, err := ioutil.ReadAll(stdout)
			Expect(err).ToNot(HaveOccurred())

			cmd.Wait()
			return b
		}),
	}
}

func benchmark(name string, t time.Time, dur time.Duration, profiles []*profile) *benchmarkResult {
	br := &benchmarkResult{name: name}
	timer := timing.New()
	testing.Profile("benchmark-"+name, func() {
		testing.TmpDir(func(path string) {
			defer GinkgoRecover()
			cfg := config.NewForTests(path)
			s, err := storage.New(cfg)
			Expect(err).ToNot(HaveOccurred())
			ctrl := New(cfg, s)

			server := httptest.NewServer(http.HandlerFunc(ctrl.ingestHandler))
			defer server.Close()

			for _, p := range profiles {
				qvalues := url.Values{}
				qvalues.Set("labels[\"host\"]", "localhost")
				qvalues.Set("from", t.Format("2006-01-02T15:04:05.999Z"))
				qvalues.Set("until", t.Add(dur).Format("2006-01-02T15:04:05.999Z"))
				res, err := http.Post(server.URL+"/ingest", "plain/text", bytes.NewReader(p.bytes))
				Expect(err).ToNot(HaveOccurred())
				r, err := ioutil.ReadAll(res.Body)
				Expect(err).ToNot(HaveOccurred())
				Expect(r).ToNot(BeNil())
				log.Debugf("%s", string(r))
			}
			br.totalTime = timer.End("")
			br.avgTime = br.totalTime / time.Duration(len(profiles))
			br.directories, br.files, br.totalSize = testing.DirStats(path)
		})
	})

	return br
}

// func measure(name string, t time.Time, dur time.Duration, profiles []*profile) {
// 	Measure(name, func(b Benchmarker) {
// 		var br *benchmarkResult

// 		b.Time("runtime", func() {
// 			br = benchmark(name, t, dur, profiles)
// 		})

// 		spew.Dump(br)

// 		b.RecordValue("disk usage (in Bytes)", float64(br.totalSize))
// 	}, 1)
// }

func measureSimple(name string, t time.Time, dur time.Duration, profiles []*profile) {
	It(name, func() {
		br := benchmark(name, t, dur, profiles)
		spew.Dump(br)
	}, 1)
}

var _ = Describe("ingest handler", func() {
	defer GinkgoRecover()
	// defer profile.Start(profile.MemProfileAllocs).Stop()
	t1 := testing.ParseTime("0000-00-00-00:00:00")

	testing.Profile("all-benchmarks", func() {
		measureSimple("1 profile", t1, time.Minute, []*profile{
			generateProfile(1),
		})
		measureSimple("2 profiles same", t1, time.Minute, []*profile{
			generateProfile(1),
			generateProfile(1),
		})
		measureSimple("2 profiles different", t1, time.Minute, []*profile{
			generateProfile(1),
			generateProfile(2),
		})
		measureSimple("3 profiles different", t1, time.Minute, []*profile{
			generateProfile(1),
			generateProfile(2),
			generateProfile(3),
		})
	})
	It("fails", func() {
		// Expect(2).To(Equal(1))
	})
})
