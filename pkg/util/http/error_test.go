package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/gogo/status"
	"github.com/grafana/dskit/httpgrpc"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"

	"github.com/grafana/pyroscope/pkg/tenant"
)

func Test_writeError(t *testing.T) {
	for _, tt := range []struct {
		name string

		err            error
		msg            string
		expectedStatus int
	}{
		{"cancelled", context.Canceled, `{"code":"canceled","message":"The request was cancelled by the client."}`, StatusClientClosedRequest},
		{"rpc cancelled", status.New(codes.Canceled, context.Canceled.Error()).Err(), `{"code":"canceled","message":"The request was cancelled by the client."}`, StatusClientClosedRequest},
		{"orgid", tenant.ErrNoTenantID, `{"code":"invalid_argument","message":"no org id"}`, http.StatusBadRequest},
		{"deadline", context.DeadlineExceeded, `{"code":"deadline_exceeded","message":"Request timed out, decrease the duration of the request or add more label matchers (prefer exact match over regex match) to reduce the amount of data processed."}`, http.StatusGatewayTimeout},
		{"rpc deadline", status.New(codes.DeadlineExceeded, context.DeadlineExceeded.Error()).Err(), `{"code":"deadline_exceeded","message":"Request timed out, decrease the duration of the request or add more label matchers (prefer exact match over regex match) to reduce the amount of data processed."}`, http.StatusGatewayTimeout},
		// {"mixed context, rpc deadline and another", multierror.MultiError{errors.New("standard error"), context.DeadlineExceeded, status.New(codes.DeadlineExceeded, context.DeadlineExceeded.Error()).Err()}, "3 errors: standard error; context deadline exceeded; rpc error: code = DeadlineExceeded desc = context deadline exceeded", http.StatusInternalServerError},
		{"httpgrpc", httpgrpc.Errorf(http.StatusBadRequest, "foo"), `{"code":"invalid_argument","message":"foo"}`, http.StatusBadRequest},
		{"internal", errors.New("foo"), `{"code":"unknown","message":"foo"}`, http.StatusInternalServerError},
		{"connect", connect.NewError(connect.CodeInvalidArgument, errors.New("foo")), `{"code":"invalid_argument","message":"foo"}`, http.StatusBadRequest},
		{"connect wrapped", fmt.Errorf("foo %w", connect.NewError(connect.CodeInvalidArgument, errors.New("foo"))), `{"code":"invalid_argument","message":"foo"}`, http.StatusBadRequest},
		// {"multi mixed", multierror.MultiError{context.Canceled, context.DeadlineExceeded}, "2 errors: context canceled; context deadline exceeded", http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Error(rec, tt.err)
			assert.Equal(t, tt.expectedStatus, rec.Result().StatusCode)
			b, err := io.ReadAll(rec.Result().Body)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tt.msg, strings.TrimSpace(string(b)))
		})
	}
}
