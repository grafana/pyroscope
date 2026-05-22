// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/tools/doc-generator/writer.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/grafana/regexp"
	"github.com/mitchellh/go-wordwrap"
	"gopkg.in/yaml.v3"

	"github.com/grafana/pyroscope/v2/tools/doc-generator/parse"
)

type specWriter struct {
	out strings.Builder
}

func (w *specWriter) writeConfigBlock(b *parse.ConfigBlock, indent int) {
	if len(b.Entries) == 0 {
		return
	}

	for i, entry := range b.Entries {
		// Add a new line to separate from the previous entry
		if i > 0 {
			w.out.WriteString("\n")
		}

		w.writeConfigEntry(entry, indent)
	}
}

func (w *specWriter) writeConfigEntry(e *parse.ConfigEntry, indent int) {
	if e.Kind == parse.KindBlock {
		// If the block is a root block it will have its dedicated section in the doc,
		// so here we've just to write down the reference without re-iterating on it.
		if e.Root {
			// Description
			w.writeComment(e.BlockDesc, indent, 0)
			if e.Block.FlagsPrefix != "" {
				w.writeComment(fmt.Sprintf("The CLI flags prefix for this block configuration is: %s", e.Block.FlagsPrefix), indent, 0)
			}

			// Block reference without entries, because it's a root block
			w.out.WriteString(pad(indent) + "[" + e.Name + ": <" + e.Block.Name + ">]\n")
		} else {
			// Description
			w.writeComment(e.BlockDesc, indent, 0)

			// Name
			w.out.WriteString(pad(indent) + e.Name + ":\n")

			// Entries
			w.writeConfigBlock(e.Block, indent+tabWidth)
		}
	}

	if e.Kind == parse.KindField || e.Kind == parse.KindSlice || e.Kind == parse.KindMap {
		// Description
		w.writeComment(e.Description(), indent, 0)
		w.writeExample(e.FieldExample, indent)
		w.writeFlag(e.FieldFlag, indent)

		// Specification
		fieldDefault := e.FieldDefault
		switch e.FieldType {
		case "string":
			fieldDefault = strconv.Quote(fieldDefault)
		case "duration":
			fieldDefault = cleanupDuration(fieldDefault)
		}

		if e.Required {
			w.out.WriteString(pad(indent) + e.Name + ": <" + e.FieldType + "> | default = " + fieldDefault + "\n")
		} else {
			w.out.WriteString(pad(indent) + "[" + e.Name + ": <" + e.FieldType + "> | default = " + fieldDefault + "]\n")
		}
	}
}

func (w *specWriter) writeFlag(name string, indent int) {
	if name == "" {
		return
	}

	w.out.WriteString(pad(indent) + "# CLI flag: -" + name + "\n")
}

func (w *specWriter) writeComment(comment string, indent, innerIndent int) {
	if comment == "" {
		return
	}

	wrapped := wordwrap.WrapString(comment, uint(maxLineWidth-indent-innerIndent-2))
	w.writeWrappedString(wrapped, indent, innerIndent)
}

func (w *specWriter) writeExample(example *parse.FieldExample, indent int) {
	if example == nil {
		return
	}

	w.writeComment("Example:", indent, 0)
	if example.Comment != "" {
		w.writeComment(example.Comment, indent, 2)
	}

	data, err := yaml.Marshal(example.Yaml)
	if err != nil {
		panic(fmt.Errorf("can't render example: %w", err))
	}

	w.writeWrappedString(string(data), indent, 2)
}

func (w *specWriter) writeWrappedString(s string, indent, innerIndent int) {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for _, line := range lines {
		w.out.WriteString(pad(indent) + "# " + pad(innerIndent) + line + "\n")
	}
}

func (w *specWriter) string() string {
	return strings.TrimSpace(w.out.String())
}

type markdownWriter struct {
	out strings.Builder
}

func (w *markdownWriter) writeConfigDoc(blocks []*parse.ConfigBlock) {
	// Deduplicate root blocks.
	uniqueBlocks := map[string]*parse.ConfigBlock{}
	for _, block := range blocks {
		uniqueBlocks[block.Name] = block
	}

	// Generate the markdown, honoring the root blocks order.
	if topBlock, ok := uniqueBlocks[""]; ok {
		w.writeConfigBlock(topBlock)
	}

	for _, rootBlock := range parse.RootBlocks {
		if block, ok := uniqueBlocks[rootBlock.Name]; ok {
			// Keep the root block description.
			blockToWrite := *block
			blockToWrite.Desc = rootBlock.Desc

			w.writeConfigBlock(&blockToWrite)
		}
	}
}

