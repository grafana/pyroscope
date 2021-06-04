package convert

import (
	"fmt"
	"io"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

func Cli(cfg *config.Convert, logger func(string), args []string) error {
	var input io.Reader
	var err error

	if len(args) == 0 {
		input = os.Stdin
	} else {
		input, err = os.Open(args[0])
		if err != nil {
			return err
		}
	}

	parser := ParseGroups
	switch cfg.Format {
	case "tree":
		t := tree.New()
		err = parser(input, func(name []byte, val int) {
			t.Insert(name, uint64(val))
		})
		if err != nil {
			return err
		}

		err = t.SerializeNoDict(4096, os.Stdout)
		if err != nil {
			return err
		}
	case "trie":
		t := transporttrie.New()
		err = parser(input, func(name []byte, val int) {
			t.Insert(name, uint64(val), true)
		})
		if err != nil {
			return err
		}

		err = t.Serialize(os.Stdout)
		if err != nil {
			return err
		}
	default:
		logger(fmt.Sprintf("unknown format: %s", cfg.Format))
	}
	return nil
}
