package fsm

import (
	"encoding/binary"
	"fmt"
	"reflect"

	"google.golang.org/protobuf/proto"
)

var ErrInvalidCommand = fmt.Errorf("invalid command format; expected at least 4 bytes")

type RaftLogEntryType uint32

type RaftLogEntry struct {
	Type RaftLogEntryType
	Data []byte
}

type Response struct {
	Data proto.Message
	Err  error
}

func MarshalEntry(t RaftLogEntryType, payload proto.Message) ([]byte, error) {
	b, err := marshal(payload)
	if err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint32(b, uint32(t))
	return b, nil
}

func (c *RaftLogEntry) UnmarshalBinary(b []byte) error {
	if len(b) < 4 {
		return ErrInvalidCommand
	}
	c.Type = RaftLogEntryType(binary.BigEndian.Uint32(b))
	c.Data = b[4:]
	return nil
}

type vtMarshaller interface {
	MarshalToSizedBufferVT([]byte) (int, error)
	SizeVT() int
}

// marshal MUST allocate a buffer of size covering
// the whole message, including the entry header.
func marshal(v proto.Message) ([]byte, error) {
	if m, ok := any(v).(vtMarshaller); ok {
		size := m.SizeVT()
		buf := make([]byte, size+4)
		_, err := m.MarshalToSizedBufferVT(buf[4:])
		return buf, err
	}
	raw, err := proto.Marshal(v)
	if err != nil {
		return raw, err
	}
	buf := make([]byte, 4+len(raw))
	copy(buf[4:], raw)
	return buf, err
}

type vtUnmarshaler interface {
	UnmarshalVT([]byte) error
}

func newProto[T proto.Message]() T {
	var msg T
	msgType := reflect.TypeOf(msg).Elem()
	return reflect.New(msgType).Interface().(T)
}

func unmarshal[T proto.Message](b []byte) (v T, err error) {
	v = newProto[T]()
	if vt, ok := any(v).(vtUnmarshaler); ok {
		err = vt.UnmarshalVT(b)
	} else {
		err = proto.Unmarshal(b, v)
	}
	return v, err
}
