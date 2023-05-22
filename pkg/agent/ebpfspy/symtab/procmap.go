package symtab

import (
	"bytes"
	"fmt"
	"strconv"
)

type procMapEntry struct {
	start  uint64
	end    uint64
	offset uint64
	inode  uint64
	file   string
}

func parseProcMaps(procMaps []byte) ([]procMapEntry, error) {
	lines := bytes.Split(procMaps, []byte("\n"))
	var modules []procMapEntry

	//if (fscanf(procmap, "%lx-%lx %4s %llx %lx:%lx %lu%[^\n]",
	//	&mod.start_addr, &mod.end_addr, perm, &mod.file_offset,
	//	&mod.dev_major, &mod.dev_minor, &mod.inode, buf) != 8)

	for _, line := range lines {
		parts := bytes.Fields(line)
		if len(parts) != 6 {
			continue
		}

		addrs := bytes.Split(parts[0], []byte("-"))
		if len(addrs) != 2 {
			continue
		}
		start, err := strconv.ParseUint(string(addrs[0]), 16, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing start address: %w", err)
		}
		end, err := strconv.ParseUint(string(addrs[1]), 16, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing end address: %w", err)
		}

		perm := parts[1]
		if perm[2] != 'x' {
			continue
		}

		inode, err := strconv.ParseUint(string(parts[4]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing inode: %w", err)
		}
		offset, err := strconv.ParseUint(string(parts[2]), 16, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing file offset: %w", err)
		}

		file := string(parts[5])

		modules = append(modules, procMapEntry{
			start:  start,
			end:    end,
			offset: offset,
			inode:  inode,
			file:   file,
		})
	}

	return modules, nil
}
