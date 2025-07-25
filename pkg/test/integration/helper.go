package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/prometheus/common/expfmt"

	profilesv1 "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/cfg"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/pyroscope"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func EachPyroscopeTest(t *testing.T, f func(p *PyroscopeTest, t *testing.T)) {
	tests := []struct {
		name string
		f    func(t *testing.T) *PyroscopeTest
	}{
		{
			"v1",
			func(t *testing.T) *PyroscopeTest {
				return new(PyroscopeTest).Configure(t, false)
			},
		},
		{
			"v2",
			func(t *testing.T) *PyroscopeTest {
				return new(PyroscopeTest).Configure(t, true)
			},
		},
	}
	for _, pt := range tests {
		t.Run(pt.name, func(t *testing.T) {
			p := pt.f(t)
			p.start(t)
			t.Cleanup(func() {
				p.stop()
			})
			f(p, t)
		})
	}
}

type PyroscopeTest struct {
	config         pyroscope.Config
	it             *pyroscope.Pyroscope
	wg             sync.WaitGroup
	prevReg        prometheus.Registerer
	reg            *prometheus.Registry
	httpPort       int
	memberlistPort int
	grpcPort       int
	raftPort       int
}

const address = "127.0.0.1"
const storeInMemory = "inmemory"

func (p *PyroscopeTest) start(t *testing.T) {
	var err error

	p.it, err = pyroscope.New(p.config)

	require.NoError(t, err)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		err := p.it.Run()
		require.NoError(t, err)
	}()
	require.Eventually(t, func() bool {
		return p.ringActive() && p.ready()
	}, 30*time.Second, 100*time.Millisecond)
}

func (p *PyroscopeTest) Configure(t *testing.T, v2 bool) *PyroscopeTest {
	ports, err := GetFreePorts(4)
	require.NoError(t, err)
	p.httpPort = ports[0]
	p.memberlistPort = ports[1]
	p.grpcPort = ports[2]
	p.raftPort = ports[3]
	t.Logf("ports: http %d memberlist %d grpc %d raft %d", p.httpPort, p.memberlistPort, p.grpcPort, p.raftPort)

	p.prevReg = prometheus.DefaultRegisterer
	p.reg = prometheus.NewRegistry()
	prometheus.DefaultRegisterer = p.reg

	p.config.V2 = v2
	err = cfg.DynamicUnmarshal(&p.config, []string{"pyroscope"}, flag.NewFlagSet("pyroscope", flag.ContinueOnError))
	require.NoError(t, err)

	// set addresses and ports
	p.config.Server.HTTPListenAddress = address
	p.config.Server.HTTPListenPort = p.httpPort
	p.config.Server.GRPCListenAddress = address
	p.config.Server.GRPCListenPort = p.grpcPort
	p.config.Worker.SchedulerAddress = address
	p.config.MemberlistKV.AdvertisePort = p.memberlistPort
	p.config.MemberlistKV.TCPTransport.BindPort = p.memberlistPort
	p.config.Ingester.LifecyclerConfig.Addr = address
	p.config.Ingester.LifecyclerConfig.MinReadyDuration = 0
	p.config.QueryScheduler.ServiceDiscovery.SchedulerRing.InstanceAddr = address
	p.config.Frontend.Addr = address

	// heartbeat more often
	p.config.Distributor.DistributorRing.HeartbeatPeriod = time.Second
	p.config.Ingester.LifecyclerConfig.HeartbeatPeriod = time.Second
	p.config.OverridesExporter.Ring.Ring.HeartbeatPeriod = time.Second
	p.config.QueryScheduler.ServiceDiscovery.SchedulerRing.HeartbeatPeriod = time.Second

	// do not use memberlist
	p.config.Distributor.DistributorRing.KVStore.Store = storeInMemory
	p.config.Ingester.LifecyclerConfig.RingConfig.KVStore.Store = storeInMemory
	p.config.OverridesExporter.Ring.Ring.KVStore.Store = storeInMemory
	p.config.QueryScheduler.ServiceDiscovery.SchedulerRing.KVStore.Store = storeInMemory

	p.config.SelfProfiling.DisablePush = true
	p.config.Analytics.Enabled = false // usage-stats terminating slow as hell
	p.config.LimitsConfig.MaxQueryLength = 0
	p.config.LimitsConfig.MaxQueryLookback = 0
	p.config.LimitsConfig.RejectOlderThan = 0
	_ = p.config.Server.LogLevel.Set("debug")

	if v2 {
		p.config.Storage.Bucket.Filesystem.Directory = t.TempDir()
		p.config.Storage.Bucket.Backend = "filesystem"
		p.config.LimitsConfig.WritePathOverrides.WritePath = "segment-writer"
		p.config.LimitsConfig.ReadPathOverrides.EnableQueryBackend = true
		p.config.SegmentWriter.LifecyclerConfig.MinReadyDuration = 0 * time.Second
		p.config.SegmentWriter.LifecyclerConfig.Addr = address
		p.config.SegmentWriter.MetadataUpdateTimeout = 0 * time.Second
		p.config.Metastore.MinReadyDuration = 0 * time.Second
		p.config.QueryBackend.Address = fmt.Sprintf("%s:%d", address, p.grpcPort)
		p.config.Metastore.Address = fmt.Sprintf("%s:%d", address, p.grpcPort)
		p.config.Metastore.Raft.ServerID = fmt.Sprintf("%s:%d", address, p.raftPort)
		p.config.Metastore.Raft.BindAddress = fmt.Sprintf("%s:%d", address, p.raftPort)
		p.config.Metastore.Raft.AdvertiseAddress = fmt.Sprintf("%s:%d", address, p.raftPort)
		p.config.Metastore.Raft.Dir = t.TempDir()
		p.config.Metastore.Raft.SnapshotsDir = t.TempDir()
		p.config.Metastore.FSM.DataDir = t.TempDir()
	}
	return p
}

