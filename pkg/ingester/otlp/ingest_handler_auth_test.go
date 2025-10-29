package otlp

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	v1experimental2 "go.opentelemetry.io/proto/otlp/collector/profiles/v1development"
	v1experimental "go.opentelemetry.io/proto/otlp/profiles/v1development"

	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockotlp"
)

// createValidOTLPRequest creates a minimal valid OTLP profile export request for testing
func createValidOTLPRequest() *v1experimental2.ExportProfilesServiceRequest {
	b := new(otlpbuilder)
	b.dictionary.MappingTable = []*v1experimental.Mapping{{
		MemoryStart:      0x1000,
		MemoryLimit:      0x2000,
		FilenameStrindex: b.addstr("test.so"),
	}}
	b.dictionary.LocationTable = []*v1experimental.Location{{
		MappingIndex: 0,
		Address:      0x1100,
	}}
	b.dictionary.StackTable = []*v1experimental.Stack{{
		LocationIndices: []int32{0},
	}}
	b.profile.SampleType = &v1experimental.ValueType{
		TypeStrindex: b.addstr("samples"),
		UnitStrindex: b.addstr("count"),
	}
	b.profile.Sample = []*v1experimental.Sample{{
		StackIndex: 0,
		Values:     []int64{100},
	}}
	b.profile.TimeUnixNano = 1234567890

	return &v1experimental2.ExportProfilesServiceRequest{
		ResourceProfiles: []*v1experimental.ResourceProfiles{{
			ScopeProfiles: []*v1experimental.ScopeProfiles{{
				Profiles: []*v1experimental.Profile{&b.profile},
			}},
		}},
		Dictionary: &b.dictionary,
	}
}

func TestMultitenancyDisabled_GRPCRequestsPassThrough(t *testing.T) {
	// Setup mock service
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	// Create handler with multitenancy disabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, false)

	// Test gRPC request without tenant metadata
	ctx := context.Background()
	req := createValidOTLPRequest()

	resp, err := h.Export(ctx, req)

	// Verify request succeeds and default tenant is used
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, tenant.DefaultTenantID, capturedTenantID)
}

func TestMultitenancyDisabled_HTTPRequestsPassThrough(t *testing.T) {
	// Setup mock service
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	// Create handler with multitenancy disabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, false)

	// Create HTTP request without tenant header
	req := createValidOTLPRequest()
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader(reqBytes))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httpReq)

	// Verify request succeeds and default tenant is used
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, tenant.DefaultTenantID, capturedTenantID)
}

func TestMultitenancyEnabled_GRPCRequestWithoutTenantRejected(t *testing.T) {
	// Setup mock service (should not be called)
	svc := mockotlp.NewMockPushService(t)

	// Create handler with multitenancy enabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, true)

	// Test gRPC request without tenant metadata
	ctx := context.Background()
	req := createValidOTLPRequest()

	resp, err := h.Export(ctx, req)

	// Verify request is rejected
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to extract tenant ID")
	assert.NotNil(t, resp) // Response object is returned but with error
}

func TestMultitenancyEnabled_GRPCRequestWithTenantAccepted(t *testing.T) {
	// Setup mock service
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	// Create handler with multitenancy enabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, true)

	// Test gRPC request with tenant metadata
	// Use dskit's user package to inject tenant ID into gRPC metadata
	md := metadata.Pairs(user.OrgIDHeaderName, "test-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	req := createValidOTLPRequest()

	resp, err := h.Export(ctx, req)

	// Verify request succeeds with correct tenant
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "test-tenant", capturedTenantID)
}

func TestMultitenancyEnabled_HTTPRequestWithoutTenantRejected(t *testing.T) {
	// Setup mock service (should not be called)
	svc := mockotlp.NewMockPushService(t)

	// Create handler with multitenancy enabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, true)

	// Create HTTP request without tenant header
	req := createValidOTLPRequest()
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader(reqBytes))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httpReq)

	// Verify request is rejected with 401 Unauthorized
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to extract tenant ID")
}

func TestMultitenancyEnabled_HTTPRequestWithTenantAccepted(t *testing.T) {
	// Setup mock service
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	// Create handler with multitenancy enabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, true)

	// Create HTTP request with tenant header
	req := createValidOTLPRequest()
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader(reqBytes))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set(user.OrgIDHeaderName, "test-tenant")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httpReq)

	// Verify request succeeds with correct tenant
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test-tenant", capturedTenantID)
}

