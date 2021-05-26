package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/josephspurrier/goversioninfo"
)

const (
	defaultVersion    = "0.0.0"
	defaultVarName    = "GIT_TAG"
	defaultOutputFile = "resource.syso"
)

// This tool generates syso file which is required for windows build.
// In particular, version info is used in MSI build.

func main() {
	var version string
	flag.StringVar(&version, "version", defaultVersion, "Version in semver format.")

	var variable string
	flag.StringVar(&variable, "variable", defaultVarName, "Environment variable containing version.")

	var outputFile string
	flag.StringVar(&outputFile, "out", defaultOutputFile, "Output file name.")
	flag.Parse()

	// Variable specified with "variable" flag takes precedence over environment variable.
	vv, ok := os.LookupEnv(variable)
	if ok && version == defaultVersion {
		version = vv
	}

	v, err := semver.Parse(strings.TrimPrefix(version, "v"))
	if err != nil {
		fatalf("invalid version %q: %v", version, err)
	}

	versionInfo := goversioninfo.VersionInfo{
		FixedFileInfo: goversioninfo.FixedFileInfo{
			FileVersion: goversioninfo.FileVersion{
				Major: int(v.Major),
				Minor: int(v.Minor),
				Patch: int(v.Patch),
				Build: 0,
			},
			ProductVersion: goversioninfo.FileVersion{
				Major: int(v.Major),
				Minor: int(v.Minor),
				Patch: int(v.Patch),
				Build: 0,
			},
			FileFlagsMask: "3f",
			FileFlags:     "00",
			FileOS:        "040004",
			FileType:      "01",
			FileSubType:   "00",
		},
		StringFileInfo: goversioninfo.StringFileInfo{
			Comments:         "",
			CompanyName:      "Pyroscope, Inc",
			FileDescription:  "",
			FileVersion:      version,
			InternalName:     "pyroscope.exe",
			LegalCopyright:   "Copyright (c) 2021 Pyroscope, Inc",
			LegalTrademarks:  "",
			OriginalFilename: "",
			PrivateBuild:     "",
			ProductName:      "Pyroscope Agent",
			ProductVersion:   version,
			SpecialBuild:     "",
		},
		VarFileInfo: goversioninfo.VarFileInfo{
			Translation: goversioninfo.Translation{
				LangID:    goversioninfo.LngUSEnglish,
				CharsetID: goversioninfo.CsUnicode,
			},
		},
		IconPath:     "",
		ManifestPath: "",
	}

	versionInfo.Build()
	versionInfo.Walk()

	if err = versionInfo.WriteSyso(outputFile, "amd64"); err != nil {
		fatalf("failed to write output file %s: %v", outputFile, err)
	}
}

func fatalf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
