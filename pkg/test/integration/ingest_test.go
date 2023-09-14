package integration

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/pyroscope/pkg/cfg"
	pprof2 "github.com/grafana/pyroscope/pkg/og/convert/pprof"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/grafana/pyroscope/pkg/phlare"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type PyroscopeTest struct {
	config phlare.Config
	it     *phlare.Phlare
	wg     sync.WaitGroup
}

func (p *PyroscopeTest) Start(t *testing.T) {

	err := cfg.DynamicUnmarshal(&p.config, []string{"pyroscope"}, flag.NewFlagSet("pyroscope", flag.ContinueOnError))
	require.NoError(t, err)
	p.config.SelfProfiling.DisablePush = true
	p.it, err = phlare.New(p.config)

	require.NoError(t, err)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		err := p.it.Run()
		require.NoError(t, err)
	}()
	require.Eventually(t, func() bool {
		return p.ringActive(t)
	}, 10005*time.Second, 100*time.Millisecond)
}

func (p *PyroscopeTest) Stop(t *testing.T) {
	p.it.SignalHandler.Stop()
	p.wg.Wait()
}

func (p *PyroscopeTest) ringActive(t *testing.T) bool {
	res, err := http.Get("http://localhost:4040/ring")
	if err != nil {
		return false
	}
	if res.StatusCode != 200 || res.Body == nil {
		return false
	}
	body := bytes.NewBuffer(nil)
	_, err = io.Copy(body, res.Body)
	if err != nil {
		return false
	}
	if strings.Contains(body.String(), "ACTIVE") {
		return true
	}
	return false
}

