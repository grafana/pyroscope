package firedb

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/fire/pkg/gen/google/v1"
)

func TestCreate(t *testing.T) {
	fName := "testdata/heap"
	f, err := os.Open(fName)
	require.NoError(t, err, "failed opening profile: ", fName)
	r, err := gzip.NewReader(f)
	require.NoError(t, err)
	content, err := ioutil.ReadAll(r)
	require.NoError(t, err, "failed reading file: ", fName)

	sch := parquet.SchemaOf(&profilev1.Profile{})
	t.Logf("%v", sch.Columns())

	p := &profilev1.Profile{}
	require.NoError(t, p.UnmarshalVT(content))

	//require.Equal(t, sch, "")

	buffer := new(bytes.Buffer)
	pw := parquet.NewWriter(buffer, sch)

	//spew.Print(p.Sample[:5])

	p2 := profilev1.Profile{
		StringTable: p.StringTable,
		Sample:      p.Sample[0:1],
	}
	spew.Print(p2.Sample)

	require.NoError(t, pw.Write(p2))

	//t.Logf("%v", pw.Schema().Columns())

}
