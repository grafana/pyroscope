package profiledump

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestRecorderMetadataRoundTrip(t *testing.T) {
	metadata := Metadata{SchemaVersion: schemaVersion, TenantID: "3648", SourceProtocol: sourceConnect, Format: formatPprof}
	metadata.CapturedAt = time.Date(2026, 9, 25, 23, 59, 59, 123456789, time.FixedZone("offset", -7*60*60))
	metadata.DistributorID = "distributor\xff\n\x00"
	metadata.OriginalProfileID = "profile\xfe\n\x00"
	metadata.Labels = []*typesv1.LabelPair{
		{Name: "", Value: "empty name"},
		{Name: "empty value", Value: ""},
		{Name: "\xff", Value: "\xfe"},
		{Name: "control\nname", Value: "tab\tvalue\x00\u0085<>&"},
		{Name: "duplicate", Value: "first"},
		{Name: "duplicate", Value: "second"},
		nil,
	}
	r, err := NewRecorder(DefaultRecorderConfig(), &policySet{}, discardUpload, metadata.DistributorID, log.NewNopLogger(), nil)
	require.NoError(t, err)
	item, reason := r.prepareCapture(metadata.TenantID, metadata, nil, metadata.CapturedAt)
	require.Empty(t, reason)
	r.release(item)
	var decoded Metadata
	require.NoError(t, json.Unmarshal(item.metadataJSON, &decoded))
	metadata.DistributorID = "distributor\ufffd\n\x00"
	metadata.OriginalProfileID = "profile\ufffd\n\x00"
	metadata.Labels[2] = &typesv1.LabelPair{Name: "\ufffd", Value: "\ufffd"}
	metadata.CaptureID = decoded.CaptureID
	metadata.CapturedAt = metadata.CapturedAt.UTC()
	require.Equal(t, metadata, decoded)
	r, err = NewRecorder(DefaultRecorderConfig(), &policySet{}, discardUpload, "", log.NewNopLogger(), nil)
	require.NoError(t, err)
	item, reason = r.prepareCapture("a", captureMetadata(), nil, recorderNow)
	require.Empty(t, reason)
	defer r.release(item)
	var withoutDistributor Metadata
	require.NoError(t, json.Unmarshal(item.metadataJSON, &withoutDistributor))
	require.Empty(t, withoutDistributor.DistributorID)
}
