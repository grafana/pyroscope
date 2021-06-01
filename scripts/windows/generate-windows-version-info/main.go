package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/josephspurrier/goversioninfo"
)

func main() {
	var version string
	flag.StringVar(&version, "version", "", "Version in semver format.")

	var outputPath string
	flag.StringVar(&outputPath, "out", "", "Output file path.")

	var iconPath string
	flag.StringVar(&iconPath, "icon", "", "Icon file path.")
	flag.Parse()

	if version == "" {
		fatalf("version is required")
	}
	version = strings.Trim(version, `"`)
	v, err := semver.Parse(strings.TrimPrefix(version, "v"))
	if err != nil {
		fatalf("invalid version %q: %v", version, err)
	}

	if outputPath == "" {
		fatalf("output path is required")
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
			FileDescription:  "Pyroscope continuous profiling platform agent",
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
		IconPath:     iconPath,
		ManifestPath: "",
	}

	versionInfo.Build()
	versionInfo.Walk()

	if err = versionInfo.WriteSyso(outputPath, "amd64"); err != nil {
		fatalf("failed to write output file %s: %v", outputPath, err)
	}
}

func fatalf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
