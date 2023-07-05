package symtab

import (
	"bytes"
	"fmt"
	"runtime"
	"strconv"
)

var kallsymsModule = []byte("kernel")

func NewKallsyms(kallsyms []byte) (*SymbolTab, error) {
	kernelAddrSpace := uint64(0)
	if runtime.GOARCH == "amd64" {
		// https://www.kernel.org/doc/Documentation/x86/x86_64/mm.txt
		kernelAddrSpace = 0x00ffffffffffffff
	}

	var syms []Symbol
	allZeros := true
	for len(kallsyms) > 0 {
		i := bytes.IndexByte(kallsyms, '\n')
		var line []byte
		if i == -1 {
			line = kallsyms
			kallsyms = nil
		} else {
			line = kallsyms[:i]
			kallsyms = kallsyms[i+1:]
		}

		if len(line) == 0 {
			continue
		}
		space := bytes.IndexByte(line, ' ')
		if space == -1 {
			return nil, fmt.Errorf("no space found")
		}
		addr := line[:space]
		line = line[space+1:]

		space = bytes.IndexByte(line, ' ')
		if space == -1 {
			return nil, fmt.Errorf("no space found")
		}
		typ := line[:space]
		line = line[space+1:]

		var name []byte
		var mod []byte
		tab := bytes.IndexByte(line, '\t')
		if tab == -1 {
			name = line
			mod = kallsymsModule
		} else {
			name = line[:tab]
			mod = line[tab+1:]
		}

		if typ[0] == 'b' || typ[0] == 'B' || typ[0] == 'd' ||
			typ[0] == 'D' || typ[0] == 'r' || typ[0] == 'R' {

			continue
		}

		istart, err := strconv.ParseUint(string(addr), 16, 64)
		if err != nil {
			return nil, err
		}
		if istart < kernelAddrSpace {
			continue
		}
		if bytes.HasPrefix(mod, []byte{'['}) && bytes.HasSuffix(mod, []byte{']'}) {
			mod = mod[1 : len(mod)-1]
		}
		if istart != 0 {
			allZeros = false
		}
		syms = append(syms, Symbol{istart, string(name), string(mod)})
	}
	if allZeros {
		return NewSymbolTab(nil), nil
	}
	return NewSymbolTab(syms), nil
}
