package querybackend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestValidateExemplarType(t *testing.T) {
	tests := []struct {
		name             string
		exemplarType     typesv1.ExemplarType
		expectedInclude  bool
		expectedErrorMsg string
		expectedCode     codes.Code
	}{
		{
			name:            "UNSPECIFIED returns false, no error",
			exemplarType:    typesv1.ExemplarType_EXEMPLAR_TYPE_UNSPECIFIED,
			expectedInclude: false,
		},
		{
			name:            "NONE returns false, no error",
			exemplarType:    typesv1.ExemplarType_EXEMPLAR_TYPE_NONE,
			expectedInclude: false,
		},
		{
			name:            "INDIVIDUAL returns true, no error",
			exemplarType:    typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL,
			expectedInclude: true,
		},
		{
			name:             "SPAN returns error with Unimplemented code",
			exemplarType:     typesv1.ExemplarType_EXEMPLAR_TYPE_SPAN,
			expectedInclude:  false,
			expectedErrorMsg: "exemplar type span is not implemented",
			expectedCode:     codes.Unimplemented,
		},
		{
			name:             "Unknown type returns error with InvalidArgument code",
			exemplarType:     typesv1.ExemplarType(999),
			expectedInclude:  false,
			expectedErrorMsg: "unknown exemplar type",
			expectedCode:     codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			include, err := validateExemplarType(tt.exemplarType)
			if tt.expectedErrorMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tt.expectedCode, st.Code())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedInclude, include)
			}
		})
	}
}
