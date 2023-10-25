package pprof

import (
	"testing"

	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/stretchr/testify/require"
)

func TestEmptyPPROF(t *testing.T) {

	p := RawProfile{
		FormDataContentType: "multipart/form-data; boundary=ae798a53dec9077a712cf18e2ebf54842f5c792cfed6a31b6f469cfd2684",
		RawData: []byte("--ae798a53dec9077a712cf18e2ebf54842f5c792cfed6a31b6f469cfd2684\r\n" +
			"Content-Disposition: form-data; name=\"profile\"; filename=\"profile.pprof\"\r\n" +
			"Content-Type: application/octet-stream\r\n" +
			"\r\n" +
			"\r\n" +
			"--ae798a53dec9077a712cf18e2ebf54842f5c792cfed6a31b6f469cfd2684--\r\n"),
	}
	profile, err := p.ParseToPprof(nil, ingestion.Metadata{})
	require.NoError(t, err)
	require.Equal(t, 0, len(profile.Series))
}
