package lidia

import "hash/crc32"

// File format constants
const (
	// Magic number for lidia files ("ldia" in little-endian ASCII)
	// magic uint32 = 0x6169646c

	// Current version of the lidia format
	version uint32 = 1

	// Size of the file header in bytes
	headerSize = 0x80

	// Number of fields in a line table entry
	lineTableFieldsCount = 2

	// Number of fields in a range entry
	fieldsCount = 8

	// Size of a range entry with 4-byte fields
	fieldsEntrySize4 = fieldsCount * 4

	// Size of a range entry with 8-byte fields
	fieldsEntrySize8 = fieldsCount * 8
)

// CRC32 table using the Castagnoli polynomial
var (
	castagnoli = crc32.MakeTable(crc32.Castagnoli)
	magic      = []byte{0x2e, 0x64, 0x69, 0x61} // ".dia"
)