func (p *PyroscopeTest) stop() {
	defer func() {
		prometheus.DefaultRegisterer = p.prevReg
	}()
	p.it.SignalHandler.Stop()
	p.wg.Wait()
}

func (p *PyroscopeTest) ready() bool {
	return httpBodyContains(p.URL()+"/ready", "ready")
}
func (p *PyroscopeTest) ringActive() bool {
	return httpBodyContains(p.URL()+"/ring", "ACTIVE")
}
func (p *PyroscopeTest) URL() string {
	return fmt.Sprintf("http://%s:%d", address, p.httpPort)
}

func (p *PyroscopeTest) Metrics(t testing.TB, keep func(string) bool) string {
	dto, err := p.reg.Gather()
	require.NoError(t, err)
	gotBuf := bytes.NewBuffer(nil)
	enc := expfmt.NewEncoder(gotBuf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range dto {
		if err := enc.Encode(mf); err != nil {
			require.NoError(t, err)
		}
	}
	split := strings.Split(gotBuf.String(), "\n")
	res := []string{}
	for _, line := range split {
		if keep(line) {
			res = append(res, line)
		}
	}
	return strings.Join(res, "\n")
}

func httpBodyContains(url string, needle string) bool {
	fmt.Println("httpBodyContains", url, needle)
	res, err := http.Get(url)
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

	return strings.Contains(body.String(), needle)
}

func (p *PyroscopeTest) NewRequestBuilder(t *testing.T) *RequestBuilder {
	return &RequestBuilder{
		t:       t,
		url:     p.URL(),
		AppName: p.TempAppName(),
		spy:     "foo239",
	}
}

func (p *PyroscopeTest) TempAppName() string {
	return fmt.Sprintf("pprof.integration.%d",
		rand.Uint64())
}

func createRenderQuery(metric, app string) string {
	return metric + "{service_name=\"" + app + "\"}"
}

type RequestBuilder struct {
	AppName string
	url     string
	spy     string
	t       *testing.T
}

func (b *RequestBuilder) Spy(spy string) *RequestBuilder {
	b.spy = spy
	return b
}

func (b *RequestBuilder) IngestPPROFRequest(profilePath, prevProfilePath, sampleTypeConfigPath string) *http.Request {
	var (
		profile, prevProfile, sampleTypeConfig []byte
		err                                    error
	)
	profile, err = os.ReadFile(profilePath)
	assert.NoError(b.t, err)
	if prevProfilePath != "" {
		prevProfile, err = os.ReadFile(prevProfilePath)
		assert.NoError(b.t, err)
	}
	if sampleTypeConfigPath != "" {
		sampleTypeConfig, err = os.ReadFile(sampleTypeConfigPath)
		assert.NoError(b.t, err)
	}

	const (
		formFieldProfile          = "profile"
		formFieldPreviousProfile  = "prev_profile"
		formFieldSampleTypeConfig = "sample_type_config"
	)

	var bb bytes.Buffer
	w := multipart.NewWriter(&bb)

	profileW, err := w.CreateFormFile(formFieldProfile, "not used")
	require.NoError(b.t, err)
	_, err = profileW.Write(profile)
	require.NoError(b.t, err)

	if sampleTypeConfig != nil {

		sampleTypeConfigW, err := w.CreateFormFile(formFieldSampleTypeConfig, "not used")
		require.NoError(b.t, err)
		_, err = sampleTypeConfigW.Write(sampleTypeConfig)
		require.NoError(b.t, err)
	}

	if prevProfile != nil {
		prevProfileW, err := w.CreateFormFile(formFieldPreviousProfile, "not used")
		require.NoError(b.t, err)
		_, err = prevProfileW.Write(prevProfile)
		require.NoError(b.t, err)
	}
	err = w.Close()
	require.NoError(b.t, err)

	bs := bb.Bytes()
	ct := w.FormDataContentType()

	url := b.url + "/ingest?name=" + b.AppName + "&spyName=" + b.spy
	req, err := http.NewRequest("POST", url, bytes.NewReader(bs))
	require.NoError(b.t, err)
	req.Header.Set("Content-Type", ct)
	return req
}

func (b *RequestBuilder) IngestJFRRequestFiles(jfrPath, labelsPath string) *http.Request {
	var (
		jfr, labels []byte
		err         error
	)
	jfr, err = os.ReadFile(jfrPath)
	assert.NoError(b.t, err)
	if labelsPath != "" {
		labels, err = os.ReadFile(labelsPath)
		assert.NoError(b.t, err)
	}

	return b.IngestJFRRequestBody(jfr, labels)
}

func (b *RequestBuilder) IngestJFRRequestBody(jfr []byte, labels []byte) *http.Request {
	var bb bytes.Buffer
	w := multipart.NewWriter(&bb)
	jfrw, err := w.CreateFormFile("jfr", "jfr")
	require.NoError(b.t, err)
	_, err = jfrw.Write(jfr)
	require.NoError(b.t, err)
	if labels != nil {
		labelsw, err := w.CreateFormFile("labels", "labels")
		require.NoError(b.t, err)
		_, err = labelsw.Write(labels)
		require.NoError(b.t, err)
	}
	err = w.Close()
	require.NoError(b.t, err)
	ct := w.FormDataContentType()
	bs := bb.Bytes()

	url := b.url + "/ingest?name=" + b.AppName + "&spyName=" + b.spy + "&format=jfr"
	req, err := http.NewRequest("POST", url, bytes.NewReader(bs))
	require.NoError(b.t, err)
	req.Header.Set("Content-Type", ct)

	return req
}

func (b *RequestBuilder) Render(metric string) *flamebearer.FlamebearerProfile {
	queryURL := b.url + "/pyroscope/render?query=" + createRenderQuery(metric, b.AppName) + "&from=946656000&until=now&format=collapsed"
	fmt.Println(queryURL)
	queryRes, err := http.Get(queryURL)
	require.NoError(b.t, err)
	body := bytes.NewBuffer(nil)
	_, err = io.Copy(body, queryRes.Body)
	assert.NoError(b.t, err)
	fb := new(flamebearer.FlamebearerProfile)
	err = json.Unmarshal(body.Bytes(), fb)
	assert.NoError(b.t, err, body.String(), queryURL)
	assert.Greater(b.t, len(fb.Flamebearer.Names), 1, body.String(), queryRes)
	assert.Greater(b.t, fb.Flamebearer.NumTicks, 1, body.String(), queryRes)
	// todo check actual stacktrace contents
	return fb
}

func (b *RequestBuilder) PushPPROFRequestFromFile(file string, metric string) *connect.Request[pushv1.PushRequest] {
	updateTimestamp := func(rawProfile []byte) []byte {
		expectedProfile, err := pprof.RawFromBytes(rawProfile)
		require.NoError(b.t, err)
		expectedProfile.TimeNanos = time.Now().Add(-time.Minute).UnixNano()
		buf := bytes.NewBuffer(nil)
		_, err = expectedProfile.WriteTo(buf)
		require.NoError(b.t, err)
		rawProfile = buf.Bytes()
		return rawProfile
	}

	rawProfile, err := os.ReadFile(file)
	require.NoError(b.t, err)

	rawProfile = updateTimestamp(rawProfile)

	metricName := strings.Split(metric, ":")[0]

	req := connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{{
			Labels: []*typesv1.LabelPair{
				{Name: "__name__", Value: metricName},
				{Name: "__delta__", Value: "false"},
				{Name: "service_name", Value: b.AppName},
			},
			Samples: []*pushv1.RawSample{{RawProfile: rawProfile}},
		}},
	})
	return req
}

