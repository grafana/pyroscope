package cmd

import (
	"bytes"
	"io"
	"os"

	"github.com/dgraph-io/badger/v2"
	"github.com/spf13/cobra"
)

func (d *dbTool) newGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "get <key>",
		Short:   "Retrieves key value and dumps it to stdout",
		RunE:    d.runGet,
		Args:    cobra.MinimumNArgs(1),
		PreRunE: d.openDB(true),
	}
}

func (d *dbTool) runGet(_ *cobra.Command, args []string) error {
	return d.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(args[0]))
		if err != nil {
			return err
		}
		v, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		_, _ = io.Copy(os.Stdout, bytes.NewBuffer(v))
		return nil
	})
}
