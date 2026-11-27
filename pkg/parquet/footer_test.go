package parquet

import (
	"bytes"
	"encoding/binary"
	"testing"

	parquetgo "github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"
)

func TestFooterSize(t *testing.T) {
	for _, rows := range []int{0, 100} {
		var data bytes.Buffer
		w := parquetgo.NewGenericWriter[struct{ Value int64 }](&data)
		for i := 0; i < rows; i++ {
			_, err := w.Write([]struct{ Value int64 }{{int64(i)}})
			require.NoError(t, err)
			if i%10 == 0 {
				require.NoError(t, w.Flush())
			}
		}
		require.NoError(t, w.Close())
		size, err := FooterSize(w.File())
		require.NoError(t, err)
		f, err := parquetgo.OpenFile(bytes.NewReader(data.Bytes()), int64(data.Len()), parquetgo.SkipPageIndex(true))
		require.NoError(t, err)
		readSize, err := FooterSize(f)
		require.NoError(t, err)
		require.Equal(t, size, readSize)
		metadataSize := uint64(binary.LittleEndian.Uint32(data.Bytes()[data.Len()-8:])) + 8
		if rows == 0 {
			require.Equal(t, metadataSize, size)
		} else {
			require.Greater(t, size, metadataSize)
		}
	}
}
