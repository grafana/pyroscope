package main

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"log"
	"net/http"

	"github.com/grafana/dskit/server"

	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/gcs"
	"github.com/grafana/pyroscope/pkg/operations"
)

type bucketWebToolParams struct {
	httpListenPort  int
	objectStoreType string
	bucketName      string
}

//go:embed static
var staticFiles embed.FS

func addBucketWebToolParams(bucketWebToolCmd commander) *bucketWebToolParams {
	var (
		params = &bucketWebToolParams{}
	)
	bucketWebToolCmd.Flag("object-store-type", "The type of the object storage (e.g., gcs).").Default("gcs").StringVar(&params.objectStoreType)
	bucketWebToolCmd.Flag("bucket-name", "The name of the object storage bucket.").StringVar(&params.bucketName)
	bucketWebToolCmd.Flag("http-listen-port", "The port to run the HTTP server on.").Default("4201").IntVar(&params.httpListenPort)
	return params
}

type bucketWebTool struct {
	params *bucketWebToolParams
	server *server.Server
}

func newBucketWebTool(params *bucketWebToolParams) *bucketWebTool {
	var (
		serverCfg = server.Config{
			HTTPListenPort: params.httpListenPort,
			Log:            logger,
		}
	)

	s, err := server.New(serverCfg)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	b, err := initObjectStoreBucket(params)
	if err != nil {
		log.Fatal(err)
		return nil
	}

	tool := &bucketWebTool{
		params: params,
		server: s,
	}

	handlers := operations.Handlers{
		Logger: logger,
		Bucket: b,
	}

	s.HTTP.PathPrefix("/static").Handler(http.FileServer(http.FS(staticFiles)))
	s.HTTP.Path("/ops/object-store/tenants").HandlerFunc(handlers.CreateIndexHandler())
	s.HTTP.Path("/ops/object-store/tenants/{tenant}/blocks").HandlerFunc(handlers.CreateBlocksHandler())
	s.HTTP.Path("/ops/object-store/tenants/{tenant}/blocks/{block}").HandlerFunc(handlers.CreateBlockDetailsHandler())

	return tool
}

func initObjectStoreBucket(params *bucketWebToolParams) (phlareobj.Bucket, error) {
	objectStoreConfig := objstoreclient.Config{
		StoragePrefix: "",
		StorageBackendConfig: objstoreclient.StorageBackendConfig{
			Backend: params.objectStoreType,
			GCS: gcs.Config{
				BucketName: params.bucketName,
			},
		},
	}
	return objstoreclient.NewBucket(
		context.Background(),
		objectStoreConfig,
		"storage",
	)
}

func (t *bucketWebTool) run(ctx context.Context) error {
	out := output(ctx)

	fmt.Fprintf(out, "The bucket web tool is available at http://localhost:%d/ops/object-store/tenants\n", t.params.httpListenPort)

	if err := t.server.Run(); err != nil {
		return err
	}

	return nil
}
