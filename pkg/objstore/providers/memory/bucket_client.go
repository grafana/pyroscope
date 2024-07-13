package memory

import (
	"github.com/thanos-io/objstore"
)

func NewBucketClient() objstore.Bucket {
	return objstore.NewInMemBucket()
}
