package metastore

import (
	"errors"
	"testing"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

var testError = errors.New("test error")
var typedNil *metastorev1.AddBlockResponse = nil

func Test_handleCommandErrorHandling(t *testing.T) {
	type args[Req proto.Message, Resp proto.Message] struct {
		raw  []byte
		cmd  *raft.Log
		call commandCall[Req, Resp]
	}
	type testCase[Req proto.Message, Resp proto.Message] struct {
		name      string
		args      args[Req, Resp]
		want      fsmResponse
		wantPanic bool
	}
	tests := []testCase[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse]{
		{
			name: "no error",
			args: args[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse]{
				raw: make([]byte, 0),
				cmd: &raft.Log{},
				call: func(log *raft.Log, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
					return &metastorev1.AddBlockResponse{}, nil
				},
			},
			want: fsmResponse{
				msg: &metastorev1.AddBlockResponse{},
				err: nil,
			},
		},
		{
			name: "a simple error is returned",
			args: args[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse]{
				raw: make([]byte, 0),
				cmd: &raft.Log{},
				call: func(log *raft.Log, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
					return nil, testError
				},
			},
			want: fsmResponse{
				msg: typedNil,
				err: testError,
			},
		},
		{
			name: "a panic with a fatal error results in a real panic",
			args: args[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse]{
				raw: make([]byte, 0),
				cmd: &raft.Log{},
				call: func(log *raft.Log, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
					panic(fatalCommandError{testError})
				},
			},
			wantPanic: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					assert.True(t, tt.wantPanic)
				}
			}()
			assert.Equal(t, tt.want, handleCommand(tt.args.raw, tt.args.cmd, tt.args.call))
		})
	}
}
