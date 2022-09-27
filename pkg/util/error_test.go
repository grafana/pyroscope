package util

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/weaveworks/common/httpgrpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/fire/pkg/tenant"
)

func Test_writeError(t *testing.T) {
	for _, tt := range []struct {
		name string

		err            error
		msg            string
		expectedStatus int
	}{
		{"cancelled", context.Canceled, ErrClientCanceled, StatusClientClosedRequest},
		{"rpc cancelled", status.New(codes.Canceled, context.Canceled.Error()).Err(), "rpc error: code = Canceled desc = context canceled", http.StatusInternalServerError},
		{"orgid", tenant.ErrNoTenantID, tenant.ErrNoTenantID.Error(), http.StatusBadRequest},
		{"deadline", context.DeadlineExceeded, ErrDeadlineExceeded, http.StatusGatewayTimeout},
		{"rpc deadline", status.New(codes.DeadlineExceeded, context.DeadlineExceeded.Error()).Err(), ErrDeadlineExceeded, http.StatusGatewayTimeout},
		// {"mixed context, rpc deadline and another", multierror.MultiError{errors.New("standard error"), context.DeadlineExceeded, status.New(codes.DeadlineExceeded, context.DeadlineExceeded.Error()).Err()}, "3 errors: standard error; context deadline exceeded; rpc error: code = DeadlineExceeded desc = context deadline exceeded", http.StatusInternalServerError},
		{"httpgrpc", httpgrpc.Errorf(http.StatusBadRequest, errors.New("foo").Error()), "foo", http.StatusBadRequest},
		{"internal", errors.New("foo"), "foo", http.StatusInternalServerError},
		// {"multi mixed", multierror.MultiError{context.Canceled, context.DeadlineExceeded}, "2 errors: context canceled; context deadline exceeded", http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(tt.err, rec)
			require.Equal(t, tt.expectedStatus, rec.Result().StatusCode)
			b, err := ioutil.ReadAll(rec.Result().Body)
			if err != nil {
				t.Fatal(err)
			}
			require.Equal(t, tt.msg, string(b[:len(b)-1]))
		})
	}
}
