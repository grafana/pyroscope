// options.go
package lidia

// Option configures a Table or file creation process.
type Option func(*options)

// Options for controlling the behavior of lidia operations
type options struct {
	crc   bool // Enable CRC checking
	lines bool // Include line number information
	files bool // Include file path information

	parseGoPclntab bool
	symtab         bool
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

func WithParseGoPclntab(parse bool) Option {
	return func(o *options) {
		o.parseGoPclntab = parse
	}
}

func WithSymtab(parse bool) Option {
	return func(o *options) {
		o.symtab = parse
	}
}
