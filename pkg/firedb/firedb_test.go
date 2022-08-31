package firedb

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
)

func TestCreateLocalDir(t *testing.T) {
	dataPath := t.TempDir()
	localFile := dataPath + "/local"
	require.NoError(t, ioutil.WriteFile(localFile, []byte("d"), 0o644))
	_, err := New(&Config{
		DataPath: dataPath,
	}, log.NewNopLogger(), nil)
	require.Error(t, err)
	require.NoError(t, os.Remove(localFile))
	_, err = New(&Config{
		DataPath: dataPath,
	}, log.NewNopLogger(), nil)
	require.NoError(t, err)
}
