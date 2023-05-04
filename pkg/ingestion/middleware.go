package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

type MiddlewareIngester struct{}

func NewMiddlewareIngester() *MiddlewareIngester {
	return &MiddlewareIngester{}
}
func (*MiddlewareIngester) Ingest(_ context.Context, data *IngestInput) error {
	fmt.Println("middleware happens here ...")

	fmt.Println("data type...\n", reflect.TypeOf(data))
	fmt.Println("data format...\n", data.Format)
	_, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error:", err)
		// return
	}

	fmt.Println("data ---------------")
	// fmt.Println(string(jsonBytes))

	_, err2 := json.Marshal(data.Profile)
	if err2 != nil {
		fmt.Println("Error:", err)
		// return
	}

	fmt.Println("triedata ---------------")
	// fmt.Println(string(jsonBytes2))

	return nil
}
