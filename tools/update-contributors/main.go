package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

type contributor struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

func fetchContributors(ctx context.Context, owner, repo string) ([]contributor, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contributors?per_page=200", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	var contributors []contributor
	if err := json.NewDecoder(resp.Body).Decode(&contributors); err != nil {
		return nil, err
	}

	return contributors, nil
}

func generateContributors() (string, error) {
	ctx := context.Background()
	contributors, err := fetchContributors(ctx, "grafana", "pyroscope")
	if err != nil {
		return "", err
	}

	sb := strings.Builder{}

	limit := 9 * 7

	for _, c := range contributors {
		// filter bots
		if strings.HasSuffix(c.Login, "[bot]") || c.Login == "pyroscopebot" {
			continue
		}
		sb.WriteString(`<a href="`)
		sb.WriteString(c.HTMLURL)
		sb.WriteString(`"><img src="`)
		sb.WriteString(c.AvatarURL)
		sb.WriteString(`" title="`)
		sb.WriteString(c.Login)
		sb.WriteString(`" width="80" height="80"></a>`)
		sb.WriteByte('\n')
		limit--
		if limit == 0 {
			break
		}
	}

	return sb.String(), nil
}

const (
	marker     = "[//]: contributor-faces"
	readmeFile = "README.md"
)

func replaceReadme() error {
	b, err := os.ReadFile(readmeFile)
	if err != nil {
		return err
	}
	s := string(b)

	start := strings.Index(s, marker)
	if start == -1 {
		return fmt.Errorf("could not find marker %q", marker)
	}
	start += len(marker) + 1

	end := strings.Index(s[start:], marker)
	if end == -1 {
		return fmt.Errorf("could not find end marker %q", marker)
	}
	end += start

	contributor, err := generateContributors()
	if err != nil {
		return err
	}

	f, err := os.Create(readmeFile)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = f.WriteString(s[:start])
	if err != nil {
		return err
	}

	_, err = f.WriteString(contributor)
	if err != nil {
		return err
	}

	_, err = f.WriteString("\n")
	if err != nil {
		return err
	}

	_, err = f.WriteString(s[end:])
	if err != nil {
		return err
	}

	return nil
}

func main() {
	if err := replaceReadme(); err != nil {
		log.Fatal(err)
	}
}
