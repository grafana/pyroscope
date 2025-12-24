package distributor_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/server"
	grpcgw "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/require"
	otlpcolv1 "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	otlpv1 "go.opentelemetry.io/proto/otlp/profiles/v1development"
	"google.golang.org/protobuf/encoding/protojson"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/api"
	"github.com/grafana/pyroscope/pkg/distributor"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/validation"
)

type apiTest struct {
	*api.API
	HTTPMux   *mux.Router
	GRPCGWMux *grpcgw.ServeMux
}

func newAPITest(t testing.TB, cfg api.Config, logger log.Logger) *apiTest {
	a := &apiTest{
		HTTPMux:   mux.NewRouter(),
		GRPCGWMux: grpcgw.NewServeMux(),
	}

	cfg.HTTPAuthMiddleware = util.AuthenticateUser(true)
	cfg.GrpcAuthMiddleware = connect.WithInterceptors(tenant.NewAuthInterceptor(true))

	serv := &server.Server{
		HTTP: a.HTTPMux,
	}

	var err error
	a.API, err = api.New(
		cfg,
		serv,
		a.GRPCGWMux,
		logger,
	)
	if err != nil {
		t.Error("failed to create api: ", err)
	}
	return a
}

func defaultLimits() *validation.Overrides {
	return validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.IngestionBodyLimitMB = 1 // 1 MB
		l.IngestionRateMB = 10000  // 100 MB
		tenantLimits["1mb-body-limit"] = l

	})
}

// generateProfileOfSize creates a valid pprof profile with approximately the target size in bytes.
// It creates a minimal profile and pads the string table to reach the desired size.
func generateProfileOfSize(targetSize int, compress bool) ([]byte, error) {
	// Create a minimal valid profile
	p := &profilev1.Profile{
		SampleType: []*profilev1.ValueType{
			{Type: 1, Unit: 2},
		},
		Sample: []*profilev1.Sample{
			{LocationId: []uint64{1}, Value: []int64{100}},
		},
		Location: []*profilev1.Location{
			{Id: 1, MappingId: 1, Line: []*profilev1.Line{{FunctionId: 1, Line: 1}}},
		},
		Mapping: []*profilev1.Mapping{
			{Id: 1, Filename: 3},
		},
		Function: []*profilev1.Function{
			{Id: 1, Name: 4, SystemName: 4, Filename: 3},
		},
		StringTable: []string{
			"",
			"cpu", "nanoseconds",
			"main.go",
			"foo",
		},
		PeriodType: &profilev1.ValueType{Type: 1, Unit: 2},
		TimeNanos:  1,
		Period:     1,
	}

	// Marshal to see the base size
	baseData, err := pprof.Marshal(p, false)
	if err != nil {
		return nil, err
	}

	// If we need more size, pad the string table
	if len(baseData) < targetSize {
		// Add a large padding string to reach target size
		// Each character in the string table adds roughly 1 byte
		paddingSize := targetSize - len(baseData)
		p.StringTable[4] = "f" + strings.Repeat("o", paddingSize)
	}

	return pprof.Marshal(p, compress)
}

func pprofWithSize(t testing.TB, target int) []byte {
	t.Helper()
	uncompressed, err := generateProfileOfSize(target, false)
	require.NoError(t, err)
	require.Equal(t, target, len(uncompressed))
	return uncompressed
}

func reqIngestGzPprof(data []byte) func(context.Context) *http.Request {
	requestF := reqIngestPprof(data)
	return func(ctx context.Context) *http.Request {
		req := requestF(ctx)
		req.Header.Set("Content-Encoding", "gzip")
		return req
	}
}

func reqIngestPprof(data []byte) func(context.Context) *http.Request {
	return func(ctx context.Context) *http.Request {
		req, err := http.NewRequestWithContext(
			ctx, "POST", "/ingest?name=testapp&format=pprof",
			bytes.NewReader(data),
		)
		if err != nil {
			panic(err)
		}

		return req
	}
}

