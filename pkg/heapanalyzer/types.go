package heapanalyzer

import (
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

type HeapDump struct {
	Id string `json:"id"`

	// HeapStats - stats of the heap
	HeapStats HeapStats `json:"stats"`

	// unix millis
	CreatedAt int64           `json:"createdAt"`
	Labels    *typesv1.Labels `json:"labels"`
}

// HeapStats represents the stats of a heap
type HeapStats struct {
	// TotalSize - total size of the heap in bytes
	TotalSize int64 `json:"totalSize"`
	// TotalObjects - number of objects in the heap
	TotalObjects int64 `json:"totalObjects"`
}

// ObjectTypeStats represents the stats of a heap object type
// like the Type (name),Count (number of objects),
// and Size (total size of all objects of this type in bytes)
type ObjectTypeStats struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
	Size  int64  `json:"size"`
}

// ObjectTypesResult is a wrapper for the ObjectTypeStats
// that includes the total count and size of all objects
// warning: TotalCount is not the same as len(Items)
// it's a sum of all the counts in the Items
type ObjectTypesResult struct {
	TotalCount int64              `json:"totalCount"`
	TotalSize  int64              `json:"totalSize"`
	Items      []*ObjectTypeStats `json:"items"`
}

// ObjectResults is a wrapper for the Object
// that includes the total count and size of all objects
// in heap
type ObjectResults struct {
	TotalCount int64     `json:"totalCount"`
	TotalSize  int64     `json:"totalSize"`
	Items      []*Object `json:"items"`
}

type Object struct {
	Id          string `json:"id"`
	Type        string `json:"type"`
	Address     string `json:"address"`
	DisplayName string `json:"displayName"`
	Size        int64  `json:"size"`
}

type Field struct {
	Name     string `json:"name,omitempty"`
	Type     string `json:"type"`
	Value    string `json:"value,omitempty"`
	ValueHex string `json:"value_hex,omitempty"` // for debugging unknowns
	Pointer  string `json:"pointer,omitempty"`

	Fields []*Field `json:"fields,omitempty"`
}

type ObjectWithDetails struct {
	Object

	Fields     []*Field     `json:"fields"`
	References []*Reference `json:"references"`
}

type Reference struct {
	From    string `json:"from"`
	Type    string `json:"type"`
	Reason  string `json:"reason,omitempty"`
	Pointer string `json:"pointer,omitempty"`
}
