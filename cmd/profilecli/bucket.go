package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/grafana/dskit/server"
	"github.com/oklog/ulid/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/thanos-io/objstore"

	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/operations"
)

type bucketParams struct {
	objectStoreCfg objstoreclient.Config
}

func (b *bucketParams) initClient(ctx context.Context) (phlareobj.Bucket, error) {
	return objstoreclient.NewBucket(
		ctx,
		b.objectStoreCfg,
		"storage",
	)
}

type bucketWebToolParams struct {
	*bucketParams
	httpListenPort int
}

func addBucketParams(cmd commander) *bucketParams {
	var (
		params = &bucketParams{}
	)
	// keep in sync with objstoreclient.RegisterFlags
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	params.objectStoreCfg.RegisterFlagsWithPrefix("storage.", fs)
	fs.VisitAll(func(f *flag.Flag) {
		switch f.Name {
		case "storage.backend":
			cmd.Flag(f.Name, f.Usage).Default("filesystem").StringVar(&params.objectStoreCfg.Backend)
		case "storage.prefix":
			cmd.Flag(f.Name, f.Usage).Default(f.DefValue).StringVar(&params.objectStoreCfg.Prefix)
		case "storage.filesystem.dir":
			cmd.Flag(f.Name, f.Usage).Default(f.DefValue).StringVar(&params.objectStoreCfg.Filesystem.Directory)
		case "storage.gcs.bucket-name":
			cmd.Flag(f.Name, f.Usage).Default(f.DefValue).StringVar(&params.objectStoreCfg.GCS.BucketName)
		case "storage.s3.bucket-name":
			cmd.Flag(f.Name, f.Usage).Default(f.DefValue).StringVar(&params.objectStoreCfg.S3.BucketName)
		case "storage.s3.endpoint":
			cmd.Flag(f.Name, f.Usage).Default(f.DefValue).StringVar(&params.objectStoreCfg.S3.Endpoint)
		case "storage.s3.region":
			cmd.Flag(f.Name, f.Usage).Default(f.DefValue).StringVar(&params.objectStoreCfg.S3.Region)
		case "storage.azure.container-name":
			cmd.Flag(f.Name, f.Usage).Default(f.DefValue).StringVar(&params.objectStoreCfg.Azure.ContainerName)
		case "storage.azure.account-name":
			cmd.Flag(f.Name, f.Usage).Default(f.DefValue).StringVar(&params.objectStoreCfg.Azure.StorageAccountName)
		default:
			break
		}
	})
	return params
}

//go:embed static
var staticFiles embed.FS

func addBucketWebToolParams(cmd commander) *bucketWebToolParams {
	var (
		params = &bucketWebToolParams{}
	)
	params.bucketParams = addBucketParams(cmd)
	cmd.Flag("http-listen-port", "The port to run the HTTP server on.").Default("4201").IntVar(&params.httpListenPort)
	return params
}

type bucketWebTool struct {
	params *bucketWebToolParams
}

func newBucketWebTool(params *bucketWebToolParams) *bucketWebTool {
	return &bucketWebTool{
		params: params,
	}
}

func (t *bucketWebTool) run(ctx context.Context) error {
	s, err := server.New(server.Config{
		HTTPListenPort: t.params.httpListenPort,
		Log:            logger,
	})
	if err != nil {
		return err
	}

	b, err := t.params.initClient(ctx)
	if err != nil {
		log.Fatal(err)
		return err
	}

	handlers := operations.Handlers{
		Logger: logger,
		Bucket: b,
	}

	s.HTTP.PathPrefix("/static").Handler(http.FileServer(http.FS(staticFiles)))
	s.HTTP.Path("/ops/object-store/tenants").HandlerFunc(handlers.CreateIndexHandler())
	s.HTTP.Path("/ops/object-store/tenants/{tenant}/blocks").HandlerFunc(handlers.CreateBlocksHandler())
	s.HTTP.Path("/ops/object-store/tenants/{tenant}/blocks/{block}").HandlerFunc(handlers.CreateBlockDetailsHandler())

	out := output(ctx)

	fmt.Fprintf(out, "The bucket web tool is available at http://localhost:%d/ops/object-store/tenants\n", t.params.httpListenPort)

	if err := s.Run(); err != nil {
		return err
	}

	return nil
}

func labelStrings(v []int32, s []string, sb *strings.Builder) {
	pairs := metadata.LabelPairs(v)
	for pairs.Next() {
		p := pairs.At()
		first := true
		for len(p) > 0 {
			if first {
				first = false
				_, _ = sb.WriteString("- ")
			} else {
				_, _ = sb.WriteString("  ")
			}
			_, _ = sb.WriteString(s[p[0]])
			_, _ = sb.WriteString(`="`)
			_, _ = sb.WriteString(s[p[1]])
			_, _ = sb.WriteString(`"`)
			_, _ = sb.WriteString("\n")
			p = p[2:]
		}
	}
}

