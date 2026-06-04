package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var ghToken string
var all = flag.Bool("all", true, "")
var golang = flag.Bool("go", false, "")
var java = flag.Bool("java", false, "")
var ruby = flag.Bool("ruby", false, "")
var python = flag.Bool("python", false, "")
var dotnet = flag.Bool("dotnet", false, "")
var node = flag.Bool("node", false, "")
var rust = flag.Bool("rust", false, "")
var tempo = flag.Bool("tempo", false, "")

// this program requires ruby, bundle, yarn, go to be installed
func main() {

	getGHToken()
	flag.Parse()
	if *all {
		*golang = true
		*java = true
		*ruby = true
		*python = true
		*dotnet = true
		*node = true
		*rust = true
		*tempo = true
	}

	if *golang {
		updateGolang()
		updateGodeltaprof()
		updateJfrParser()
		s.sh("make go/mod")
	}

	if *java {
		updateJava()
		updateOtelProfilingJava()
	}
	if *ruby {
		updateRuby()
	}
	if *python {
		updatePython()
	}
	if *dotnet {
		updateDotnet()
	}
	if *node {
		updateNodeJS()
	}
	if *rust {
		updateRust()
	}
	if *tempo {
		updateTempo()
	}
}

func getGHToken() {
	ghToken = os.Getenv("GITHUB_TOKEN")
}

func extractNodeJSVersion(tag Tag) *version {
	re := regexp.MustCompile(`(v)(\d+).(\d+).(\d+)`)
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
	for _, x := range []string{"express", "express-pull", "express-ts", "express-ts-inline", "tinyhttp"} {
		path := filepath.Join("examples/language-sdk-instrumentation/nodejs", x)
		replaceInplace(rePackageJson, filepath.Join(path, "package.json"), replPackageJson)
		s.sh(fmt.Sprintf(`cd "%s"       && yarn`, path))
	}
}

func updateRust() {
	libTags := getTagsV("grafana/pyroscope-rs", extractRSVersion("lib"))
	lastLibTag := libTags[len(libTags)-1]
	fmt.Println(lastLibTag)
	s.sh(fmt.Sprintf("cd examples/language-sdk-instrumentation/rust/rideshare/server && cargo add pyroscope@^%s", lastLibTag.version()))
	s.sh(fmt.Sprintf("cd examples/language-sdk-instrumentation/rust/basic && cargo add pyroscope@^%s", lastLibTag.version()))
}

func updateDotnet() {
	tags := getTagsV("grafana/pyroscope-dotnet", extractDotnetComponentVersion("pyroscope"))
	last := tags[len(tags)-1]
	fmt.Println(last)

	reArg := regexp.MustCompile(`ARG PROFILER_VERSION=\d+\.\d+\.\d+`)
	replArg := fmt.Sprintf("ARG PROFILER_VERSION=%s", last.version())
	for _, f := range []string{
		"examples/tracing/dotnet/Dockerfile",
		"examples/language-sdk-instrumentation/dotnet/fast-slow/Dockerfile",
		"examples/language-sdk-instrumentation/dotnet/fast-slow/musl.Dockerfile",
		"examples/language-sdk-instrumentation/dotnet/rideshare/Dockerfile",
		"examples/language-sdk-instrumentation/dotnet/rideshare/musl.Dockerfile",
		"examples/language-sdk-instrumentation/dotnet/web-new/Dockerfile",
	} {
		replaceInplace(reArg, f, replArg)
	}

	reUrl := regexp.MustCompile(`https://github\.com/grafana/pyroscope-dotnet/releases/download/(?:v\d+\.\d+\.\d+-pyroscope|pyroscope-\d+\.\d+\.\d+)/pyroscope\.\d+\.\d+\.\d+-glibc-x86_64\.tar\.gz`)
	replUrl := fmt.Sprintf("https://github.com/grafana/pyroscope-dotnet/releases/download/pyroscope-%s/pyroscope.%s-glibc-x86_64.tar.gz", last.version(), last.version())
	replaceInplace(reUrl, "docs/sources/configure-client/language-sdks/dotnet.md", replUrl)

	otelTags := getTagsV("grafana/pyroscope-dotnet", extractDotnetComponentVersion("opentelemetry"))
	lastOtel := otelTags[len(otelTags)-1]
	fmt.Println("Pyroscope.OpenTelemetry", lastOtel)
	reOtelPkg := regexp.MustCompile(`<PackageReference Include="Pyroscope.OpenTelemetry" Version="\d+\.\d+\.\d+" />`)
	replOtelPkg := fmt.Sprintf(`<PackageReference Include="Pyroscope.OpenTelemetry" Version="%s" />`, lastOtel.version())
	replaceInplace(reOtelPkg, "examples/tracing/dotnet/example/Example.csproj", replOtelPkg)
}