func reqPushPprofJson(profiles ...[][]byte) func(context.Context) *http.Request {
	req := &pushv1.PushRequest{}

	for pIdx, p := range profiles {
		s := &pushv1.RawProfileSeries{
			Labels: []*typesv1.LabelPair{
				{Name: "__name__", Value: "fake_cpu"},
				{Name: "service_name", Value: "killer"},
			},
		}
		for sIdx, sample := range p {
			s.Samples = append(s.Samples, &pushv1.RawSample{
				ID:         fmt.Sprintf("%d-%d", pIdx, sIdx),
				RawProfile: sample,
			})
		}
		req.Series = append(req.Series, s)
	}

	jsonData, err := protojson.Marshal(req)
	if err != nil {
		panic(err)
	}

	return func(ctx context.Context) *http.Request {
		req, err := http.NewRequestWithContext(
			ctx, "POST", "/push.v1.PusherService/Push",
			bytes.NewReader(jsonData),
		)
		if err != nil {
			panic(err)
		}
		req.Header.Set("Content-Type", "application/json")
		return req
	}
}

func gzipper(t testing.TB, fn func(testing.TB, int) []byte, targetSize int) []byte {
	t.Helper()
	data := fn(t, targetSize)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := io.Copy(zw, bytes.NewReader(data))
	require.NoError(t, err)
	require.NoError(t, zw.Close()) // Must close to flush and write gzip trailer
	return buf.Bytes()
}

// otlpbuilder helps build OTLP profiles with controlled sizes
type otlpbuilder struct {
	profile    otlpv1.Profile
	dictionary otlpv1.ProfilesDictionary
	stringmap  map[string]int32
}

func (o *otlpbuilder) addstr(s string) int32 {
	if o.stringmap == nil {
		o.stringmap = make(map[string]int32)
	}
	if idx, ok := o.stringmap[s]; ok {
		return idx
	}
	idx := int32(len(o.stringmap))
	o.stringmap[s] = idx
	o.dictionary.StringTable = append(o.dictionary.StringTable, s)
	return idx
}

func otlpJSONWithSize(t testing.TB, targetSize int) []byte {
	t.Helper()

	b := new(otlpbuilder)

	fileNameIdx := b.addstr("foo")

	// Create minimal valid OTLP profile structure with cpu:nanoseconds type (maps to process_cpu)
	b.dictionary.MappingTable = []*otlpv1.Mapping{{
		MemoryStart:      0x1000,
		MemoryLimit:      0x2000,
		FilenameStrindex: fileNameIdx,
	}}
	b.dictionary.LocationTable = []*otlpv1.Location{{
		MappingIndex: 0,
		Address:      0x1100,
	}}
	b.dictionary.StackTable = []*otlpv1.Stack{{
		LocationIndices: []int32{0},
	}}
	// Use cpu:nanoseconds which will be recognized as a valid profile type
	cpuIdx := b.addstr("cpu")
	nanosIdx := b.addstr("nanoseconds")
	b.profile.SampleType = &otlpv1.ValueType{
		TypeStrindex: cpuIdx,
		UnitStrindex: nanosIdx,
	}
	b.profile.PeriodType = &otlpv1.ValueType{
		TypeStrindex: cpuIdx,
		UnitStrindex: nanosIdx,
	}
	b.profile.Period = 10000000
	b.profile.Sample = []*otlpv1.Sample{{
		StackIndex: 0,
		Values:     []int64{100},
	}}
	b.profile.TimeUnixNano = 1234567890

	// Calculate current size
	req := &otlpcolv1.ExportProfilesServiceRequest{
		ResourceProfiles: []*otlpv1.ResourceProfiles{{
			ScopeProfiles: []*otlpv1.ScopeProfiles{{
				Profiles: []*otlpv1.Profile{&b.profile},
			}},
		}},
		Dictionary: &b.dictionary,
	}
	jsonData, err := protojson.Marshal(req)
	require.NoError(t, err)
	baseSize := len(jsonData)

	// Pad string table to reach target size
	if baseSize < targetSize {
		paddingSize := targetSize - baseSize - 1
		b.dictionary.StringTable[fileNameIdx] += "f" + strings.Repeat("o", paddingSize)
	}

	jsonData, err = protojson.Marshal(req)
	require.NoError(t, err)
	if len(jsonData) != targetSize {
		panic(fmt.Sprintf("json size=%d is not matching targetSize=%d", len(jsonData), targetSize))
	}

	return jsonData
}

