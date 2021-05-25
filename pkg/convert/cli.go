package convert

import (
	"fmt"
	"io"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

func Cli(cfg *config.Config, logger func(string), args []string) error {
	var input io.Reader
	if len(args) == 0 {
		input = os.Stdin
	} else {
		logger("not implemented yet")
	}

	parser := ParseGroups
	switch cfg.Convert.Format {
	case "tree":
		t := tree.New()
		parser(input, func(name []byte, val int) {
			t.Insert(name, uint64(val))
		})

		t.SerializeNoDict(4096, os.Stdout)
	case "trie":
		t := transporttrie.New()
		parser(input, func(name []byte, val int) {
			t.Insert(name, uint64(val), true)
		})

		t.Serialize(os.Stdout)
	default:
		logger(fmt.Sprintf("unknown format: %s", cfg.Convert.Format))
	}
	return nil
}
