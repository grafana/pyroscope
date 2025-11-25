package main

import (
	"encoding/xml"
	"fmt"
)

// POM represents a Maven Project Object Model XML structure.
type POM struct {
	XMLName xml.Name `xml:"project"`
	GroupID string   `xml:"groupId"`
	URL     string   `xml:"url"`
	SCM     SCM      `xml:"scm"`
	Parent  Parent   `xml:"parent"`
}

// Parent represents a parent POM reference.
type Parent struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

// SCM represents Source Control Management information in a POM.
type SCM struct {
	URL        string `xml:"url"`
	Connection string `xml:"connection"`
	Tag        string `xml:"tag"`
}

type POMParser struct{}

func (p *POMParser) Parse(data []byte) (*POM, error) {
	var pom POM
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil, fmt.Errorf("invalid POM XML: %w", err)
	}
	return &pom, nil
}

func (p *POMParser) ExtractGroupID(data []byte) (string, error) {
	pom, err := p.Parse(data)
	if err != nil {
		return "", err
	}
	return pom.GroupID, nil
}

func (p *POMParser) ParseSCM(data []byte) (*SCM, error) {
	pom, err := p.Parse(data)
	if err != nil {
		return nil, err
	}

	if pom.SCM.URL == "" && pom.SCM.Connection == "" {
		return nil, fmt.Errorf("no SCM information found")
	}

	scm := &pom.SCM
	if scm.URL == "" {
		scm.URL = scm.Connection
	}

	return scm, nil
}
