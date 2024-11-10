package store

type BlockEntry struct {
	Index      uint64
	AppendedAt int64
	ID         string
	Tenant     string
	Shard      uint32
	Level      uint32
}
