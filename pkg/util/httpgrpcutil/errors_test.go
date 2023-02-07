// SPDX-License-Identifier: AGPL-3.0-only

package httpgrpcutil

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/pkg/util/httpgrpc"
)

func TestPrioritizeRecoverableErr(t *testing.T) {
	type testCase struct {
		name          string
		errorsIn      []error
		expectedError error
	}

	recoverableGrpcError := httpgrpc.Errorf(http.StatusBadGateway, "recoverable error")
	nonRecoverableGrpcError := httpgrpc.Errorf(http.StatusBadRequest, "non-recoverable error")
	tooManyRequestsGrpcError := httpgrpc.Errorf(http.StatusTooManyRequests, "too many requests error")
	nonGrpcError := errors.New("non-grpc error")

	testCases := []testCase{
		{
			name:          "recoverable grpc error and non-recoverable grpc error",
			errorsIn:      []error{recoverableGrpcError, nonRecoverableGrpcError},
			expectedError: recoverableGrpcError,
		}, {
			name:          "recoverable grpc error and non-recoverable grpc error, reverse order",
			errorsIn:      []error{nonRecoverableGrpcError, recoverableGrpcError},
			expectedError: recoverableGrpcError,
		}, {
			name:          "non-recoverable grpc error and non-grpc error",
			errorsIn:      []error{nonRecoverableGrpcError, nonGrpcError},
			expectedError: nonGrpcError,
		}, {
			name:          "recoverable grpc error, non-recoverable grpc error and non-grpc error",
			errorsIn:      []error{recoverableGrpcError, nonRecoverableGrpcError, nonGrpcError},
			expectedError: recoverableGrpcError,
		}, {
			name:          "non-recoverable grpc error and too many requests error",
			errorsIn:      []error{nonRecoverableGrpcError, tooManyRequestsGrpcError},
			expectedError: tooManyRequestsGrpcError,
		}, {
			name:          "non-recoverable grpc error",
			errorsIn:      []error{nonRecoverableGrpcError},
			expectedError: nonRecoverableGrpcError,
		}, {
			name:          "no error",
			errorsIn:      []error{},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotErr := PrioritizeRecoverableErr(tc.errorsIn...)
			require.Equal(t, tc.expectedError, gotErr)
		})
	}
}
