package s3

import (
	"fmt"
	"testing"

	"github.com/minio/minio-go/v7"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestBucket_IsConditionNotMetErr(t *testing.T) {
	b := &bucket{}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "412 PreconditionFailed",
			err:  minio.ErrorResponse{Code: "PreconditionFailed"},
			want: true,
		},
		{
			name: "409 ConditionalRequestConflict",
			err:  minio.ErrorResponse{Code: "ConditionalRequestConflict", StatusCode: 409},
			want: true,
		},
		{
			name: "a wrapped 409 ConditionalRequestConflict",
			err:  fmt.Errorf("upload failed: %w", minio.ErrorResponse{Code: "ConditionalRequestConflict", StatusCode: 409}),
			want: true,
		},
		{
			// The provider wraps upload errors exactly like this: pkg/errors.Wrap
			// adds two chain layers (withStack, withMessage) above the minio error.
			name: "a provider-wrapped 409 with wrap layer",
			err:  pkgerrors.Wrap(minio.ErrorResponse{Code: "ConditionalRequestConflict", StatusCode: 409}, "upload s3 object"),
			want: true,
		},
		{
			name: "an unrelated error",
			err:  fmt.Errorf("some other failure"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, b.IsConditionNotMetErr(tt.err))
		})
	}
}
