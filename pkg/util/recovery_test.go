package util

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
)

func Test_onPanic(t *testing.T) {
	expectedError := fmt.Errorf("internal: test")

	t.Run("WrapUnary", func(t *testing.T) {
		resp, err := RecoveryInterceptor.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
			panic("test")
		})(context.Background(), nil)
		assert.Error(t, expectedError, err)
		assert.Nil(t, resp)
	})

	t.Run("WrapStreamingHandler", func(t *testing.T) {
		err := RecoveryInterceptor.WrapStreamingHandler(func(context.Context, connect.StreamingHandlerConn) error {
			panic("test")
		})(context.Background(), nil)
		assert.Error(t, expectedError, err)
	})
}