func bucketInspectV2(ctx context.Context, params *bucketParams, paths []string) error {
	b, err := params.initClient(ctx)
	if err != nil {
		return err
	}

	lst := bucketList{}

	for _, path := range paths {
		obj, err := block.NewObjectFromPath(ctx, b, path)
		if err != nil {
			return err
		}

		e, err := bucketListElemFromPath(path)
		if err != nil {
			return err
		}

		meta := obj.Metadata()
		stringTable := meta.GetStringTable()
		e.Size = meta.GetSize()
		e.CompactionLevel = meta.GetCompactionLevel()

		lst.lst = lst.lst[:0]
		lst.lst = append(lst.lst, *e)
		lst.renderInspectTable(ctx)

		sb := new(strings.Builder)
		tbl := tablewriter.NewWriter(output(ctx))
		tbl.SetAutoWrapText(false)
		tbl.SetHeader([]string{"Tenant", "Dataset Name", "From", "Duration", "Size", "Labels"})

		for _, ds := range meta.Datasets {
			sb.Reset()
			labelStrings(ds.Labels, stringTable, sb)
			tbl.Append([]string{
				stringTable[ds.Tenant],
				stringTable[ds.Name],
				model.Time(ds.MinTime).Time().Format(time.RFC3339),
				(time.Duration(ds.MaxTime-ds.MinTime) * time.Millisecond).String(),
				humanize.Bytes(ds.Size),
				strings.TrimRight(sb.String(), "\n"),
			})
		}
		tbl.Render()

	}

	return nil
}

type bucketList struct {
	lst []bucketListElem
	tbl *tablewriter.Table
}

func (b bucketList) renderInspectTable(ctx context.Context) {
	if b.tbl == nil {
		b.tbl = tablewriter.NewWriter(output(ctx))
	} else {
		b.tbl.ClearRows()
	}
	b.tbl.SetHeader([]string{"Block ID", "Time", "Type", "Shard", "Tenant", "Compaction Level", "Size", "Path"})
	for _, e := range b.lst {
		b.tbl.Append([]string{
			e.ID.String(),
			e.ID.Timestamp().Format(time.RFC3339),
			e.Type,
			strconv.Itoa(e.Shard),
			e.Tenant,
			strconv.Itoa(int(e.CompactionLevel)),
			humanize.Bytes(e.Size),
			e.Path,
		})

	}
	b.tbl.Render()
}

func (b bucketList) renderListTable(ctx context.Context) {
	if b.tbl == nil {
		b.tbl = tablewriter.NewWriter(output(ctx))
	} else {
		b.tbl.ClearRows()
	}
	b.tbl.SetHeader([]string{"Block ID", "Time", "Type", "Shard", "Tenant", "Path"})
	for _, e := range b.lst {
		b.tbl.Append([]string{
			e.ID.String(),
			e.ID.Timestamp().Format(time.RFC3339),
			e.Type,
			strconv.Itoa(e.Shard),
			e.Tenant,
			e.Path,
		})

	}
	b.tbl.Render()
}

type bucketListElem struct {
	ID              ulid.ULID
	Type            string
	Shard           int
	Tenant          string
	Path            string
	CompactionLevel uint32
	Size            uint64
}

var errUnexpectedPath = errors.New("unexpected path")

func bucketListElemFromPath(s string) (*bucketListElem, error) {
	parts := strings.Split(s, "/")
	if parts[len(parts)-1] != "block.bin" {
		return nil, errUnexpectedPath
	}

	id, err := ulid.Parse(parts[len(parts)-2])
	if err != nil {
		return nil, err
	}
	shard, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, err
	}

	tenant := parts[2]
	if parts[0] == "segments" {
		tenant = "various"
	}

	return &bucketListElem{
		ID:     id,
		Type:   parts[0],
		Shard:  shard,
		Tenant: tenant,
		Path:   s,
	}, nil
}

func bucketListV2(ctx context.Context, params *bucketParams) error {
	b, err := params.initClient(ctx)
	if err != nil {
		return err
	}

	lst := bucketList{}
	limit := 40 // might be depending on the size of the terminal

	addBlock := func(name string) error {
		e, err := bucketListElemFromPath(name)
		if err != nil {
			if err == errUnexpectedPath {
				return nil
			}
			return err
		}

		lst.lst = append(lst.lst, *e)
		if len(lst.lst) > limit {
			lst.renderListTable(ctx)
			lst.lst = lst.lst[:0]
		}

		return nil
	}

	if err := b.Iter(ctx, "segments", addBlock, objstore.WithRecursiveIter()); err != nil {
		return err
	}
	if err := b.Iter(ctx, "blocks", addBlock, objstore.WithRecursiveIter()); err != nil {
		return err
	}
	lst.renderListTable(ctx)
	return nil
}