func TestIngestPPROF(t *testing.T) {
	p := PyroscopeTest{}
	p.Start(t)
	defer p.Stop(t)

	const repoRoot = "../../../"

	golangHeap := []string{
		"memory:inuse_space:bytes:space:bytes",
		"memory:inuse_objects:count:space:bytes",
		"memory:alloc_space:bytes:space:bytes",
		"memory:alloc_objects:count:space:bytes",
	}
	golangCPU := []string{
		"process_cpu:samples:count:cpu:nanoseconds",
		"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
	}
	_ = golangHeap
	_ = golangCPU
	testdata := []struct {
		profile          string
		prevProfile      string
		sampleTypeConfig string
		spyName          string

		expectStatus int
		metrics      []string
	}{
		{
			profile:      repoRoot + "pkg/pprof/testdata/heap",
			expectStatus: 200,
			metrics:      golangHeap,
		},
		{
			profile:      repoRoot + "pkg/pprof/testdata/profile_java",
			expectStatus: 200,
			metrics: []string{
				"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
			},
		},
		{
			profile:      repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			expectStatus: 200,
			metrics:      golangCPU,
		},
		{
			profile:      repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			prevProfile:  repoRoot + "pkg/og/convert/testdata/cpu.pprof",
			expectStatus: 422,
		},

		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/cpu.pb.gz",
			prevProfile:  "",
			expectStatus: 200,
			metrics:      golangCPU,
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/cpu-exemplars.pb.gz",
			expectStatus: 200,
			metrics:      golangCPU,
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/cpu-js.pb.gz",
			expectStatus: 200,
			metrics: []string{
				"wall:sample:count:wall:microseconds",
				"wall:wall:microseconds:wall:microseconds",
			},
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/heap.pb",
			expectStatus: 200,
			metrics:      golangHeap,
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/heap.pb.gz",
			expectStatus: 200,
			metrics:      golangHeap,
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/heap-js.pprof",
			expectStatus: 200,
			metrics: []string{
				"memory:space:bytes:space:bytes",
				"memory:objects:count:space:bytes",
			},
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/nodejs-heap.pb.gz",
			expectStatus: 200,
			metrics: []string{
				"memory:inuse_space:bytes:inuse_space:bytes",
				"memory:inuse_objects:count:inuse_space:bytes",
			},
		},
		{
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/nodejs-wall.pb.gz",
			expectStatus: 200,
			metrics: []string{
				"wall:samples:count:wall:microseconds",
				"wall:wall:microseconds:wall:microseconds",
			},
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_2.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_2.st.json",
			expectStatus:     200,
			metrics: []string{
				"goroutines:goroutine:count:goroutine:count",
			},
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_3.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_3.st.json",
			expectStatus:     200,
			metrics: []string{
				"block:delay:nanoseconds:contentions:count",
				"block:contentions:count:contentions:count",
			},
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_4.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_4.st.json",
			expectStatus:     200,
			metrics: []string{
				"mutex:delay:nanoseconds:contentions:count",
				"mutex:contentions:count:contentions:count",
			},
		},
		{
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/req_5.pprof",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/req_5.st.json",
			expectStatus:     200,
			metrics: []string{
				"memory:alloc_objects:count:space:bytes",
				"memory:alloc_space:bytes:space:bytes",
			},
		},
		{
			// this one have milliseconds in Profile.TimeNanos
			// https://github.com/grafana/pyroscope/pull/2376/files
			profile:      repoRoot + "pkg/og/convert/pprof/testdata/pyspy-1.pb.gz",
			expectStatus: 200,
			metrics: []string{
				"process_cpu:samples:count::milliseconds",
			},
			spyName: pprof2.SpyNameForFunctionNameRewrite(),
		},

		{
			// this one is broken dotnet pprof
			// it has function.id == 0 for every function
			profile:          repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-3.pb.gz",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-3.st.json",
			expectStatus:     200,
			metrics: []string{
				"rocess_cpu:cpu:nanoseconds::nanoseconds",
			},
		},
		{
			// this one is broken dotnet pprof
			// it has function.id == 0 for every function
			// it also has "-" in sample type name
			profile:          repoRoot + "TODO",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/dotnet-pprof-3.st.json",
			expectStatus:     200,
			metrics: []string{
				"fail",
			},
		},
		{
			// this is a fixed dotnet pprof
			profile:          repoRoot + "TODO",
			sampleTypeConfig: repoRoot + "pkg/og/convert/pprof/testdata/TODO",
			expectStatus:     200,
			metrics: []string{
				"fail",
			},
		},
	}
	for _, testdatum := range testdata {
		var (
			profile, prevProfile, sampleTypeConfig []byte
			err                                    error
		)
		profile, err = os.ReadFile(testdatum.profile)
		assert.NoError(t, err)
		if testdatum.prevProfile != "" {
			prevProfile, err = os.ReadFile(testdatum.prevProfile)
			assert.NoError(t, err)
		}
		if testdatum.sampleTypeConfig != "" {
			sampleTypeConfig, err = os.ReadFile(testdatum.sampleTypeConfig)
			assert.NoError(t, err)
		}
		bs, ct := createPProfRequest(t, profile, prevProfile, sampleTypeConfig)

		spyName := "foo239"
		if testdatum.spyName != "" {
			spyName = testdatum.spyName
		}

		appName := fmt.Sprintf("pprof.integration.%s.%d",
			strings.ReplaceAll(filepath.Base(testdatum.profile), "-", "_"),
			rand.Uint64())
		url := "http://localhost:4040/ingest?name=" + appName + "&spyName=" + spyName
		req, err := http.NewRequest("POST", url, bytes.NewReader(bs))
		require.NoError(t, err)
		req.Header.Set("Content-Type", ct)

		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, testdatum.expectStatus, res.StatusCode, testdatum.profile)
		fmt.Printf("%+v %+v\n", testdatum, res)

		if testdatum.expectStatus == 200 {
			for _, metric := range testdatum.metrics {
				// todo use not only /render
				queryURL := "http://localhost:4040/pyroscope/render?query=" + metric + "{service_name=\"" + appName + "\"}&from=now-1h&until=now&format=collapsed"
				queryRes, err := http.Get(queryURL)
				require.NoError(t, err)
				body := bytes.NewBuffer(nil)
				_, err = io.Copy(body, queryRes.Body)
				assert.NoError(t, err)
				fb := new(flamebearer.FlamebearerProfile)
				err = json.Unmarshal(body.Bytes(), fb)
				assert.NoError(t, err, testdatum.profile)
				assert.Greater(t, len(fb.Flamebearer.Names), 0, testdatum.profile)
				// todo check actual stacktrace contents
			}
		}
	}

	time.Sleep(time.Hour)
}

func createPProfRequest(t *testing.T, profile, prevProfile, sampleTypeConfig []byte) ([]byte, string) {
	const (
		formFieldProfile          = "profile"
		formFieldPreviousProfile  = "prev_profile"
		formFieldSampleTypeConfig = "sample_type_config"
	)

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	profileW, err := w.CreateFormFile(formFieldProfile, "not used")
	require.NoError(t, err)
	_, err = profileW.Write(profile)
	require.NoError(t, err)

	if sampleTypeConfig != nil {

		sampleTypeConfigW, err := w.CreateFormFile(formFieldSampleTypeConfig, "not used")
		require.NoError(t, err)
		_, err = sampleTypeConfigW.Write(sampleTypeConfig)
		require.NoError(t, err)
	}

	if prevProfile != nil {
		prevProfileW, err := w.CreateFormFile(formFieldPreviousProfile, "not used")
		require.NoError(t, err)
		_, err = prevProfileW.Write(prevProfile)
		require.NoError(t, err)
	}
	err = w.Close()
	require.NoError(t, err)

	return b.Bytes(), w.FormDataContentType()
}
