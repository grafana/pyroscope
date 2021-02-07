package cli

import (
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/pyroscope-io/pyroscope/pkg/util/strarr"
)

var headerClr *color.Color
var itemClr *color.Color
var descClr *color.Color
var defClr *color.Color

func init() {
	headerClr = color.New(color.FgGreen)
	itemClr = color.New(color.Bold)
	// itemClr = color.New()
	descClr = color.New()
	defClr = color.New(color.FgYellow)
}

// disabled these commands for now, they are not documented and confuse people
var hiddenCommands = []string{
	"agent",
	"convert",
	"dbmanager",
}

// This is mostly copied from ffcli package
func DefaultUsageFunc(c *ffcli.Command) string {
	var b strings.Builder

	fmt.Fprintf(&b, "continuous profiling platform\n\n")
	headerClr.Fprintf(&b, "USAGE\n")
	if c.ShortUsage != "" {
		fmt.Fprintf(&b, "  %s\n", c.ShortUsage)
	} else {
		fmt.Fprintf(&b, "  %s\n", c.Name)
	}
	fmt.Fprintf(&b, "\n")

	if c.LongHelp != "" {
		fmt.Fprintf(&b, "%s\n\n", c.LongHelp)
	}

	if len(c.Subcommands) > 0 {
		headerClr.Fprintf(&b, "SUBCOMMANDS\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		for _, subcommand := range c.Subcommands {
			if !strarr.Contains(hiddenCommands, subcommand.Name) {
				fmt.Fprintf(tw, "  %s\t%s\n", itemClr.Sprintf(subcommand.Name), subcommand.ShortHelp)
			}
		}
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	if countFlags(c.FlagSet) > 0 {
		// headerClr.Fprintf(&b, "FLAGS\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		fmt.Fprintf(tw, "%s\t  %s@new-line@\n", headerClr.Sprintf("FLAGS"), defClr.Sprint("DEFAULT VALUES"))

		// TODO: it would be nice to sort by how often people would use these.
		//   But for that we'd have to have a conversion from flag-set back to struct
		c.FlagSet.VisitAll(func(f *flag.Flag) {
			def := f.DefValue
			// if def == "" {
			// 	def = "..."
			// }
			def = defClr.Sprint(def)
			// def = fmt.Sprintf("(%s)", def)
			fmt.Fprintf(tw, "  %s\t%s", itemClr.Sprintf("-"+f.Name), def)
			if f.Usage != "" {
				fmt.Fprintf(tw, "@new-line@    ")
				descClr.Fprint(tw, f.Usage)
			}
			descClr.Fprint(tw, "@new-line@")
			fmt.Fprint(tw, "\n")
		})
		tw.Flush()
		// fmt.Fprintf(&b, "\n")
	}

	if len(c.Subcommands) > 0 {
		b.WriteString("Run 'pyroscope SUBCOMMAND --help' for more information on a subcommand.\n")
	}

	return strings.ReplaceAll(b.String(), "@new-line@", "\n")
}

func countFlags(fs *flag.FlagSet) (n int) {
	fs.VisitAll(func(*flag.Flag) { n++ })
	return n
}
