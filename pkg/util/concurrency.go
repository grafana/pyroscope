package util

import (
	"runtime"
	"strconv"

	_ "go.uber.org/automaxprocs"
)

type ConcurrencyLimit int

func GoMaxProcsConcurrencyLimit() *ConcurrencyLimit {
	lim := ConcurrencyLimit(runtime.GOMAXPROCS(-1))
	return &lim
}

func (c *ConcurrencyLimit) String() string {
	if *c == 0 {
		return "auto"
	}
	return strconv.Itoa(int(*c))
}

func (c *ConcurrencyLimit) Set(v string) (err error) {
	var p int
	if v == "" || v == "auto" {
		p = runtime.GOMAXPROCS(-1)
	} else {
		p, err = strconv.Atoi(v)
	}
	if err != nil {
		return err
	}
	if p < 1 {
		*c = 1
		return
	}
	*c = ConcurrencyLimit(p)
	return nil
}

func (c *ConcurrencyLimit) UnmarshalText(text []byte) error {
	return c.Set(string(text))
}

func (c *ConcurrencyLimit) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}
