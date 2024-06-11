package connectapi

import (
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
)

// Name is the name registered for the proto compressor.
const Name = "proto"

var ProtoCodec connect.Codec = vtprotoCodec{}

type vtprotoCodec struct{}

// growcap scales up the capacity of a slice.
//
// Given a slice with a current capacity of oldcap and a desired
// capacity of wantcap, growcap returns a new capacity >= wantcap.
//
// The algorithm is mostly identical to the one used by append as of Go 1.14.
func growcap(oldcap, wantcap int) (newcap int) {
	if wantcap > oldcap*2 {
		newcap = wantcap
	} else if oldcap < 1024 {
		// The Go 1.14 runtime takes this case when len(s) < 1024,
		// not when cap(s) < 1024. The difference doesn't seem
		// significant here.
		newcap = oldcap * 2
	} else {
		newcap = oldcap
		for 0 < newcap && newcap < wantcap {
			newcap += newcap / 4
		}
		if newcap <= 0 {
			newcap = wantcap
		}
	}
	return newcap
}

type vtprotoMessage interface {
	MarshalVT() ([]byte, error)
	MarshalToSizedBufferVT([]byte) (int, error)
	SizeVT() (n int)
	UnmarshalVT([]byte) error
}

func (vtprotoCodec) Marshal(v any) ([]byte, error) {
	switch v := v.(type) {
	case vtprotoMessage:
		return v.MarshalVT()
	case proto.Message:
		return proto.Marshal(v)
	default:
		return nil, fmt.Errorf("failed to marshal, message is %T, must satisfy the vtprotoMessage interface or want proto.Message", v)
	}
}

func (vtprotoCodec) MarshalAppend(data []byte, v any) ([]byte, error) {
	switch v := v.(type) {
	case vtprotoMessage:
		if v == nil {
			return data, nil
		}

		n := v.SizeVT()
		if cap(data) < len(data)+n {
			ndata := make([]byte, len(data), growcap(cap(data), len(data)+n))
			copy(ndata, data)
			data = ndata
		}
		_, err := v.MarshalToSizedBufferVT(data[len(data) : len(data)+n])
		if err != nil {
			return nil, err
		}
		return data[:len(data)+n], nil
	case proto.Message:
		return proto.MarshalOptions{}.MarshalAppend(data, v)
	default:
		return nil, fmt.Errorf("failed to marshalAppend, message is %T, must satisfy the vtprotoMessage interface or want proto.Message", v)
	}
}

func (vtprotoCodec) Unmarshal(data []byte, v any) error {
	switch v := v.(type) {
	case vtprotoMessage:
		return v.UnmarshalVT(data)
	case proto.Message:
		return proto.Unmarshal(data, v)
	default:
		return fmt.Errorf("failed to unmarshal, message is %T, must satisfy the vtprotoMessage interface or want proto.Message", v)
	}
}

func (vtprotoCodec) Name() string {
	return Name
}
