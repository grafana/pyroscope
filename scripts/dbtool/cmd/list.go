package cmd

import (
	"fmt"
	"os"

	"github.com/dgraph-io/badger/v2"
	"github.com/spf13/cobra"
)

func (d *dbTool) newListCommand() *cobra.Command {
	cmd := cobra.Command{
		Use:     "list",
		Short:   "Lists db keys",
		RunE:    d.runList,
		PreRunE: d.openDB(true),
	}
	cmd.Flags().StringVarP(&d.prefix, "prefix", "p", "", "key prefix")
	return &cmd
}

func (d *dbTool) runList(_ *cobra.Command, _ []string) error {
	return d.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = []byte(d.prefix)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			_, _ = fmt.Fprintf(os.Stdout, "%s\n", it.Item().Key())
		}
		return nil
	})
}