func (b *RequestBuilder) PushPPROFRequestFromBytes(rawProfile []byte, name string) *connect.Request[pushv1.PushRequest] {
	req := connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{{
			Labels: []*typesv1.LabelPair{
				{Name: "__name__", Value: name},
				{Name: "service_name", Value: b.AppName},
			},
			Samples: []*pushv1.RawSample{{RawProfile: rawProfile}},
		}},
	})
	return req
}

func (b *RequestBuilder) QueryClient() querierv1connect.QuerierServiceClient {
	return querierv1connect.NewQuerierServiceClient(
		http.DefaultClient,
		b.url,
		connectapi.DefaultClientOptions()...,
	)
}

func (b *RequestBuilder) PushClient() pushv1connect.PusherServiceClient {
	return pushv1connect.NewPusherServiceClient(
		http.DefaultClient,
		b.url,
		connectapi.DefaultClientOptions()...,
	)
}

func (p *PyroscopeTest) Ingest(t *testing.T, req *http.Request, expectStatus int) {
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, expectStatus, res.StatusCode)
}

func (b *RequestBuilder) Push(request *connect.Request[pushv1.PushRequest], expectStatus int, expectedError string) {
	cl := b.PushClient()
	_, err := cl.Push(context.TODO(), request)
	if expectStatus == 200 {
		assert.NoError(b.t, err)
	} else {
		assert.Error(b.t, err)
		var connectErr *connect.Error
		if ok := errors.As(err, &connectErr); ok {
			toHTTP := connectgrpc.CodeToHTTP(connectErr.Code())
			assert.Equal(b.t, expectStatus, int(toHTTP))
			if expectedError != "" {
				assert.Contains(b.t, connectErr.Error(), expectedError)
			}
		} else {
			assert.Fail(b.t, "unexpected error type", err)
		}
	}
}

func (b *RequestBuilder) SelectMergeProfile(metric string, query map[string]string) *connect.Response[profilev1.Profile] {

	cnt := 0
	selector := strings.Builder{}
	add := func(k, v string) {
		if cnt > 0 {
			selector.WriteString(", ")
		}
		selector.WriteString(k)
		selector.WriteString("=")
		selector.WriteString("\"")
		selector.WriteString(v)
		selector.WriteString("\"")
		cnt++
	}
	selector.WriteString("{")
	if query["service_name"] == "" {
		add("service_name", b.AppName)
	}

	for k, v := range query {
		add(k, v)
	}
	selector.WriteString("}")
	qc := b.QueryClient()
	resp, err := qc.SelectMergeProfile(context.Background(), connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: metric,
		Start:         time.Unix(1, 0).UnixMilli(),
		End:           time.Now().UnixMilli(),
		LabelSelector: selector.String(),
	}))
	require.NoError(b.t, err)
	return resp
}

func (b *RequestBuilder) OtelPushClient() profilesv1.ProfilesServiceClient {
	grpcAddr := strings.TrimPrefix(b.url, "http://")

	conn, err := grpc.NewClient(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(b.t, err)

	return profilesv1.NewProfilesServiceClient(conn)
}
