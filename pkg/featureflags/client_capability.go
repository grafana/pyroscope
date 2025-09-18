package featureflags

import (
	"fmt"
	"mime"
	"net/http"
)

const (
	// Capability names
	AllowUtf8LabelNamesCapabilityName = "allow-utf8-labelnames"
)

type ClientCapability struct {
	Name  string
	Value string
}

func ParseClientCapabilities(header http.Header) ([]*ClientCapability, error) {
	acceptHeader := header.Get("Accept")
	if acceptHeader != "" {
		if _, params, err := mime.ParseMediaType(acceptHeader); err != nil {
			return nil, err
		} else {
			capabilities := make([]*ClientCapability, 0, len(params))
			seenCapabilityNames := make(map[string]struct{})
			for k, v := range params {
				// Check for duplicates
				if _, ok := seenCapabilityNames[k]; ok {
					return nil, fmt.Errorf("duplicate client capabilities parsed from `Accept:` header: '%s'",
						params)
				}
				seenCapabilityNames[k] = struct{}{}

				capabilities = append(capabilities, &ClientCapability{Name: k, Value: v})
			}

			return capabilities, nil
		}
	}

	return []*ClientCapability{}, nil
}

func GetClientCapability(capabilities []*ClientCapability, capabilityName string) *ClientCapability {
	for _, capability := range capabilities {
		if capability.Name == capabilityName {
			return capability
		}
	}
	return nil
}
