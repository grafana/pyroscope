package s3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thanos-io/objstore/providers/s3"
)

func Test_getS3BucketLookupType(t *testing.T) {
	type args struct {
		lookupType string
	}
	tests := []struct {
		name string
		args args
		want s3.BucketLookupType
	}{
		{
			name: "default is auto",
			args: args{
				lookupType: "",
			},
			want: s3.BucketLookupType(0),
		},
		{
			name: "default is auto",
			args: args{
				lookupType: "",
			},
			want: s3.BucketLookupType(0),
		},
		{
			name: "virtual-hosted",
			args: args{
				lookupType: "virtual-hosted",
			},
			want: s3.BucketLookupType(1),
		},
		{
			name: "path",
			args: args{
				lookupType: "path",
			},
			want: s3.BucketLookupType(2),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, getS3BucketLookupType(tt.args.lookupType), "getS3BucketLookupType(%v)", tt.args.lookupType)
		})
	}
}
