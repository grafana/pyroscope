package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/grafana/regexp"
)

var ghToken string

// this program requires gh cli, bundle, go to be installed
// todo run it by cron & create a PR if needed
func main() {

	getGHToken()

	updateGolang()
	updateGodeltaprof()
	updateJava()
	updateRuby()
	updatePython()
	updateDotnet()
	updateNodeJS()
}

func getGHToken() {
	ghToken, _ = s.sh("gh auth token")
}

func extractNodeJSVersion(tag Tag) *version {
	re := regexp.MustCompile("(v)(\\d+).(\\d+).(\\d+)")
	match := re.FindStringSubmatch(tag.Name)
	if match != nil {
		if match[1] == "v" {
			major, err := strconv.Atoi(match[2])
			requireNoError(err, "strconv")
			minor, err := strconv.Atoi(match[3])
			requireNoError(err, "strconv")
			patch, err := strconv.Atoi(match[4])
			requireNoError(err, "strconv")
			return &version{major: major, minor: minor, patch: patch, tag: tag}
		}
	}
	return nil
}

func updateNodeJS() {
	tags := getTagsV("grafana/pyroscope-nodejs", extractNodeJSVersion)
	last := tags[len(tags)-1]
	fmt.Println(last)

	replPackageJson := fmt.Sprintf(`    "@pyroscope/nodejs": "v%s",`, last.version())
	rePackageJson := regexp.MustCompile(`    "@pyroscope/nodejs": "[^"]+",`)
	for _, x := range []string{"express", "express-pull", "express-ts", "express-ts-inline"} {
		path := filepath.Join("examples/language-sdk-instrumentation/nodejs", x)
		replaceInplace(rePackageJson, filepath.Join(path, "package.json"), replPackageJson)
		s.sh(fmt.Sprintf(`cd "%s"       && yarn`, path))
	}
}

func updateDotnet() {
	tags := getTagsV("grafana/pyroscope-dotnet", extractDotnetVersion())
	last := tags[len(tags)-1]
	fmt.Println(last)

	reDockerGlibc := regexp.MustCompile("COPY --from=pyroscope/pyroscope-dotnet:\\d+\\.\\d+\\.\\d+-glibc")
	replDockerGlibc := fmt.Sprintf("COPY --from=pyroscope/pyroscope-dotnet:%s-glibc", last.version())
	replaceInplace(reDockerGlibc, "examples/language-sdk-instrumentation/dotnet/fast-slow/Dockerfile", replDockerGlibc)
	replaceInplace(reDockerGlibc, "examples/language-sdk-instrumentation/dotnet/rideshare/Dockerfile", replDockerGlibc)
	replaceInplace(reDockerGlibc, "examples/language-sdk-instrumentation/dotnet/web-new/Dockerfile", replDockerGlibc)
	replaceInplace(reDockerGlibc, "docs/sources/configure-client/language-sdks/dotnet.md", replDockerGlibc)

	reDockerMusl := regexp.MustCompile("COPY --from=pyroscope/pyroscope-dotnet:\\d+\\.\\d+\\.\\d+-musl")
	replDockerMusl := fmt.Sprintf("COPY --from=pyroscope/pyroscope-dotnet:%s-musl", last.version())
	replaceInplace(reDockerMusl, "examples/language-sdk-instrumentation/dotnet/fast-slow/musl.Dockerfile", replDockerMusl)
	replaceInplace(reDockerMusl, "examples/language-sdk-instrumentation/dotnet/rideshare/musl.Dockerfile", replDockerMusl)

	reUrl := regexp.MustCompile("https://github\\.com/grafana/pyroscope-dotnet/releases/download/v\\d+\\.\\d+\\.\\d+-pyroscope/pyroscope.\\d+\\.\\d+\\.\\d+-glibc-x86_64.tar.gz")
	replUrl := fmt.Sprintf("https://github.com/grafana/pyroscope-dotnet/releases/download/v%s-pyroscope/pyroscope.%s-glibc-x86_64.tar.gz", last.version(), last.version())
	replaceInplace(reUrl, "docs/sources/configure-client/language-sdks/dotnet.md", replUrl)
}

func updatePython() {
	tags := getTagsV("grafana/pyroscope-rs", extractRSVersion("python"))
	last := tags[len(tags)-1]
	fmt.Println(last)

	re := regexp.MustCompile("pyroscope-io==\\d+\\.\\d+\\.\\d+")
	repl := fmt.Sprintf("pyroscope-io==%s", last.version())
	replaceInplace(re, "examples/language-sdk-instrumentation/python/simple/requirements.txt", repl)
	replaceInplace(re, "examples/language-sdk-instrumentation/python/rideshare/flask/Dockerfile", repl)
	replaceInplace(re, "examples/language-sdk-instrumentation/python/rideshare/fastapi/Dockerfile", repl)
	replaceInplace(re, "examples/language-sdk-instrumentation/python/rideshare/django/app/requirements.txt", repl)

}