func reqOTLPJson(data []byte) func(context.Context) *http.Request {
	return func(ctx context.Context) *http.Request {
		httpReq, err := http.NewRequestWithContext(
			ctx, "POST", "/v1development/profiles",
			bytes.NewReader(data),
		)
		if err != nil {
			panic(err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		return httpReq
	}
}

func reqOTLPJsonGzip(data []byte) func(context.Context) *http.Request {
	fn := reqOTLPJson(data)
	return func(ctx context.Context) *http.Request {
		req := fn(ctx)
		req.Header.Set("Content-Encoding", "gzip")
		return req
	}
}

const (
	underOneMb = (1024 - 10) * 1024
	oneMb      = 1024 * 1024
	overOneMb  = (1024 + 10) * 1024
)

func TestDistributorAPIBodySizeLimit(t *testing.T) {
	logger := log.NewNopLogger()

	limits := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.IngestionBodyLimitMB = 1 // 1 MB
		l.IngestionRateMB = 10000  // set this high enough to not interfere
		tenantLimits["1mb-body-limit"] = l

	})

	d, err := distributor.NewTestDistributor(t,
		logger,
		defaultLimits(),
	)
	require.NoError(t, err)

	a := newAPITest(t, api.Config{}, logger)
	a.RegisterDistributor(d, limits, server.Config{})

	// generate sample payloads
	var (
		pprofUnderOneMb        = pprofWithSize(t, underOneMb)
		pprofOneMb             = pprofWithSize(t, oneMb)
		pprofOverOneMb         = pprofWithSize(t, overOneMb)
		pprofGzipUnderOneMb    = gzipper(t, pprofWithSize, underOneMb)
		pprofGzipOneMb         = gzipper(t, pprofWithSize, oneMb)
		pprofGzipOverOneMb     = gzipper(t, pprofWithSize, overOneMb)
		otlpJSONUnderOneMb     = otlpJSONWithSize(t, underOneMb)
		otlpJSONnOneMb         = otlpJSONWithSize(t, oneMb)
		otlpJSONOverOneMb      = otlpJSONWithSize(t, overOneMb)
		otlpJSONGzipUnderOneMb = gzipper(t, otlpJSONWithSize, underOneMb)
		otlpJSONGzipOneMb      = gzipper(t, otlpJSONWithSize, oneMb)
		otlpJSONGzipOverOneMb  = gzipper(t, otlpJSONWithSize, overOneMb)
	)

	testCases := []struct {
		name             string
		skipMsg          string
		request          func(context.Context) *http.Request
		tenantID         string
		expectedStatus   int
		expectedErrorMsg string
	}{
		{
			name:           "ingest/uncompressed/within-limit",
			request:        reqIngestPprof(pprofUnderOneMb),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:           "ingest/uncompressed/exact-limit",
			request:        reqIngestPprof(pprofOneMb),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:             "ingest/uncompressed/exceeds-limit",
			request:          reqIngestPprof(pprofOverOneMb),
			tenantID:         "1mb-body-limit",
			expectedStatus:   413,
			expectedErrorMsg: "request body too large",
		},
		{
			name:           "ingest/gzip/within-limit",
			request:        reqIngestGzPprof(pprofGzipUnderOneMb),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:           "ingest/gzip/exact-limit",
			request:        reqIngestGzPprof(pprofGzipOneMb),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:             "ingest/gzip/exceeds-limit",
			request:          reqIngestGzPprof(pprofGzipOverOneMb),
			tenantID:         "1mb-body-limit",
			expectedStatus:   413,
			expectedErrorMsg: "request body too large",
			skipMsg:          "TODO: Is it expected that this is not enforced?",
		},
		{
			name:           "push-json/uncompressed/within-limit",
			request:        reqPushPprofJson([][]byte{pprofUnderOneMb}),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:           "push-json/uncompressed/exact-limit",
			request:        reqPushPprofJson([][]byte{pprofOneMb}),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:             "push-json/uncompressed/exceeds-limit",
			request:          reqPushPprofJson([][]byte{pprofOverOneMb}),
			tenantID:         "1mb-body-limit",
			expectedStatus:   400, // grpc status codes used by connect have no mapping to 413
			expectedErrorMsg: "uncompressed batched profile payload size exceeds limit of 1.0 MB",
		},
		{
			name:             "push-json/uncompressed/exceeds-limit-with-two-profiles",
			request:          reqPushPprofJson([][]byte{pprofUnderOneMb}, [][]byte{pprofUnderOneMb}),
			tenantID:         "1mb-body-limit",
			expectedStatus:   400, // grpc status codes used by connect have no mapping to 413
			expectedErrorMsg: "uncompressed batched profile payload size exceeds limit of 1.0 MB",
		},
		{
			name:           "push-json/gzip/within-limit",
			request:        reqPushPprofJson([][]byte{pprofGzipUnderOneMb}),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:           "push-json/gzip/exact-limit",
			request:        reqPushPprofJson([][]byte{pprofGzipOneMb}),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:             "push-json/gzip/exceeds-limit",
			request:          reqPushPprofJson([][]byte{pprofGzipOverOneMb}),
			tenantID:         "1mb-body-limit",
			expectedStatus:   400, // grpc status codes used by connect have no mapping to 413
			expectedErrorMsg: "uncompressed batched profile payload size exceeds limit of 1.0 MB",
		},
		{
			name:             "push-json/gzip/exceeds-limit-with-two-profiles",
			request:          reqPushPprofJson([][]byte{pprofGzipUnderOneMb}, [][]byte{pprofGzipUnderOneMb}),
			tenantID:         "1mb-body-limit",
			expectedStatus:   400, // grpc status codes used by connect have no mapping to 413
			expectedErrorMsg: "uncompressed batched profile payload size exceeds limit of 1.0 MB",
		},
		{
			name:           "otlp-json/uncompressed/within-limit",
			request:        reqOTLPJson(otlpJSONUnderOneMb),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:           "otlp-json/uncompressed/exact-limit",
			request:        reqOTLPJson(otlpJSONnOneMb),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:             "otlp-json/uncompressed/exceeds-limit",
			request:          reqOTLPJson(otlpJSONOverOneMb),
			tenantID:         "1mb-body-limit",
			expectedStatus:   413,
			expectedErrorMsg: "profile payload size exceeds limit of 1.0 MB",
		},
		{
			name:           "otlp-json/gzip/within-limit",
			request:        reqOTLPJsonGzip(otlpJSONGzipUnderOneMb),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:           "otlp-json/gzip/exact-limit",
			request:        reqOTLPJsonGzip(otlpJSONGzipOneMb),
			tenantID:       "1mb-body-limit",
			expectedStatus: 200,
		},
		{
			name:             "otlp-json/gzip/exceeds-limit",
			request:          reqOTLPJsonGzip(otlpJSONGzipOverOneMb),
			tenantID:         "1mb-body-limit",
			expectedStatus:   413,
			expectedErrorMsg: "uncompressed profile payload size exceeds limit of 1.0 MB",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipMsg != "" {
				t.Skip(tc.skipMsg)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := tc.request(ctx)
			req.Header.Set("X-Scope-OrgID", tc.tenantID)

			// Execute request through the mux
			res := httptest.NewRecorder()
			a.HTTPMux.ServeHTTP(res, req)

			// Assertions
			require.Equal(t, tc.expectedStatus, res.Code,
				"expected status %d, got %d. Response body: %s",
				tc.expectedStatus, res.Code, res.Body.String())

			if tc.expectedErrorMsg != "" {
				require.Contains(t, res.Body.String(), tc.expectedErrorMsg)
			}

			// TODO: Check if there are metrics are collecting information about those discarded metrics
		})
	}

}

