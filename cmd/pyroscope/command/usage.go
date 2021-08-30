package command

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	headerClr *color.Color
	itemClr   *color.Color
	descClr   *color.Color
	defClr    *color.Color
)

func init() {
	headerClr = color.New(color.FgGreen)
	itemClr = color.New(color.Bold)
	// itemClr = color.New()
	descClr = color.New()
	defClr = color.New(color.FgYellow)
}

// TODO: Do we want to keep this or use cobra default one? Maybe banner + cobra default? Or something else?
// This is mostly copied from ffcli package
func DefaultUsageFunc(sf *pflag.FlagSet, c *cobra.Command) string {
	var b strings.Builder

	fmt.Fprintf(&b, "continuous profiling platform\n\n")
	headerClr.Fprintf(&b, "USAGE\n")
	if c.Use != "" {
		fmt.Fprintf(&b, "  %s\n", c.Use)
	} else {
		fmt.Fprintf(&b, "  %s\n", c.Name())
	}
	fmt.Fprintf(&b, "\n")

	if c.Long != "" {
		fmt.Fprintf(&b, "%s\n\n", c.Long)
	}

	if c.HasSubCommands() {
		headerClr.Fprintf(&b, "SUBCOMMANDS\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		for _, subcommand := range c.Commands() {
			if !subcommand.Hidden {
				fmt.Fprintf(tw, "  %s\t%s\n", itemClr.Sprintf(subcommand.Name()), subcommand.Short)
			}
		}
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	if countFlags(c.Flags()) > 0 {
		// headerClr.Fprintf(&b, "FLAGS\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		fmt.Fprintf(tw, "%s\t  %s@new-line@\n", headerClr.Sprintf("FLAGS"), defClr.Sprint("DEFAULT VALUES"))

		// TODO: it would be nice to sort by how often people would use these.
		//   But for that we'd have to have a conversion from flag-set back to struct
		sf.VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			def := f.DefValue
			// if def == "" {
			// 	def = "..."
			// }
			def = defClr.Sprint(def)
			// def = fmt.Sprintf("(%s)", def)
			fmt.Fprintf(tw, "  %s\t%s", itemClr.Sprintf("--"+f.Name), def)
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

	if c.HasSubCommands() {
		b.WriteString("Run 'pyroscope SUBCOMMAND --help' for more information on a subcommand.\n")
	}

	return strings.ReplaceAll(b.String(), "@new-line@", "\n")
}

func countFlags(fs *pflag.FlagSet) (n int) {
	fs.VisitAll(func(*pflag.Flag) { n++ })
	return n
}