func updateTempo() {
	tags := getTagsV("grafana/tempo", extractTempoVersion(2))
	last := tags[len(tags)-1]
	fmt.Println("tempo", last)

	reDockerTempo := regexp.MustCompile(`grafana/tempo:\d+\.\d+\.\d+`)
	replDockerTempo := fmt.Sprintf("grafana/tempo:%s", last.version())
	for _, f := range []string{
		"examples/tracing/dotnet/docker-compose.yml",
		"examples/tracing/golang-push/docker-compose.yml",
		"examples/tracing/java/docker-compose.yml",
		"examples/tracing/java-wall/docker-compose.yml",
		"examples/tracing/python/docker-compose.yaml",
		"examples/tracing/ruby/docker-compose.yml",
		"examples/tracing/tempo/docker-compose.yml",
		"tools/tracing/docker-compose.yml",
	} {
		replaceInplace(reDockerTempo, f, replDockerTempo)
	}
}

// extractTempoVersion returns a version extractor that only matches tags for the
// given major version (e.g. 2 for "2.x.y").
func extractTempoVersion(major int) func(tag Tag) *version {
	return func(tag Tag) *version {
		re := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
		match := re.FindStringSubmatch(tag.Name)
		if match == nil {
			return nil
		}
		maj, err := strconv.Atoi(match[1])
		requireNoError(err, "strconv")
		if maj != major {
			return nil
		}
		minor, err := strconv.Atoi(match[2])
		requireNoError(err, "strconv")
		patch, err := strconv.Atoi(match[3])
		requireNoError(err, "strconv")
		return &version{major: maj, minor: minor, patch: patch, tag: tag}
	}
}

func updatePython() {
	tags := getTagsV("grafana/pyroscope-python", extractRSVersion("python"))
	last := tags[len(tags)-1]
	fmt.Println(last)

	re := regexp.MustCompile(`pyroscope-io==\d+\.\d+\.\d+`)
	repl := fmt.Sprintf("pyroscope-io==%s", last.version())
	replaceInplace(re, "examples/language-sdk-instrumentation/python/simple/requirements.txt", repl)
	replaceInplace(re, "examples/language-sdk-instrumentation/python/rideshare/flask/requirements.txt", repl)
	replaceInplace(re, "examples/language-sdk-instrumentation/python/rideshare/fastapi/requirements.txt", repl)
	replaceInplace(re, "examples/language-sdk-instrumentation/python/rideshare/django/app/requirements.txt", repl)

}

