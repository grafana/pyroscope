package fsm

import (
	"encoding/binary"
	"fmt"
)

type RaftLogEntryType uint32

type RaftLogEntry struct {
	Type RaftLogEntryType
	Data []byte
}

func (c *RaftLogEntry) MarshalBinary() ([]byte, error) {
	b := make([]byte, 4+len(c.Data))
	binary.LittleEndian.PutUint32(b, uint32(c.Type))
	copy(b[4:], c.Data)
	return b, nil
}

var ErrInvalidCommand = fmt.Errorf("invalid command format; expected at least 4 bytes")

func (c *RaftLogEntry) UnmarshalBinary(b []byte) error {
	if len(b) < 4 {
		return ErrInvalidCommand
	}
	c.Type = RaftLogEntryType(binary.LittleEndian.Uint32(b))
	c.Data = b[4:]
	return nil
}

type Response struct {
	Data []byte
	Err  error
}
