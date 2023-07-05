package symtab

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ProcMapPermissions contains permission settings read from `/proc/[pid]/maps`.
type ProcMapPermissions struct {
	// mapping has the [R]ead flag set
	Read bool
	// mapping has the [W]rite flag set
	Write bool
	// mapping has the [X]ecutable flag set
	Execute bool
	// mapping has the [S]hared flag set
	Shared bool
	// mapping is marked as [P]rivate (copy on write)
	Private bool
}

// ProcMap contains the process memory-mappings of the process
// read from `/proc/[pid]/maps`.
type ProcMap struct {
	// The start address of current mapping.
	StartAddr uint64
	// The end address of the current mapping
	EndAddr uint64
	// The permissions for this mapping
	Perms *ProcMapPermissions
	// The current offset into the file/fd (e.g., shared libs)
	Offset int64
	// Device owner of this mapping (major:minor) in Mkdev format.
	Dev uint64
	// The inode of the device above
	Inode uint64
	// The file or psuedofile (or empty==anonymous)
	Pathname string
}

type file struct {
	dev   uint64
	inode uint64
	path  string
}

func (m *ProcMap) file() file {
	return file{
		dev:   m.Dev,
		inode: m.Inode,
		path:  m.Pathname,
	}
}

// parseDevice parses the device token of a line and converts it to a dev_t
// (mkdev) like structure.
func parseDevice(s []byte) (uint64, error) {
	i := bytes.IndexByte(s, ':')
	if i == -1 {
		return 0, fmt.Errorf("unexpected number of fields")
	}
	majorBytes := s[:i]
	minorBytes := s[i+1:]

	major, err := strconv.ParseUint(tokenToStringUnsafe(majorBytes), 16, 0)
	if err != nil {
		return 0, err
	}

	minor, err := strconv.ParseUint(tokenToStringUnsafe(minorBytes), 16, 0)
	if err != nil {
		return 0, err
	}

	return unix.Mkdev(uint32(major), uint32(minor)), nil
}

// parseAddress converts a hex-string to a uintptr.
func parseAddress(s []byte) (uint64, error) {
	a, err := strconv.ParseUint(tokenToStringUnsafe(s), 16, 0)
	if err != nil {
		return 0, err
	}

	return a, nil
}

// parseAddresses parses the start-end address.
func parseAddresses(s []byte) (uint64, uint64, error) {
	i := bytes.IndexByte(s, '-')
	if i == -1 {
		return 0, 0, fmt.Errorf("invalid address")
	}
	saddrBytes := s[:i]
	eaddrBytes := s[i+1:]

	saddr, err := parseAddress(saddrBytes)
	if err != nil {
		return 0, 0, err
	}

	eaddr, err := parseAddress(eaddrBytes)
	if err != nil {
		return 0, 0, err
	}

	return saddr, eaddr, nil
}

// parsePermissions parses a token and returns any that are set.
func parsePermissions(s []byte) (*ProcMapPermissions, error) {
	if len(s) < 4 {
		return nil, fmt.Errorf("invalid permissions token")
	}

	perms := ProcMapPermissions{}
	for _, ch := range s {
		switch ch {
		case 'r':
			perms.Read = true
		case 'w':
			perms.Write = true
		case 'x':
			perms.Execute = true
		case 'p':
			perms.Private = true
		case 's':
			perms.Shared = true
		}
	}

	return &perms, nil
}

// parseProcMap will attempt to parse a single line within a proc/[pid]/maps
// buffer.
// 7f5822ebe000-7f5822ec0000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
// returns nil if entry is not executable
func parseProcMap(line []byte) (*ProcMap, error) {
	var i int
	if i = bytes.IndexByte(line, ' '); i == -1 {
		return nil, fmt.Errorf("invalid procmap entry: %s", line)
	}
	addresesBytes := line[:i]
	line = line[i+1:]

	if i = bytes.IndexByte(line, ' '); i == -1 {
		return nil, fmt.Errorf("invalid procmap entry: %s", line)
	}
	permsBytes := line[:i]
	line = line[i+1:]

	if i = bytes.IndexByte(line, ' '); i == -1 {
		return nil, fmt.Errorf("invalid procmap entry: %s", line)
	}
	offsetBytes := line[:i]
	line = line[i+1:]

	if i = bytes.IndexByte(line, ' '); i == -1 {
		return nil, fmt.Errorf("invalid procmap entry: %s", line)
	}
	deviceBytes := line[:i]
	line = line[i+1:]

	var inodeBytes []byte
	if i = bytes.IndexByte(line, ' '); i == -1 {
		inodeBytes = line
		line = nil
	} else {
		inodeBytes = line[:i]
		line = line[i+1:]
	}

	perms, err := parsePermissions(permsBytes)
	if err != nil {
		return nil, err
	}

	if !perms.Execute {
		return nil, nil
	}

	saddr, eaddr, err := parseAddresses(addresesBytes)
	if err != nil {
		return nil, err
	}

	offset, err := strconv.ParseInt(tokenToStringUnsafe(offsetBytes), 16, 0)
	if err != nil {
		return nil, err
	}

	device, err := parseDevice(deviceBytes)
	if err != nil {
		return nil, err
	}

	inode, err := strconv.ParseUint(tokenToStringUnsafe(inodeBytes), 10, 0)
	if err != nil {
		return nil, err
	}

	pathname := ""

	for len(line) > 0 && line[0] == ' ' {
		line = line[1:]
	}
	if len(line) > 0 {
		pathname = string(line)
	}

	return &ProcMap{
		StartAddr: saddr,
		EndAddr:   eaddr,
		Perms:     perms,
		Offset:    offset,
		Dev:       device,
		Inode:     inode,
		Pathname:  pathname,
	}, nil
}

func parseProcMapsExecutableModules(procMaps []byte) ([]*ProcMap, error) {
	var modules []*ProcMap
	for len(procMaps) > 0 {
		nl := bytes.IndexByte(procMaps, '\n')
		var line []byte
		if nl == -1 {
			line = procMaps
			procMaps = nil
		} else {
			line = procMaps[:nl]
			procMaps = procMaps[nl+1:]
		}
		if len(line) == 0 {
			continue
		}
		m, err := parseProcMap(line)
		if err != nil {
			return nil, err
		}
		if m == nil { // not executable
			continue
		}
		modules = append(modules, m)
	}
	return modules, nil
}

func tokenToStringUnsafe(tok []byte) string {
	res := ""
	// todo remove unsafe

	sh := (*reflect.StringHeader)(unsafe.Pointer(&res))
	sh.Data = uintptr(unsafe.Pointer(&tok[0]))
	sh.Len = len(tok)
	return res
}
