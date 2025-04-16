// Package lidia implements a custom binary format for efficient symbolization of Go profiles.
//
// Lidia provides functionality to create, read, and query symbol tables stored in the
// lidia binary format. This format is optimized for fast symbol lookups by address,
// which is useful for symbolizing profiles collected from Go programs.
//
// # Creating Lidia Files
//
// There are two main ways to create a lidia file:
//
//	// From an executable file
//	err := lidia.CreateLidia("path/to/executable", "output.lidia",
//	    lidia.WithCRC(), lidia.WithLines(), lidia.WithFiles())
//	if err != nil {
//	    // handle error
//	}
//
//	// From an already opened ELF file
//	elfFile, err := elf.Open("path/to/executable")
//	if err != nil {
//	    // handle error
//	}
//	defer elfFile.Close()
//
//	output, err := os.Create("output.lidia")
//	if err != nil {
//	    // handle error
//	}
//	defer output.Close()
//
//	err = lidia.CreateLidiaFromELF(elfFile, output,
//	    lidia.WithCRC(), lidia.WithLines(), lidia.WithFiles())
//	if err != nil {
//	    // handle error
//	}
//
// # Reading and Querying Lidia Files
//
//	// Read a lidia file into memory
//	data, err := os.ReadFile("path/to/file.lidia")
//	if err != nil {
//	    // handle error
//	}
//
//	// Create a reader from the data
//	var reader lidia.ReaderAtCloser = &MyReaderAtCloser{data, 0}
//
//	// Open the lidia table
//	table, err := lidia.OpenReader(reader, lidia.WithCRC())
//	if err != nil {
//	    // handle error
//	}
//	defer table.Close()
//
//	// Look up a function symbol by address
//	frames, err := table.Lookup(0x408ed0)
//	if err != nil {
//	    // handle error
//	}
//
//	// Use the symbolization results
//	for _, frame := range frames {
//	    fmt.Println(frame.FunctionName)
//	}
//
// # Available Options
//
// The following options can be used when creating or opening lidia files:
//
//   - WithCRC(): Enables CRC32C checksums for data integrity
//   - WithFiles(): Includes source file information
//   - WithLines(): Includes line number information
//
// When creating a lidia file with WithCRC(), the same option must be used when
// opening the file, or an error will be returned.
package lidia
