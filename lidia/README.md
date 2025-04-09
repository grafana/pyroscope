# Lidia

Lidia is a custom binary format for efficient symbolization of Go profiling data.

## Features

- Fast lookup of function symbols by address
- Compact binary format
- CRC32C checksums for data integrity
- Support for source file and line information

## Installation

```bash
go get github.com/grafana/pyroscope/lidia
```

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

## Available Options

- `WithCRC()`: Enables CRC32C checksums for data integrity
- `WithFiles()`: Includes source file information
- `WithLines()`: Includes line number information

## File Format

Lidia uses a binary format with the following sections:
- Header: Magic number, version, and section tables
- VA Table: Sorted virtual addresses
- Range Table: Function symbol information
- Strings Table: String data pool
- Line Tables: Source line information (when included)

## Versioning

This module is currently in development (v0.x). The API may change until v1.0.0 is released.

We follow semantic versioning.

## License

See the LICENSE file for details.
