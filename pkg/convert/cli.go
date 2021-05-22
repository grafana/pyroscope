package convert

import (
	"io"
	"log"
	"os"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/sirupsen/logrus"
)

func Cli(cfg *config.Convert, args []string) error {
	logrus.SetOutput(os.Stderr)
	var input io.Reader
	if len(args) == 0 {
		input = os.Stdin
	} else {
		log.Fatal("not implemented yet")
	}

	parser := ParseGroups
	switch cfg.Format {
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
		log.Fatal("unknown format: ", cfg.Format)
	}
	return nil
}
