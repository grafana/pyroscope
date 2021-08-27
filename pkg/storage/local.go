package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/varint"
)

func (s *Storage) collectLocalProfile(path string) error {
	defer os.Remove(path)

	logrus.WithField("path", path).Debug("collecting local profile")

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	r := bytes.NewReader(b)

	l, err := varint.Read(r)
	if err != nil {
		return err
	}
	nameBuf := make([]byte, l)

	_, err = r.Read(nameBuf)
	if err != nil {
		return err
	}

	l, err = varint.Read(r)
	if err != nil {
		return err
	}

	metadataBuf := make([]byte, l)
	_, err = r.Read(metadataBuf)
	if err != nil {
		return err
	}
	pi := PutInput{}
	metadataReader := bytes.NewReader(metadataBuf)
	d := json.NewDecoder(metadataReader)
	err = d.Decode(&pi)
	if err != nil {
		return err
	}

	t, err := tree.DeserializeNoDict(r)
	if err != nil {
		return err
	}

	pi.Key, err = segment.ParseKey(string(nameBuf))
	if err != nil {
		return err
	}
	pi.Val = t

	return s.Put(&pi)
}

func (s *Storage) CollectLocalProfiles() error {
	matches, err := filepath.Glob(filepath.Join(s.localProfilesDir, "*.profile"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		if err := s.collectLocalProfile(path); err != nil {
			logrus.WithError(err).WithField("path", path).Error("failed to collect local profile")
		}
	}
	return nil
}

func (s *Storage) PutLocal(po *PutInput) error {
	logrus.Debug("PutLocal")
	if err := s.performFreeSpaceCheck(); err != nil {
		return err
	}

	name := fmt.Sprintf("%d-%s.profile", po.StartTime.Unix(), po.Key.AppName())

	buf := bytes.Buffer{}

	metadataBuf := bytes.Buffer{}
	t := po.Val
	po.Val = nil
	e := json.NewEncoder(&metadataBuf)
	if err := e.Encode(po); err != nil {
		return err
	}

	nameBuf := []byte(po.Key.Normalized())
	varint.Write(&buf, uint64(len(nameBuf)))
	buf.Write(nameBuf)

	mb := metadataBuf.Bytes()
	varint.Write(&buf, uint64(len(mb)))
	buf.Write(mb)

	if err := t.SerializeNoDict(s.config.MaxNodesSerialization, &buf); err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(s.localProfilesDir, name), buf.Bytes(), 0600); err != nil {
		return err
	}

	return nil
}