func updateRuby() {
	tags := getTagsV("grafana/pyroscope-ruby", extractRSVersion("ruby"))
	last := tags[len(tags)-1]
	fmt.Println(last)

	re := regexp.MustCompile(`gem ['"]pyroscope['"].*`)
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
	reJarURL := regexp.MustCompile(`https://github\.com/grafana/pyroscope-java/releases/download/(v\d+\.\d+\.\d+)/pyroscope\.jar`)
	lastJarURL := "https://github.com/grafana/pyroscope-java/releases/download/" + last.versionV() + "/pyroscope.jar"
	replaceInplace(reJarURL, "examples/language-sdk-instrumentation/java/fib/Dockerfile", lastJarURL)
	replaceInplace(reJarURL, "examples/language-sdk-instrumentation/java/simple/Dockerfile", lastJarURL)
	replaceInplace(reJarURL, "examples/tracing/java/Dockerfile", lastJarURL)
	replaceInplace(reJarURL, "examples/language-sdk-instrumentation/java/rideshare/Dockerfile", lastJarURL)

	reGradelDep := regexp.MustCompile(`implementation\("io\.pyroscope:agent:\d+\.\d+\.\d+"\)`)
	lastGradleDep := fmt.Sprintf("implementation(\"io.pyroscope:agent:%s\")", last.version())
	replaceInplace(reGradelDep, "examples/language-sdk-instrumentation/java/rideshare/build.gradle.kts", lastGradleDep)
	replaceInplace(reGradelDep, "examples/tracing/java/build.gradle.kts", lastGradleDep)
	replaceInplace(reGradelDep, "docs/sources/configure-client/language-sdks/java.md", lastGradleDep)

	reMaven := regexp.MustCompile(`<version>\d+\.\d+\.\d+</version>`)
	replMaven := fmt.Sprintf("<version>%s</version>", last.version())
	replaceInplace(reMaven, "docs/sources/configure-client/language-sdks/java.md", replMaven)

}

func updateOtelProfilingJava() {
	tags := getTagsV("grafana/otel-profiling-java", extractGoVersion(""))
	last := tags[len(tags)-1]
	reJarURL := regexp.MustCompile(`https://github\.com/grafana/otel-profiling-java/releases/download/(v\d+\.\d+\.\d+)/pyroscope-otel-javaagent-extension\.jar`)
	lastJarURL := "https://github.com/grafana/otel-profiling-java/releases/download/" + last.versionV() + "/pyroscope-otel-javaagent-extension.jar"
	replaceInplace(reJarURL, "docs/sources/configure-client/trace-span-profiles/java-span-profiles.md", lastJarURL)
	replaceInplace(reJarURL, "examples/tracing/java/Dockerfile", lastJarURL)
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
		re := regexp.MustCompile(`([^/]*)/?v(\d+).(\d+).(\d+)`)
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
		re := regexp.MustCompile(`(\S+)-(\d+).(\d+).(\d+)`)
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

func updateJfrParser() {
	parserVersions := getTagsV("grafana/jfr-parser", extractGoVersion(""))
	parserVersion := parserVersions[len(parserVersions)-1]
	fmt.Printf("jfr-parser  %+v\n", parserVersion)

	s.sh(" go get github.com/grafana/jfr-parser@" + parserVersion.versionV())
}

// extractDotnetComponentVersion matches release tags for a pyroscope-dotnet
// component (e.g. "pyroscope" or "opentelemetry"), which are versioned
// independently of each other. The repo migrated to release-please, which tags
// releases with a component prefix and no leading "v" (e.g. "pyroscope-1.0.0",
// "opentelemetry-0.4.1"). Older releases used the opposite convention: a "v"
// prefix and a component suffix (e.g. "v0.15.0-pyroscope"). Both forms are
// matched so the script keeps working across the transition.
func extractDotnetComponentVersion(component string) func(tag Tag) *version {
	prefix := regexp.MustCompile(`^` + component + `-(\d+)\.(\d+)\.(\d+)$`)
	suffix := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)-` + component + `$`)
	return func(tag Tag) *version {
		match := prefix.FindStringSubmatch(tag.Name)
		if match == nil {
			match = suffix.FindStringSubmatch(tag.Name)
		}
		if match == nil {
			return nil
		}
		major, err := strconv.Atoi(match[1])
		requireNoError(err, "strconv")
		minor, err := strconv.Atoi(match[2])
		requireNoError(err, "strconv")
		patch, err := strconv.Atoi(match[3])
		requireNoError(err, "strconv")
		return &version{major: major, minor: minor, patch: patch, tag: tag}
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
