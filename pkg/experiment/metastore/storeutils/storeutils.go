package storeutils

import (
	"encoding/binary"

	"go.etcd.io/bbolt"
)

func ParseTenantShardBucketName(b []byte) (shard uint32, tenant string, ok bool) {
	if len(b) >= 4 {
		return binary.BigEndian.Uint32(b), string(b[4:]), true
	}
	return 0, "", false
}

func GetOrCreateSubBucket(parent *bbolt.Bucket, name []byte) (*bbolt.Bucket, error) {
	bucket := parent.Bucket(name)
	if bucket == nil {
		return parent.CreateBucket(name)
	}
	return bucket, nil
}