func TestDistributorAPIMaxProfileSizeBytes(t *testing.T) {
	logger := log.NewNopLogger()

	limits := validation.MockOverrides(func(defaults *validation.Limits, tenantLimits map[string]*validation.Limits) {
		l := validation.MockDefaultLimits()
		l.MaxProfileSizeBytes = 1024 * 1024 // 1 MB
		l.IngestionRateMB = 10000           // set this high enough to not interfere
		tenantLimits["1mb-profile-limit"] = l
	})

	d, err := distributor.NewTestDistributor(t,
		logger,
		limits,
	)
	require.NoError(t, err)

	a := newAPITest(t, api.Config{}, logger)
	a.RegisterDistributor(d, limits, server.Config{})

	// generate sample payloads
	var (
		pprofUnderOneMb        = pprofWithSize(t, underOneMb)
		pprofOverOneMb         = pprofWithSize(t, overOneMb)
		pprofGzipUnderOneMb    = gzipper(t, pprofWithSize, underOneMb)
		pprofGzipOverOneMb     = gzipper(t, pprofWithSize, overOneMb)
		otlpJSONUnderOneMb     = otlpJSONWithSize(t, underOneMb)
		otlpJSONOverOneMb      = otlpJSONWithSize(t, overOneMb)
		otlpJSONGzipUnderOneMb = gzipper(t, otlpJSONWithSize, underOneMb)
		otlpJSONGzipOverOneMb  = gzipper(t, otlpJSONWithSize, overOneMb)
	)

	testCases := []struct {
		name             string
		skipMsg          string
		request          func(context.Context) *http.Request
		tenantID         string
		expectedStatus   int
		expectedErrorMsg string
	}{
		{
			name:           "ingest/uncompressed/within-limit",
			request:        reqIngestPprof(pprofUnderOneMb),
			tenantID:       "1mb-profile-limit",
			expectedStatus: 200,
		},
		{
			name:           "ingest/uncompressed/exact-limit",
			request:        reqIngestPprof(pprofWithSize(t, 1024*1024-8)), // Note the extra 8 byte are used up by added metadata
			tenantID:       "1mb-profile-limit",
			expectedStatus: 200,
		},
		{
			name:             "ingest/uncompressed/exceeds-limit",
			request:          reqIngestPprof(pprofOverOneMb),
			tenantID:         "1mb-profile-limit",
			expectedStatus:   422,
			expectedErrorMsg: "exceeds maximum allowed size",
		},
		{
			name:           "ingest/gzip/within-limit",
			request:        reqIngestGzPprof(pprofGzipUnderOneMb),
			tenantID:       "1mb-profile-limit",
			expectedStatus: 200,
		},
		{
			name:           "ingest/gzip/exact-limit",
			request:        reqIngestPprof(gzipper(t, pprofWithSize, 1024*1024-8)), // Note the extra 8 byte are used up by added metadata
			tenantID:       "1mb-profile-limit",
			expectedStatus: 200,
		},
		{
			name:             "ingest/gzip/exceeds-limit",
			request:          reqIngestGzPprof(pprofGzipOverOneMb),
			tenantID:         "1mb-profile-limit",
			expectedStatus:   422,
			expectedErrorMsg: "exceeds maximum allowed size",
		},
		{
			name:           "push-json/uncompressed/within-limit",
			request:        reqPushPprofJson([][]byte{pprofUnderOneMb}),
			tenantID:       "1mb-profile-limit",
			expectedStatus: 200,
		},
		{
			name:             "push-json/uncompressed/exceeds-limit",
			request:          reqPushPprofJson([][]byte{pprofOverOneMb}),
			tenantID:         "1mb-profile-limit",
			expectedStatus:   400,
			expectedErrorMsg: "uncompressed profile payload size exceeds limit of 1.0 MB",
		},
		{
			name:           "push-json/gzip/within-limit",
			request:        reqPushPprofJson([][]byte{pprofGzipUnderOneMb}),
			tenantID:       "1mb-profile-limit",
			expectedStatus: 200,
		},
		{
			name:             "push-json/gzip/exceeds-limit",
			request:          reqPushPprofJson([][]byte{pprofGzipOverOneMb}),
			tenantID:         "1mb-profile-limit",
			expectedStatus:   400,
			expectedErrorMsg: "uncompressed profile payload size exceeds limit of 1.0 MB",
		},
		{
			name:           "otlp-json/uncompressed/within-limit",
			request:        reqOTLPJson(otlpJSONUnderOneMb),
			tenantID:       "1mb-profile-limit",
			expectedStatus: 200,
		},
		{
			name:             "otlp-json/uncompressed/exceeds-limit",
			request:          reqOTLPJson(otlpJSONOverOneMb),
			tenantID:         "1mb-profile-limit",
			expectedStatus:   400,
			expectedErrorMsg: "exceeds the size limit",
		},
		{
			name:           "otlp-json/gzip/within-limit",
			request:        reqOTLPJsonGzip(otlpJSONGzipUnderOneMb),
			tenantID:       "1mb-profile-limit",
			expectedStatus: 200,
		},
		{
			name:             "otlp-json/gzip/exceeds-limit",
			request:          reqOTLPJsonGzip(otlpJSONGzipOverOneMb),
			tenantID:         "1mb-profile-limit",
			expectedStatus:   400,
			expectedErrorMsg: "exceeds the size limit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipMsg != "" {
				t.Skip(tc.skipMsg)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := tc.request(ctx)
			req.Header.Set("X-Scope-OrgID", tc.tenantID)

			// Execute request through the mux
			res := httptest.NewRecorder()
			a.HTTPMux.ServeHTTP(res, req)

			// Assertions
			require.Equal(t, tc.expectedStatus, res.Code,
				"expected status %d, got %d. Response body: %s",
				tc.expectedStatus, res.Code, res.Body.String())

			if tc.expectedErrorMsg != "" {
				require.Contains(t, res.Body.String(), tc.expectedErrorMsg)
			}

			// TODO: Check if there are metrics are collecting information about those discarded metrics
		})
	}

}
