package profile

import (
	"bufio"
	"io"

	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

const currentVersion = 2

func (p *Profile) Serialize(w io.Writer) error {
	vw := varint.NewWriter()
	var err error
	_, err = vw.Write(w, currentVersion)
	if err != nil {
		return err
	}
	_, err = vw.Write(w, uint64(len(p.Stacks)))
	if err != nil {
		return err
	}
	for _, s := range p.Stacks {
		_, err = vw.Write(w, s.ID)
		if err != nil {
			return err
		}
		_, err = vw.Write(w, s.Value)
		if err != nil {
			return err
		}
	}
	return nil
}

func Deserialize(r io.Reader) (*Profile, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	// Version.
	_, err := varint.Read(br)
	if err != nil {
		return nil, err
	}

	// Stacks length.
	var l uint64
	l, err = varint.Read(br)
	if err != nil {
		return nil, err
	}

	p := Profile{Stacks: make([]Stack, l)}
	var s Stack
	for i := uint64(0); i < l; i++ {
		s.ID, err = varint.Read(br)
		if err != nil {
			return nil, err
		}
		s.Value, err = varint.Read(br)
		if err != nil {
			return nil, err
		}
		p.Stacks[i] = s
	}

	return &p, nil
}
