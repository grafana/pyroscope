package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/spf13/cobra"
)

type dbTool struct {
	dir    string
	prefix string

	db *badger.DB
}

func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	d := new(dbTool)
	root := cobra.Command{
		Use:                "dbtool [command]",
		Short:              "DB helper tool",
		SilenceUsage:       true,
		SilenceErrors:      true,
		PersistentPostRunE: d.closeDB(),
	}

	root.PersistentFlags().StringVarP(&d.dir, "dir", "d", ".", "database directory path")
	root.AddCommand(
		d.newListCommand(),
		d.newRemoveCommand(),
		d.newGetCommand())

	return &root
}

func (d *dbTool) openDB(ro bool) func(cmd *cobra.Command, args []string) error {
	return func(c *cobra.Command, _ []string) error {
		db, err := openDB(d.dir, ro)
		if err != nil {
			return fmt.Errorf("failed to open %s: %v", d.dir, err)
		}
		d.db = db
		return nil
	}
}

func (d *dbTool) closeDB() func(cmd *cobra.Command, args []string) error {
	return func(c *cobra.Command, _ []string) error {
		if d.db == nil {
			return nil
		}
		if err := d.db.Close(); err != nil {
			return fmt.Errorf("closing database: %w", err)
		}
		return nil
	}
}

func openDB(dir string, ro bool) (*badger.DB, error) {
	if !ro {
		// When DB is opened for RW, badger.Open does not fail
		// if the directory is not a badger database, instead it
		// creates one.
		f, err := os.Open(filepath.Join(dir, badger.ManifestFilename))
		if err != nil {
			return nil, err
		}
		_ = f.Close()
	}
	return badger.Open(badger.DefaultOptions(dir).
		WithCompression(options.ZSTD).
		WithReadOnly(ro))
}
