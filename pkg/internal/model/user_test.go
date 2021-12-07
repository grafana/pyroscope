package model

import (
	"math/rand"
	"testing"
)

func TestUser_VerifyPassword(t *testing.T) {
	p := make([]byte, 8)
	rand.Read(p)
	h := MustPasswordHash(p)
	if err := VerifyPassword(h, p); err != nil {
		panic(err)
	}
	p[0]++
	if err := VerifyPassword(h, p); err == nil {
		panic("!")
	}
}