func updateRuby() {
	tags := getTagsV("grafana/pyroscope-rs", extractRSVersion("ruby"))
	last := tags[len(tags)-1]
	fmt.Println(last)

	re := regexp.MustCompile("gem ['\"]pyroscope['\"].*")
	repl := fmt.Sprintf("gem 'pyroscope', '= %s'", last.version())
	replaceInplace(re, "examples/language-sdk-instrumentation/ruby/rideshare/Gemfile", repl)
	replaceInplace(re, "examples/language-sdk-instrumentation/ruby/rideshare_rails/Gemfile", repl)
	replaceInplace(re, "examples/language-sdk-instrumentation/ruby/simple/Gemfile", repl)

	s.sh("cd examples/language-sdk-instrumentation/ruby/rideshare       && bundle update pyroscope")
	s.sh("cd examples/language-sdk-instrumentation/ruby/rideshare_rails && bundle update pyroscope")
	s.sh("cd examples/language-sdk-instrumentation/ruby/simple          && bundle update pyroscope")
}

func updateJava() {
	tags := getTagsV("grafana/pyroscope-java", extractGoVersion(""))
	last := tags[len(tags)-1]
	fmt.Println(last)
	reJarURL := regexp.MustCompile("https://github\\.com/grafana/pyroscope-java/releases/download/(v\\d+\\.\\d+\\.\\d+)/pyroscope\\.jar")
	lastJarURL := "https://github.com/grafana/pyroscope-java/releases/download/" + last.versionV() + "/pyroscope.jar"
	replaceInplace(reJarURL, "examples/language-sdk-instrumentation/java/fib/Dockerfile", lastJarURL)
	replaceInplace(reJarURL, "examples/language-sdk-instrumentation/java/simple/Dockerfile", lastJarURL)
	replaceInplace(reJarURL, "examples/language-sdk-instrumentation/java/rideshare/Dockerfile", lastJarURL)

	reGradelDep := regexp.MustCompile("implementation\\(\"io\\.pyroscope:agent:\\d+\\.\\d+\\.\\d+\"\\)")
	lastGradleDep := fmt.Sprintf("implementation(\"io.pyroscope:agent:%s\")", last.version())
	replaceInplace(reGradelDep, "examples/language-sdk-instrumentation/java/rideshare/build.gradle.kts", lastGradleDep)
	replaceInplace(reGradelDep, "docs/sources/configure-client/language-sdks/java.md", lastGradleDep)

	reMaven := regexp.MustCompile("<version>\\d+\\.\\d+\\.\\d+</version>")
	replMaven := fmt.Sprintf("<version>%s</version>", last.version())
	replaceInplace(reMaven, "docs/sources/configure-client/language-sdks/java.md", replMaven)

}

func replaceInplace(re *regexp.Regexp, file string, replacement string) {
	bs, err := os.ReadFile(file)
	requireNoError(err, "read file "+file)
	str1 := string(bs)
	str2 := re.ReplaceAllString(str1, replacement)
	err = os.WriteFile(file, []byte(str2), 066)
	requireNoError(err, "write file "+file)
}

func updateGodeltaprof() {
	tags := getTagsV("grafana/pyroscope-go", extractGoVersion("godeltaprof"))
	last := tags[len(tags)-1]
	log.Println(last.tag.Name)
	s.sh(" go get github.com/grafana/pyroscope-go/godeltaprof@" + last.versionV() + " && go mod tidy")
	s.sh("cd ./examples/language-sdk-instrumentation/golang-push/rideshare &&  go get github.com/grafana/pyroscope-go/godeltaprof@" + last.versionV() +
		" && go mod tidy")
	s.sh("cd ./examples/language-sdk-instrumentation/golang-push/simple    &&  go get github.com/grafana/pyroscope-go/godeltaprof@" + last.versionV() +
		" && go mod tidy")
}

var s = sh{}

func updateGolang() {
	tags := getTagsV("grafana/pyroscope-go", extractGoVersion(""))
	last2 := tags[len(tags)-1]
	last := last2
	log.Println(last.tag.Name)
	s.sh("cd ./examples/language-sdk-instrumentation/golang-push/rideshare &&  go get github.com/grafana/pyroscope-go@" + last.versionV() +
		" && go mod tidy")
	s.sh("cd ./examples/language-sdk-instrumentation/golang-push/simple    &&  go get github.com/grafana/pyroscope-go@" + last.versionV() +
		" && go mod tidy")
}

