package main

import (
	"errors"
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

	if err := generateVersionInfo(version, outputPath, iconPath); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generateVersionInfo(version, outputPath, iconPath string) error {
	version = strings.Trim(version, `"`)
	v, err := semver.Parse(strings.TrimPrefix(version, "v"))
  
	if version == "" {
		return errors.New("version is required")
	}

	if err != nil {
		return fmt.Errorf("invalid version %q: %w", version, err)
	}

	if outputPath == "" {
		return errors.New("output path is required")
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
		return fmt.Errorf("failed to write output file %s: %w", outputPath, err)
	}

	return nil
}
