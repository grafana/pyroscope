package elf

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
)

type BuildID struct {
	ID  string
	Typ string
}

func GNUBuildID(s string) BuildID {
	return BuildID{ID: s, Typ: "gnu"}
}
func GoBuildID(s string) BuildID {
	return BuildID{ID: s, Typ: "go"}
}

func (b *BuildID) Empty() bool {
	return b.ID == "" || b.Typ == ""
}

func (b *BuildID) GNU() bool {
	return b.Typ == "gnu"
}

var (
	ErrNoBuildIDSection = fmt.Errorf("build ID section not found")
)

func (f *MMapedElfFile) BuildID() (BuildID, error) {
	id, err := f.GNUBuildID()
	if err != nil && !errors.Is(err, ErrNoBuildIDSection) {
		return BuildID{}, err
	}
	if !id.Empty() {
		return id, nil
	}
	id, err = f.GoBuildID()
	if err != nil && !errors.Is(err, ErrNoBuildIDSection) {
		return BuildID{}, err
	}
	if !id.Empty() {
		return id, nil
	}

	return BuildID{}, ErrNoBuildIDSection
}

var goBuildIDSep = []byte("/")

func (f *MMapedElfFile) GoBuildID() (BuildID, error) {
	buildIDSection := f.Section(".note.go.buildid")
	if buildIDSection == nil {
		return BuildID{}, ErrNoBuildIDSection
	}
	data, err := f.SectionData(buildIDSection)
	if err != nil {
		return BuildID{}, fmt.Errorf("reading .note.go.buildid %w", err)
	}
	if len(data) < 17 {
		return BuildID{}, fmt.Errorf(".note.gnu.build-id is too small")
	}

	data = data[16 : len(data)-1]
	if len(data) < 40 || bytes.Count(data, goBuildIDSep) < 2 {
		return BuildID{}, fmt.Errorf("wrong .note.go.buildid %s", f.fpath)
	}
	id := string(data)
	if id == "redacted" {
		return BuildID{}, fmt.Errorf("blacklisted  .note.go.buildid %s", f.fpath)
	}
	return GoBuildID(id), nil
}

func (f *MMapedElfFile) GNUBuildID() (BuildID, error) {
	buildIDSection := f.Section(".note.gnu.build-id")
	if buildIDSection == nil {
		return BuildID{}, ErrNoBuildIDSection
	}

	data, err := f.SectionData(buildIDSection)
	if err != nil {
		return BuildID{}, fmt.Errorf("reading .note.gnu.build-id %w", err)
	}
	if len(data) < 16 {
		return BuildID{}, fmt.Errorf(".note.gnu.build-id is too small")
	}
	if !bytes.Equal([]byte("GNU"), data[12:15]) {
		return BuildID{}, fmt.Errorf(".note.gnu.build-id is not a GNU build-id")
	}
	rawBuildID := data[16:]
	if len(rawBuildID) != 20 && len(rawBuildID) != 8 { // 8 is xxhash, for example in Container-Optimized OS
		return BuildID{}, fmt.Errorf(".note.gnu.build-id has wrong size %s", f.fpath)
	}
	buildIDHex := hex.EncodeToString(rawBuildID)
	return GNUBuildID(buildIDHex), nil
}
