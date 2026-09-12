package profileid

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"sort"

	"github.com/google/uuid"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// GenerateFromTrace creates a deterministic UUID from the request trace and
// the profile's ingress metadata.
func GenerateFromTrace(
	tenantID string,
	traceID string,
	labels []*typesv1.LabelPair,
	timeNanos int64,
	position uint64,
) uuid.UUID {
	h := sha256.New()

	writeString(h, tenantID)
	writeString(h, traceID)

	sortedLabels := sortLabels(labels)
	writeUint64(h, uint64(len(sortedLabels)))
	for _, label := range sortedLabels {
		writeString(h, label.Name)
		writeString(h, label.Value)
	}

	writeUint64(h, uint64(timeNanos))
	writeUint64(h, position)

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

func writeString(h hash.Hash, s string) {
	writeUint64(h, uint64(len(s)))
	_, _ = h.Write([]byte(s))
}

func writeUint64(h hash.Hash, n uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], n)
	_, _ = h.Write(buf[:])
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
