package model

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestUser_RoleJSON(t *testing.T) {
	x := struct{ Role Role }{}
	b := []byte(`{"Role":"Admin"}`)
	err := json.Unmarshal(b, &x)
	if err != nil {
		panic(err)
	}
	r, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	if !bytes.Equal(r, b) {
		panic(string(r))
	}
}
