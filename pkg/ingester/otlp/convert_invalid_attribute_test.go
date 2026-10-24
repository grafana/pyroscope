package otlp

import (
	"context"
	"testing"

	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1experimental "go.opentelemetry.io/proto/otlp/profiles/v1development"

	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockotlp"
)

// TestConvertOtelToGoogle_InvalidSampleAttributeIndex reproduces the bug where a
// sample referencing an out-of-bounds AttributeIndices entry makes
// ConvertOtelToGoogle return an empty, error-free result instead of surfacing
// the malformed input. A real conversion error must not be indistinguishable
// from the legitimate "zero samples" case.
func TestConvertOtelToGoogle_InvalidSampleAttributeIndex(t *testing.T) {
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
	// AttributeTable is empty, so index 0 is out of bounds.
	b.profile.Samples = []*v1experimental.Sample{{
		StackIndex:       0,
		Values:           []int64{100},
		AttributeIndices: []int32{0},
	}}

	_, err := ConvertOtelToGoogle(&b.profile, &b.dictionary)
	require.Error(t, err, "an out-of-bounds sample attribute index must surface as an error, not an empty success")
}

// TestExport_InvalidSampleAttributeIndex_RejectsAsClientError reproduces the
// end-to-end symptom: a malformed profile is silently dropped. PushBatch must
// never be called, and the client must see a visible error rather than a 200/OK
// "success" with no stored profile.
func TestExport_InvalidSampleAttributeIndex_RejectsAsClientError(t *testing.T) {
	svc := mockotlp.NewMockPushService(t)
	svc.On("PushBatch", mock.Anything, mock.Anything).Return(nil).Maybe()

	req := createValidOTLPRequest()
	// Reference an attribute index that doesn't exist in AttributeTable.
	req.ResourceProfiles[0].ScopeProfiles[0].Profiles[0].Samples[0].AttributeIndices = []int32{0}

	logger := test.NewTestingLogger(t)
	h := NewOTLPIngestHandler(testConfig(), svc, logger, defaultLimits())
	_, err := h.Export(user.InjectOrgID(context.Background(), tenant.DefaultTenantID), req)

	require.Error(t, err, "malformed sample attribute index must be rejected, not silently acknowledged")
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	svc.AssertNotCalled(t, "PushBatch", mock.Anything, mock.Anything)
}
