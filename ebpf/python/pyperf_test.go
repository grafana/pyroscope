package python

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestReadEvent(t *testing.T) {
	require.Equal(t, 328, int(unsafe.Sizeof(PerfPyEvent{})))
	raw := make([]byte, 328)
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
	binary.LittleEndian.PutUint64(raw[320:], 0xdeadbeefcafebabe) //	Value       uint64
	event := new(PerfPyEvent)
	err := ReadPyEvent(raw, event)
	require.NoError(t, err)
	require.Equal(t, event.Hdr.StackStatus, uint8(0x1f))
	require.Equal(t, event.Hdr.Err, uint8(42))
	require.Equal(t, event.Hdr.Flags, uint8(0xef))
	require.Equal(t, event.Hdr.Reserved3, uint8(0xfe))
	require.Equal(t, event.Hdr.Pid, uint32(0xcafebabe))
	require.Equal(t, event.Hdr.KernStack, int64(0x7acecefadeadbeef))
	require.Equal(t, event.StackLen, uint32(0x61616161))
	require.Equal(t, event.Value, uint64(0xdeadbeefcafebabe))
	for i := 0; i < 75; i++ {
		require.Equal(t, event.Stack[i], 0xcafe000+uint32(i))
	}

}
