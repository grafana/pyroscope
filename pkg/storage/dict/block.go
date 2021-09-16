package dict

type block struct{ data []byte }

const (
	lengthMask = 1<<32 - 1
	offsetMask = lengthMask << 32
)

func (c *block) insert(v []byte) uint64 {
	offset := len(c.data) << 32
	c.data = append(c.data, v...)
	return uint64(offset | (len(c.data) & lengthMask))
}

func (c *block) load(k uint64) []byte {
	return c.data[((k & offsetMask) >> 32):(k & lengthMask)]
}
