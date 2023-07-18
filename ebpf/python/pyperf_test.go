package python

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/grafana/pyroscope/ebpf/pyrobpf"
	"github.com/stretchr/testify/require"
)

func TestReadEvent(t *testing.T) {
	require.Equal(t, 320, int(unsafe.Sizeof(pyrobpf.ProfilePyEvent{})))
	raw := make([]byte, 320)
	raw[0] = 0x1f                                              //	StackStatus uint8
	raw[1] = 42                                                //	Err         uint8
	raw[2] = 0xef                                              //	Reserved2   uint8
	raw[3] = 0xfe                                              //	Reserved3   uint8
	binary.LittleEndian.PutUint32(raw[4:], 0xcafebabe)         //	Pid         uint32
	binary.LittleEndian.PutUint64(raw[8:], 0x7acecefadeadbeef) //	KernStack   int64
	binary.LittleEndian.PutUint32(raw[16:], 0x61616161)        //	StackLen    uint32
	for i := 0; i < 75; i++ {
		binary.LittleEndian.PutUint32(raw[20+i*4:], 0xcafe000+uint32(i)) //	Stack       [75]uint32
	}
	event, err := ReadPyEvent(raw)
	require.NoError(t, err)
	require.Equal(t, event.StackStatus, uint8(0x1f))
	require.Equal(t, event.Err, uint8(42))
	require.Equal(t, event.Reserved2, uint8(0xef))
	require.Equal(t, event.Reserved3, uint8(0xfe))
	require.Equal(t, event.Pid, uint32(0xcafebabe))
	require.Equal(t, event.KernStack, int64(0x7acecefadeadbeef))
	require.Equal(t, event.StackLen, uint32(0x61616161))
	for i := 0; i < 75; i++ {
		require.Equal(t, event.Stack[i], 0xcafe000+uint32(i))
	}

}

func TestReadEventError(t *testing.T) {

	raw := make([]byte, 16)
	raw[0] = 1                                                 //	StackStatus uint8
	raw[1] = 42                                                //	Err         uint8
	raw[2] = 0xef                                              //	Reserved2   uint8
	raw[3] = 0xfe                                              //	Reserved3   uint8
	binary.LittleEndian.PutUint32(raw[4:], 0xcafebabe)         //	Pid         uint32
	binary.LittleEndian.PutUint64(raw[8:], 0x7acecefadeadbeef) //	KernStack   int64

	event, err := ReadPyEvent(raw)
	require.NoError(t, err)
	require.Equal(t, event.StackStatus, uint8(0x1))
	require.Equal(t, event.Err, uint8(42))
	require.Equal(t, event.Reserved2, uint8(0xef))
	require.Equal(t, event.Reserved3, uint8(0xfe))
	require.Equal(t, event.Pid, uint32(0xcafebabe))
	require.Equal(t, event.KernStack, int64(0x7acecefadeadbeef))
	require.Equal(t, event.StackLen, int64(0))
	for i := 0; i < 75; i++ {
		require.Equal(t, event.Stack[i], uint32(0))
	}

}
