package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrRoleUnknown = ValidationError{errors.New("unknown role")}
)

type Role int

const (
	_ Role = iota + 1
	ViewerRole
	EditorRole
	AdminRole
)

var roles = map[Role]string{
	AdminRole:  "Admin",
	EditorRole: "Editor",
	ViewerRole: "Viewer",
}

func (r Role) String() string {
	s, ok := roles[r]
	if !ok {
		return fmt.Sprintf("invalid role (%d)", r)
	}
	return s
}

func (r Role) IsValid() bool {
	_, ok := roles[r]
	return ok
}

func ParseRole(s string) (Role, error) {
	for k, v := range roles {
		if strings.ToLower(s) == strings.ToLower(v) {
			return k, nil
		}
	}
	return 0, ErrRoleUnknown
}

func (r *Role) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	x, err := ParseRole(s)
	if err != nil {
		return err
	}
	*r = x
	return nil
}

func (r Role) MarshalJSON() ([]byte, error) {
	if r.IsValid() {
		return json.Marshal(r.String())
	}
	return nil, ErrRoleUnknown
}