func TestMultitenancyEnabled_HTTPRequestWithJSONAndTenantAccepted(t *testing.T) {
	// Setup mock service
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	// Create handler with multitenancy enabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, true)

	// Create a minimal JSON request
	jsonRequest := `{
		"resourceProfiles": [{
			"scopeProfiles": [{
				"profiles": [{
					"sampleType": {"typeStrindex": 0, "unitStrindex": 1},
					"sample": [{"stackIndex": 0, "values": [100]}],
					"timeUnixNano": "1234567890"
				}]
			}]
		}],
		"dictionary": {
			"stringTable": ["samples", "count", "test.so"],
			"mappingTable": [{"memoryStart": "4096", "memoryLimit": "8192", "filenameStrindex": 2}],
			"locationTable": [{"mappingIndex": 0, "address": "4352"}],
			"stackTable": [{"locationIndices": [0]}]
		}
	}`

	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader([]byte(jsonRequest)))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(user.OrgIDHeaderName, "json-tenant")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httpReq)

	// Verify request succeeds with correct tenant
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "json-tenant", capturedTenantID)
}

func TestMultitenancyEnabled_GRPCRequestWithAlternateHeader(t *testing.T) {
	// Setup mock service
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	// Create handler with multitenancy enabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, true)

	// Test gRPC request with lowercase header
	md := metadata.Pairs(strings.ToLower(user.OrgIDHeaderName), "alternate-tenant")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	req := createValidOTLPRequest()

	resp, err := h.Export(ctx, req)

	// Verify request succeeds with correct tenant
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "alternate-tenant", capturedTenantID)
}

func TestHTTPRequestWithGzipCompression(t *testing.T) {
	// Setup mock service
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	// Create handler with multitenancy disabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, false)

	// Create a protobuf request
	req := createValidOTLPRequest()
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err)

	// Compress the request body with gzip
	var gzipBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuf)
	_, err = gzipWriter.Write(reqBytes)
	require.NoError(t, err)
	err = gzipWriter.Close()
	require.NoError(t, err)

	// Create HTTP request with gzip-encoded body
	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader(gzipBuf.Bytes()))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("Content-Encoding", "gzip")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httpReq)

	// Verify request succeeds and default tenant is used
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, tenant.DefaultTenantID, capturedTenantID)
}

func TestHTTPRequestWithGzipCompressionAndJSON(t *testing.T) {
	// Setup mock service
	svc := mockotlp.NewMockPushService(t)
	var capturedTenantID string
	svc.On("PushBatch", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		require.NoError(t, err)
		capturedTenantID = tenantID
	}).Return(nil, nil)

	// Create handler with multitenancy enabled
	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, true)

	// Create a minimal JSON request
	jsonRequest := `{
		"resourceProfiles": [{
			"scopeProfiles": [{
				"profiles": [{
					"sampleType": {"typeStrindex": 0, "unitStrindex": 1},
					"sample": [{"stackIndex": 0, "values": [100]}],
					"timeUnixNano": "1234567890"
				}]
			}]
		}],
		"dictionary": {
			"stringTable": ["samples", "count", "test.so"],
			"mappingTable": [{"memoryStart": "4096", "memoryLimit": "8192", "filenameStrindex": 2}],
			"locationTable": [{"mappingIndex": 0, "address": "4352"}],
			"stackTable": [{"locationIndices": [0]}]
		}
	}`

	// Compress the JSON request body with gzip
	var gzipBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuf)
	_, err := gzipWriter.Write([]byte(jsonRequest))
	require.NoError(t, err)
	err = gzipWriter.Close()
	require.NoError(t, err)

	// Create HTTP request with gzip-encoded JSON body
	httpReq := httptest.NewRequest("POST", "/otlp/v1/profiles", bytes.NewReader(gzipBuf.Bytes()))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Content-Encoding", "gzip")
	httpReq.Header.Set(user.OrgIDHeaderName, "gzip-json-tenant")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httpReq)

	// Verify request succeeds with correct tenant
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip-json-tenant", capturedTenantID)
}
