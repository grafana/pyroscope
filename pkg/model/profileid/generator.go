package profileid

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"slices"

	"github.com/google/uuid"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

// Source identifies how a profile ID was generated.
type Source string

const (
	SourceUserSupplied Source = "user_supplied"
	SourceTimestamp    Source = "timestamp"
	SourceTraceID      Source = "trace_id"
	SourceRandom       Source = "random"
)

// Generate creates a UUID v8 from the SHA-256 hash of the profile's external
// identity. A timestamp takes precedence over a trace ID. When neither is
// available, a random UUID v4 is returned.
func Generate(
	tenantID string,
	profileType string,
	externalLabels []*typesv1.LabelPair,
	timeNanos int64,
	traceID string,
) (uuid.UUID, Source) {
	if timeNanos == 0 && traceID == "" {
		return uuid.New(), SourceRandom
	}

	h := sha256.New()
	writeString(h, tenantID)
	writeString(h, profileType)

	sortedLabels := slices.Clone(externalLabels)
	slices.SortFunc(sortedLabels, model.CompareLabelPairs2)
	writeUint64(h, uint64(len(sortedLabels)))
	for _, label := range sortedLabels {
		writeString(h, label.Name)
		writeString(h, label.Value)
	}

	if timeNanos != 0 {
		writeUint64(h, uint64(timeNanos))
		return uuidFromHash(h), SourceTimestamp
	}

	writeString(h, traceID)
	return uuidFromHash(h), SourceTraceID
}

func uuidFromHash(h hash.Hash) uuid.UUID {
	sum := h.Sum(nil)

	// Use the first 16 bytes of SHA-256 with the version and variant bits set
	// for a custom UUID v8 (RFC 9562).
	var uuidBytes [16]byte
	copy(uuidBytes[:], sum[:16])
	uuidBytes[6] = (uuidBytes[6] & 0x0f) | 0x80
	uuidBytes[8] = (uuidBytes[8] & 0x3f) | 0x80

	id, _ := uuid.FromBytes(uuidBytes[:])
	return id
}

func writeString(h hash.Hash, s string) {
	writeUint64(h, uint64(len(s)))
	_, _ = h.Write([]byte(s))
}

func writeUint64(h hash.Hash, n uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], n)
	_, _ = h.Write(buf[:])
}