func extractGoVersion(module string) func(tag Tag) *version {
	return func(tag Tag) *version {
		re := regexp.MustCompile("([^/]*)/?v(\\d+).(\\d+).(\\d+)")
		match := re.FindStringSubmatch(tag.Name)
		if match != nil {
			//fmt.Println(len(match), match)
			if match[1] == module {
				major, err := strconv.Atoi(match[2])
				requireNoError(err, "strconv")
				minor, err := strconv.Atoi(match[3])
				requireNoError(err, "strconv")
				patch, err := strconv.Atoi(match[4])
				requireNoError(err, "strconv")
				return &version{major: major, minor: minor, patch: patch, tag: tag}
			}
		}
		return nil
	}
}

func extractRSVersion(module string) func(tag Tag) *version {
	return func(tag Tag) *version {
		re := regexp.MustCompile("(\\S+)-(\\d+).(\\d+).(\\d+)")
		match := re.FindStringSubmatch(tag.Name)
		if match != nil {
			//fmt.Println(len(match), match)
			if match[1] == module {
				major, err := strconv.Atoi(match[2])
				requireNoError(err, "strconv")
				minor, err := strconv.Atoi(match[3])
				requireNoError(err, "strconv")
				patch, err := strconv.Atoi(match[4])
				requireNoError(err, "strconv")
				return &version{major: major, minor: minor, patch: patch, tag: tag}
			}
		}
		return nil
	}
}

func extractDotnetVersion() func(tag Tag) *version {
	return func(tag Tag) *version {
		re := regexp.MustCompile("v(\\d+).(\\d+).(\\d+)-pyroscope")
		match := re.FindStringSubmatch(tag.Name)
		if match != nil {
			fmt.Println(len(match), match)

			major, err := strconv.Atoi(match[1])
			requireNoError(err, "strconv")
			minor, err := strconv.Atoi(match[2])
			requireNoError(err, "strconv")
			patch, err := strconv.Atoi(match[3])
			requireNoError(err, "strconv")
			return &version{major: major, minor: minor, patch: patch, tag: tag}

		}
		return nil
	}
}

func getTagsV(repo string, extractVersion func(Tag) *version) []version {
	tags := getTags(repo)
	fmt.Println(tags)
	versions := []version{}

	for _, tag := range tags {
		v := extractVersion(tag)
		if v != nil {
			versions = append(versions, *v)
		}
	}
	slices.SortFunc(versions, compareVersion)
	return versions
}

type version struct {
	major int
	minor int
	patch int

	tag Tag
}

func (v *version) versionV() string {
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
}

func (v *version) version() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

func compareVersion(a, b version) int {
	cmp := func(a, b int) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}

	if c := cmp(a.major, b.major); c != 0 {
		return c
	}
	if c := cmp(a.minor, b.minor); c != 0 {
		return c
	}
	if c := cmp(a.patch, b.patch); c != 0 {
		return c
	}

	return 0
}

func getTags(repo string) []Tag {
	// get "https://api.github.com/repos/<owner>/<repo>/tags"
	var res []Tag
	page := 1
	for {
		var pageTags []Tag
		url := "https://api.github.com/repos/" + repo + "/tags?page=" + strconv.Itoa(page) + "&per_page=100"
		log.Printf("GET %s", url)
		req, err := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(ghToken))
		requireNoError(err, "new request")
		resp, err := http.DefaultClient.Do(req)
		requireNoError(err, "do request")
		if resp.StatusCode != 200 {
			log.Fatalf("status code %d", resp.StatusCode)
		}
		defer resp.Body.Close()
		err = json.NewDecoder(resp.Body).Decode(&pageTags)
		requireNoError(err, "decoding json")
		res = append(res, pageTags...)
		if len(pageTags) == 0 {
			break
		}
		page++
	}
	return res
}

type Tag struct {
	Name       string `json:"name"`
	ZipballUrl string `json:"zipball_url"`
	TarballUrl string `json:"tarball_url"`
	Commit     struct {
		Sha string `json:"sha"`
		Url string `json:"url"`
	} `json:"commit"`
	NodeId string `json:"node_id"`
}

type sh struct {
	wd string
}

func (s *sh) sh(sh string) (string, string) {
	return s.cmd("/bin/sh", "-c", sh)
}

func (s *sh) cmd(cmdArgs ...string) (string, string) {
	log.Printf("cmd %s\n", strings.Join(cmdArgs, " "))
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Dir = s.wd
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	fmt.Println(stdout.String())
	fmt.Println(stderr.String())
	requireNoError(err, strings.Join(cmdArgs, " "))
	return stdout.String(), stderr.String()
}

func requireNoError(err error, msg string) {
	if err != nil {
		log.Fatalf("msg %s err %v", msg, err)
	}
}