func (w *markdownWriter) writeConfigBlock(block *parse.ConfigBlock) {
	// Title
	if block.Name != "" {
		w.out.WriteString("### " + block.Name + "\n")
		w.out.WriteString("\n")
	}

	// Description
	if block.Desc != "" {
		desc := block.Desc

		// Wrap first instance of the config block name with backticks
		if block.Name != "" {
			var matches int
			nameRegexp := regexp.MustCompile(regexp.QuoteMeta(block.Name))
			desc = nameRegexp.ReplaceAllStringFunc(desc, func(input string) string {
				if matches == 0 {
					matches++
					return "`" + input + "`"
				}
				return input
			})
		}

		// List of all prefixes used to reference this config block.
		if len(block.FlagsPrefixes) > 1 {
			sortedPrefixes := sort.StringSlice(block.FlagsPrefixes)
			sortedPrefixes.Sort()

			desc += " The supported CLI flags `<prefix>` used to reference this configuration block are:\n\n"

			for _, prefix := range sortedPrefixes {
				if prefix == "" {
					desc += "- _no prefix_\n"
				} else {
					desc += fmt.Sprintf("- `%s`\n", prefix)
				}
			}

			// Unfortunately the markdown compiler used by the website generator has a bug
			// when there's a list followed by a code block (no matter know many newlines
			// in between). To workaround it we add a non-breaking space.
			desc += "\n&nbsp;"
		}

		w.out.WriteString(desc + "\n")
		w.out.WriteString("\n")
	}

	// Config specs
	spec := &specWriter{}
	spec.writeConfigBlock(block, 0)

	w.out.WriteString("```yaml\n")
	w.out.WriteString(spec.string() + "\n")
	w.out.WriteString("```\n")
	w.out.WriteString("\n")
}

func (w *markdownWriter) string() string {
	return strings.TrimSpace(w.out.String())
}

func pad(length int) string {
	return strings.Repeat(" ", length)
}

type yamlExampleWriter struct {
	out        strings.Builder
	skipBlocks map[string]bool
}

func (w *yamlExampleWriter) writeConfigYAML(blocks []*parse.ConfigBlock) {
	var topBlock *parse.ConfigBlock
	for _, b := range blocks {
		if b.Name == "" {
			topBlock = b
			break
		}
	}
	if topBlock == nil {
		return
	}

	w.out.WriteString("# Pyroscope example configuration file.\n")
	w.out.WriteString("#\n")
	w.out.WriteString("# All fields are shown with their default values, commented out.\n")
	w.out.WriteString("# Uncomment and modify the ones you want to override.\n")
	w.out.WriteString("# For the full reference see:\n")
	w.out.WriteString("#   https://grafana.com/docs/pyroscope/latest/configure-server/reference-configuration-parameters/\n")
	w.out.WriteString("\n")

	w.writeBlock(topBlock, 0)
}

func (w *yamlExampleWriter) hasBasicContent(block *parse.ConfigBlock) bool {
	for _, entry := range block.Entries {
		switch entry.Kind {
		case parse.KindBlock:
			if w.hasBasicContent(entry.Block) {
				return true
			}
		default:
			if entry.FieldCategory == "" || entry.FieldCategory == "basic" {
				return true
			}
		}
	}
	return false
}

func (w *yamlExampleWriter) writeBlock(block *parse.ConfigBlock, indent int) {
	first := true
	for _, entry := range block.Entries {
		switch entry.Kind {
		case parse.KindBlock:
			if w.skipBlocks[entry.Name] {
				continue
			}
			if !w.hasBasicContent(entry.Block) {
				continue
			}
			if !first {
				w.out.WriteString("\n")
			}
			first = false
			w.out.WriteString(pad(indent) + entry.Name + ":\n")
			w.writeBlock(entry.Block, indent+tabWidth)

		case parse.KindField, parse.KindSlice, parse.KindMap:
			if entry.FieldCategory != "" && entry.FieldCategory != "basic" {
				continue
			}
			if !first {
				w.out.WriteString("\n")
			}
			first = false
			if entry.FieldDesc != "" {
				w.writeDescComment(entry.FieldDesc, indent)
			}
			w.out.WriteString(fmt.Sprintf("%s# %s: %s\n", pad(indent), entry.Name, yamlDefaultValue(entry)))
		}
	}
}

func (w *yamlExampleWriter) writeDescComment(desc string, indent int) {
	wrapped := wordwrap.WrapString(desc, uint(maxLineWidth-indent-2))
	for _, line := range strings.Split(strings.TrimSpace(wrapped), "\n") {
		w.out.WriteString(pad(indent) + "# " + line + "\n")
	}
}

func (w *yamlExampleWriter) string() string {
	return strings.TrimSpace(w.out.String()) + "\n"
}

func yamlDefaultValue(e *parse.ConfigEntry) string {
	def := e.FieldDefault
	switch e.FieldType {
	case "string", "url":
		return strconv.Quote(def)
	case "duration":
		cleaned := cleanupDuration(def)
		if cleaned == "" {
			cleaned = "0s"
		}
		return cleaned
	default:
		if def == "" {
			return "0"
		}
		return def
	}
}

func cleanupDuration(value string) string {
	// This is the list of suffixes to remove from the duration if they're not
	// the whole duration value.
	suffixes := []string{"0s", "0m"}

	for _, suffix := range suffixes {
		re := regexp.MustCompile("(^.+\\D)" + suffix + "$")

		if groups := re.FindStringSubmatch(value); len(groups) == 2 {
			value = groups[1]
		}
	}

	return value
}
