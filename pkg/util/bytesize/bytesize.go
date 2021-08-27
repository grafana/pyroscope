package bytesize

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type ByteSize int64

var Byte ByteSize = 1

var (
	KB = 1024 * Byte
	MB = 1024 * KB
	GB = 1024 * MB
	TB = 1024 * GB
	PB = 1024 * TB
)

var (
	KiB = 1000 * Byte
	MiB = 1000 * KiB
	GiB = 1000 * MiB
	TiB = 1000 * GiB
	PiB = 1000 * TiB
)

var suffixes = []string{"KB", "MB", "GB", "TB", "PB"}

func (b ByteSize) String() string {
	if b < KB {
		return fmt.Sprintf("%d bytes", b)
	}
	bf := float64(b)
	for _, s := range suffixes {
		bf /= 1024.0
		if bf < 1024 {
			return fmt.Sprintf("%.2f %s", bf, s)
		}
	}
	return fmt.Sprintf("%.2f %s", bf, suffixes[len(suffixes)-1])
}

var multipliers = map[string]ByteSize{
	"":    Byte,
	"b":   Byte,
	"kb":  KB,
	"mb":  MB,
	"gb":  GB,
	"tb":  TB,
	"pb":  PB,
	"kib": KiB,
	"mib": MiB,
	"gib": GiB,
	"tib": TiB,
	"pib": PiB,
}

var byteSizeRegexp = regexp.MustCompile("^([\\d\\.]+)\\s*([^\\d]*)$")

var errParse = errors.New("could not parse ByteSize")

func Parse(str string) (ByteSize, error) {
	r := byteSizeRegexp.FindStringSubmatch(strings.TrimSpace(str))
	if len(r) != 3 {
		return 0, errParse
	}

	multiplier, ok := multipliers[strings.ToLower(r[2])]
	if !ok {
		return 0, errParse
	}

	if strings.Contains(r[1], ".") {
		val, err := strconv.ParseFloat(r[1], 64)
		if err != nil {
			return 0, errParse
		}
		return ByteSize(val * float64(multiplier)), nil
	}

	val, err := strconv.ParseUint(r[1], 10, 64)
	if err != nil {
		return 0, errParse
	}
	return ByteSize(val) * multiplier, nil
}

func (b *ByteSize) Set(value string) error {
	v, err := Parse(value)
	if err != nil {
		return err
	}
	*b = v
	return nil
}
