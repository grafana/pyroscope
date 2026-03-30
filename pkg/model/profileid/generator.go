package profileid

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/google/uuid"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// GenerateFromRequest creates a deterministic UUID based on request components.
// Hash priority:
// 1. Always: tenantID + sorted labels + raw profile bytes
// 2. If originalTimeNanos > 0: add TimeNanos
// 3. Else if traceID is non-empty: add traceID
// 4. Else: content-only hash
func GenerateFromRequest(
	tenantID string,
	labels []*typesv1.LabelPair,
	rawProfile []byte,
	originalTimeNanos int64,
	traceID string,
) uuid.UUID {
	h := sha256.New()

	// Write tenant ID
	h.Write([]byte(tenantID))

	// Write sorted labels (for consistency)
	sortedLabels := sortLabels(labels)
	for _, label := range sortedLabels {
		h.Write([]byte(label.Name))
		h.Write([]byte{0}) // separator
		h.Write([]byte(label.Value))
		h.Write([]byte{0}) // separator
	}

	// Write raw profile bytes
	h.Write(rawProfile)

	// Write temporal/trace context for uniqueness
	if originalTimeNanos > 0 {
		// Prefer explicit timestamp if provided
		var timeBytes [8]byte
		binary.LittleEndian.PutUint64(timeBytes[:], uint64(originalTimeNanos))
		h.Write(timeBytes[:])
	} else if traceID != "" {
		// Fall back to trace ID for uniqueness within trace
		h.Write([]byte(traceID))
	}
	// else: pure content hash (same content = same ID)

	sum := h.Sum(nil)

	// Convert SHA256 hash to UUID v5 format
	// Use first 16 bytes of hash
	var uuidBytes [16]byte
	copy(uuidBytes[:], sum[:16])

	// Set version (5) and variant bits according to RFC 4122
	uuidBytes[6] = (uuidBytes[6] & 0x0f) | 0x50 // Version 5
	uuidBytes[8] = (uuidBytes[8] & 0x3f) | 0x80 // Variant bits

	id, _ := uuid.FromBytes(uuidBytes[:])
	return id
}

// GenerateRandom creates a random UUID v4
func GenerateRandom() uuid.UUID {
	return uuid.New()
}

// sortLabels returns a sorted copy of labels for consistent hashing
func sortLabels(labels []*typesv1.LabelPair) []*typesv1.LabelPair {
	sorted := make([]*typesv1.LabelPair, len(labels))
	copy(sorted, labels)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Value < sorted[j].Value
	})
	return sorted
}
