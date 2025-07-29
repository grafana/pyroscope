package main

import (
	"container/list"
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/grafana/dskit/flagext"
	"golang.org/x/sync/errgroup"

	pyroscopecfg "github.com/grafana/pyroscope/pkg/cfg"
	pyroscopeobj "github.com/grafana/pyroscope/pkg/objstore"
	objstoreclient "github.com/grafana/pyroscope/pkg/objstore/client"
)

type v2MigrationBucketCleanupParams struct {
	configFile      string
	configExpandEnv bool
	dryRun          string
}

func (p *v2MigrationBucketCleanupParams) isDryRun() bool {
	return p.dryRun != "false"
}

type minimalConfig struct {
	Bucket objstoreclient.Config `yaml:"storage"`
}

// Note: These are not the flags used, but we need to register them to get the defaults.
func (c *minimalConfig) RegisterFlags(f *flag.FlagSet) {
	c.Bucket.RegisterFlags(f)
}

func (c *minimalConfig) ApplyDynamicConfig() pyroscopecfg.Source {
	return func(dst pyroscopecfg.Cloneable) error {
		return nil
	}
}

func (c *minimalConfig) Clone() flagext.Registerer {
	return func(c minimalConfig) *minimalConfig {
		return &c
	}(*c)
}

func clientFromParams(ctx context.Context, params *v2MigrationBucketCleanupParams) (pyroscopeobj.Bucket, error) {
	if params.configFile == "" {
		return nil, fmt.Errorf("config file is required")
	}
	cfg := &minimalConfig{}
	fs := flag.NewFlagSet("config-file-loader", flag.ContinueOnError)
	if err := pyroscopecfg.Unmarshal(cfg,
		pyroscopecfg.Defaults(fs),
		pyroscopecfg.YAMLIgnoreUnknownFields(params.configFile, params.configExpandEnv),
	); err != nil {
		return nil, fmt.Errorf("failed parsing config: %w", err)
	}

	return objstoreclient.NewBucket(ctx, cfg.Bucket, "profilecli")
}

func addV2MigrationBackupCleanupParam(c commander) *v2MigrationBucketCleanupParams {
	var (
		params = &v2MigrationBucketCleanupParams{}
	)
	c.Flag("config.file", "The path to the pyroscope config").Default("/etc/pyroscope/config.yaml").StringVar(&params.configFile)
	c.Flag("config.expand-env", "").Default("false").BoolVar(&params.configExpandEnv)
	c.Flag("dry-run", "Dry run the operation.").Default("true").StringVar(&params.dryRun)
	return params
}

func v2MigrationBucketCleanup(ctx context.Context, params *v2MigrationBucketCleanupParams) error {
	client, err := clientFromParams(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	var pathsToDelete []string
	// find prefix called "phlaredb/" on the second level
	if err := client.Iter(ctx, "", func(name string) error {
		if !strings.HasSuffix(name, "/") {
			return nil
		}
		err := client.Iter(ctx, name, func(name string) error {
			if strings.HasSuffix(name, "phlaredb/") {
				pathsToDelete = append(pathsToDelete, name)
			}
			return nil
		})
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to list tenants: %w", err)
	}

	if len(pathsToDelete) == 0 {
		fmt.Println("No paths to delete")
		return nil
	}

	if params.isDryRun() {
		fmt.Println("DRY-RUN: If ran with --dry-run=false, this will delete everything under:")
	} else {
		fmt.Println("This will delete everything under:")
	}
	for _, path := range pathsToDelete {
		fmt.Println(" - ", path)
	}

	if params.isDryRun() {
		fmt.Println("DRY-RUN: If ran with --dry-run=false, this will delete those object store keys:")
		return recurse(ctx, client, func(key string) error {
			fmt.Println(" - ", key)
			return nil
		}, pathsToDelete)
	}

	// We do actually delete here
	fmt.Println("Last chance to cancel, waiting 3 seconds...")
	<-time.After(3 * time.Second)

	fmt.Println("Deleted object store keys:")
	return recurse(ctx, client, func(key string) error {
		if err := client.Delete(ctx, key); err != nil {
			return fmt.Errorf("failed to delete %s: %w", key, err)
		}
		fmt.Println(" - ", key)
		return nil
	}, pathsToDelete)
}

const maxConcurrentActions = 16

func recurse(ctx context.Context, b pyroscopeobj.Bucket, action func(key string) error, paths []string) error {
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentActions)

	g.Go(func() error {
		iters := list.New()
		for _, path := range paths {
			iters.PushBack(path)
		}

		for iters.Len() > 0 {
			e := iters.Front()
			path := e.Value.(string)

			if err := b.Iter(gctx, path, func(path string) error {
				if strings.HasSuffix(path, "/") {
					iters.PushBack(path)
					return nil
				}

				g.Go(func() error {
					return action(path)
				})

				return nil
			}); err != nil {
				return fmt.Errorf("failed to iterate over %s: %w", path, err)
			}
			iters.Remove(e)
		}

		return nil
	})

	return g.Wait()
}
