// options.go
package lidia

// Option configures a Table or file creation process.
type Option func(*options)

// Options for controlling the behavior of lidia operations
type options struct {
	crc   bool // Enable CRC checking
	lines bool // Include line number information
	files bool // Include file path information
}

// WithCRC enables CRC checking when opening lidia files.
func WithCRC() Option {
	return func(o *options) {
		o.crc = true
	}
}

// WithLines includes line number information in created lidia files.
func WithLines() Option {
	return func(o *options) {
		o.lines = true
	}
}

// WithFiles includes file path information in created lidia files.
func WithFiles() Option {
	return func(o *options) {
		o.files = true
	}
}
