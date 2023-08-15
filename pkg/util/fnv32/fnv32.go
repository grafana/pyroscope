package fnv32

// Inline and byte-free variant of hash/fnv's fnv64a.
const (
	offset32 = 2166136261
	prime32  = 16777619
)

// New initializies a new fnv32 hash value.
func New() uint32 {
	return offset32
}

// AddByte32 adds a byte to a fnv32 hash value, returning the updated hash.
func AddByte32(h uint32, b byte) uint32 {
	h *= prime32
	h ^= uint32(b)
	return h
}
