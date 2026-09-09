package symbolref

import "fmt"

// FallbackSymbolName renders the display name for an address that could not
// be resolved to a function name: "binary!0xaddr", or "unknown!0xaddr" when
// the binary name is empty.
func FallbackSymbolName(binaryName string, addr uint64) string {
	if binaryName == "" {
		binaryName = "unknown"
	}
	return fmt.Sprintf("%s!0x%x", binaryName, addr)
}
