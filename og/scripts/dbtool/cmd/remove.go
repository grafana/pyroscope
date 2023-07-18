package cmd

import (
	"github.com/dgraph-io/badger/v2"
	"github.com/spf13/cobra"
)

func (d *dbTool) newRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <key>",
		Short:   "Removes key value from the database",
		RunE:    d.runRemove,
		Args:    cobra.MinimumNArgs(1),
		PreRunE: d.openDB(false),
	}
}

func (d *dbTool) runRemove(_ *cobra.Command, args []string) error {
	return d.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(args[0]))
	})
}
