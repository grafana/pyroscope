package storage

import (
	"encoding/binary"
	"errors"
	"regexp"
	"strings"

	"github.com/petethepig/pyroscope/pkg/structs/sortedmap"
	"github.com/spaolacci/murmur3"
)

type Key struct {
	labels map[string]string
}

var nameParser *regexp.Regexp

const seed = 6231912

func init() {
	nameParser = regexp.MustCompile("^(.+)\\{(.+)\\}.*$")
}

// TODO: should rewrite this at some point to not rely on regular expressions & splits
func ParseKey(name string) (*Key, error) {
	res := nameParser.FindStringSubmatch(name)
	if len(res) != 3 {
		return nil, errors.New("invalid key")
	}
	labels := make(map[string]string)
	labels["__name__"] = strings.TrimSpace(res[1])
	for _, v := range strings.Split(res[2], ",") {
		arr := strings.Split(v, "=")
		if len(arr) != 2 {
			return nil, errors.New("invalid key")
		}

		labels[strings.TrimSpace(arr[0])] = strings.TrimSpace(arr[1])
	}
	return &Key{
		labels: labels,
	}, nil
}

func (k *Key) Normalized() string {
	var sb strings.Builder

	sortedMap := sortedmap.New()
	for k, v := range k.labels {
		if k == "__name__" {
			sb.WriteString(v)
		} else {
			sortedMap.Put(k, v)
		}
	}

	sb.WriteString("{")
	for i, k := range sortedMap.Keys() {
		v := sortedMap.Get(k).(string)
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
	}
	sb.WriteString("}")

	return sb.String()
}

func (k *Key) Hashed() []byte {
	u1, u2 := murmur3.Sum128WithSeed([]byte(k.Normalized()), seed)

	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[:8], u1)
	binary.LittleEndian.PutUint64(b[8:16], u2)
	return b
}
