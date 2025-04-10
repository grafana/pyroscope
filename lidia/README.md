# Lidia (Language-Independent-Debug-Information-Archive)

Lidia is a binary format for efficient lookup of symbols of a binary by virtual address.

## Features

- Fast lookup of function symbols by address
- Compact binary format
- CRC32C checksums for data integrity
- Support for source file and line information

## Features

The Lidia format achieves its performance through a carefully designed architecture:

1. **Fast Lookups**: Uses binary search on the sorted VA Table to quickly locate functions containing a specific address
2. **Direct Access**: Each VA Table entry has a corresponding Range Table entry at the same index, providing O(1) access to function metadata
3. **Memory Efficiency**: Stores each string only once in the Strings Table and references them by offset throughout the file
4. **Size Optimization**: Adapts field sizes (4 or 8 bytes) based on actual data values to minimize file size
5. **Minimal Parsing**: Stores data in a binary format that can be memory-mapped with minimal transformation

This design makes Lidia particularly well-suited for applications that need to perform many address lookups, such as symbolizing profiles with thousands of samples.

## Installation

```bash
go get github.com/grafana/pyroscope/lidia
```

## File Format

Lidia uses a binary format with the following sections:
- Header: Magic number, version, and section tables
- VA Table: Sorted virtual addresses
- Range Table: Function symbol information
- Strings Table: String data pool
- Line Tables: Source line information (when included)

## Format Specification

The Lidia file format consists of the following sections in order:

### 1. Header (128 bytes)

The header contains metadata about the file and its sections:

| Offset | Size | Description                                     |
|--------|------|-------------------------------------------------|
| 0x00   | 4    | Magic number: [0x2e, 0x64, 0x69, 0x61] (".dia") |
| 0x04   | 4    | Version number (currently 1)                    |
| 0x08   | 32   | VA Table Header                                 |
| 0x28   | 32   | Range Table Header                              |
| 0x48   | 24   | Strings Table Header                            |
| 0x60   | 32   | Line Tables Header                              |

#### VA Table Header (32 bytes at offset 0x08)
| Offset | Size | Description |
|--------|------|-------------|
| 0x00   | 8    | Entry size (4 or 8 bytes) |
| 0x08   | 8    | Number of entries |
| 0x10   | 8    | Offset to table data |
| 0x18   | 4    | CRC32C checksum |
| 0x1C   | 4    | (reserved) |

#### Range Table Header (32 bytes at offset 0x28)
| Offset | Size | Description |
|--------|------|-------------|
| 0x00   | 8    | Field size (4 or 8 bytes) |
| 0x08   | 8    | Number of entries |
| 0x10   | 8    | Offset to table data |
| 0x18   | 4    | CRC32C checksum |
| 0x1C   | 4    | (reserved) |

#### Strings Table Header (24 bytes at offset 0x48)
| Offset | Size | Description |
|--------|------|-------------|
| 0x00   | 8    | Size of strings data |
| 0x08   | 8    | Offset to table data |
| 0x10   | 4    | CRC32C checksum |
| 0x14   | 4    | (reserved) |

#### Line Tables Header (32 bytes at offset 0x60)
| Offset | Size | Description |
|--------|------|-------------|
| 0x00   | 8    | Field size (2 or 4 bytes) |
| 0x08   | 8    | Number of entries |
| 0x10   | 8    | Offset to table data |
| 0x18   | 4    | CRC32C checksum |
| 0x1C   | 4    | (reserved) |

### 2. VA Table (Variable size)

Follows immediately after the header at offset 0x80. Contains virtual addresses (VAs) of functions, sorted in ascending order.
- Each entry is either 4 or 8 bytes, as specified in the VA Table Header.
- The number of entries is specified in the VA Table Header.

### 3. Range Table (Variable size)

Follows after the VA Table. Contains information about each function range.
Each entry contains 8 fields:

| Field       | Description |
|-------------|-------------|
| length      | Length of the function in bytes |
| depth       | Inlining depth (0 for non-inlined functions) |
| funcOffset  | Offset into the Strings Table for the function name |
| fileOffset  | Offset into the Strings Table for the source file path |
| lineTable   | {idx, count} Reference to Line Table entries |
| callFile    | Offset into the Strings Table for the call site file path |
| callLine    | Line number at the call site |

Each field is either 4 or 8 bytes, as specified in the Range Table Header.

### 4. Strings Table (Variable size)

Follows after the Range Table. Contains null-terminated strings referenced by the Range Table.
- Strings are stored in a consecutive block.
- References to strings are offsets within this table.

### 5. Line Tables (Variable size)

Follows after the Strings Table. Contains line number information for functions.
Each entry contains two fields:

| Field      | Description |
|------------|-------------|
| Offset     | Offset within the function |
| LineNumber | Source line number at this offset |

Each field is either 2 or 4 bytes, as specified in the Line Tables Header.

### CRC32C Checksums

If enabled with `WithCRC()`, each section has a CRC32C checksum for data integrity validation.
The checksums are calculated using the Castagnoli polynomial.

## Usage

### Opening and Querying

```go
import "github.com/grafana/pyroscope/lidia"

// Open a lidia file
file, err := os.Open("symbolization.lidia")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

table, err := lidia.OpenReader(file, lidia.WithCRC())
if err != nil {
    log.Fatal(err)
}
defer table.Close()

// Look up a function symbol by address
frames, err := table.Lookup(0x408ed0)
if err != nil {
    log.Fatal(err)
}

for _, frame := range frames {
    fmt.Printf("Function: %s\n", frame.FunctionName)
    if frame.SourceFile != "" {
        fmt.Printf("  File: %s:%d\n", frame.SourceFile, frame.SourceLine)
    }
}
```

### Creating Lidia Files

```go
// Create from an executable file
err := lidia.CreateLidia("path/to/executable", "output.lidia",
    lidia.WithCRC(), lidia.WithLines(), lidia.WithFiles())
if err != nil {
    log.Fatal(err)
}

// Or from an already opened ELF file
elfFile, err := elf.Open("path/to/executable")
if err != nil {
    log.Fatal(err)
}
defer elfFile.Close()

output, err := os.Create("output.lidia")
if err != nil {
    log.Fatal(err)
}
defer output.Close()

err = lidia.CreateLidiaFromELF(elfFile, output,
    lidia.WithCRC(), lidia.WithLines(), lidia.WithFiles())
if err != nil {
    log.Fatal(err)
}
```

## Versioning

This module is currently in development (v0.x). The API may change until v1.0.0 is released.

We follow semantic versioning.

## License

See the [LICENSE](./LICENSE) file for details.
