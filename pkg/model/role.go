package model

import (
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrRoleUnknown = ValidationError{errors.New("unknown role")}
)

type Role int

const (
	InvalidRole Role = iota
	ReadOnlyRole
	AgentRole
	AdminRole
)

var roles = map[Role]string{
	AdminRole:    "Admin",
	AgentRole:    "Agent",
	ReadOnlyRole: "ReadOnly",
}

func (r Role) String() string {
	s, _ := roles[r]
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
	return InvalidRole, ErrRoleUnknown
}

func (r *Role) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*r, _ = ParseRole(s)
	return nil
}

func (r Role) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}
