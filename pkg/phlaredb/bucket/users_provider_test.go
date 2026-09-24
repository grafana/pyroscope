package bucket

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/s3"
)

func TestCaptureNamespaceS3ProbeDrainsProducer(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	defer goleak.VerifyNone(t, baseline)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, `<ListBucketResult><Name>captures</Name><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken>`)
		for i := range 1000 {
			_, _ = fmt.Fprintf(w, "<Contents><Key>profile-debug-dumps/phlaredb/block-%04d</Key><Size>1</Size></Contents>", i)
		}
		_, _ = fmt.Fprint(w, `</ListBucketResult>`)
	}))
	defer srv.Close()
	var cfg s3.Config
	cfg.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	cfg.Endpoint = strings.TrimPrefix(srv.URL, "http://")
	cfg.BucketName, cfg.Region, cfg.Insecure = "captures", "us-east-1", true
	cfg.AccessKeyID = "local-test-key"
	require.NoError(t, cfg.SecretAccessKey.Set("local-test-secret"))
	cfg.BucketLookupType = s3.PathStyleLookup
	cfg.HTTP.Transport = srv.Client().Transport
	b, err := s3.NewBucketClient(cfg, "test", log.NewNopLogger())
	require.NoError(t, err)
	defer func() { require.NoError(t, b.Close()) }()
	found, err := hasProfileDumpV1Tenant(context.Background(), b, "profile-debug-dumps")
	require.NoError(t, err)
	require.True(t, found)
}
