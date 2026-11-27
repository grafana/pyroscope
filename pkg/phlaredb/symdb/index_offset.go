package symdb

import "fmt"

// IndexOffset reads the section-relative index offset from a serialized V3 file.
func IndexOffset(data []byte) (uint64, error) {
	if len(data) < FooterSize {
		return 0, fmt.Errorf("symbols section is smaller than its footer")
	}
	var footer Footer
	if err := footer.UnmarshalBinary(data[len(data)-FooterSize:]); err != nil {
		return 0, err
	}
	if footer.IndexOffset > uint64(len(data)-FooterSize) {
		return 0, fmt.Errorf("symbols index offset exceeds section size")
	}
	return footer.IndexOffset, nil
}
