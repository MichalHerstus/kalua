package cli

import (
	"flag"
	"fmt"
	"os"

	"kalua/internal/host"
	"kalua/internal/version"
)

// versionCmd prints build metadata stamped in at link time (see
// internal/version). Unstamped builds report "dev (unstamped, ...)" so a
// missing stamp is obvious.
func versionCmd(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var jsonOut = fs.Bool("json", false, "Emit the build info as JSON")
	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "version: unexpected argument %q\n", fs.Arg(0))
		return int(host.ExitUsage)
	}
	info := version.Get()
	if *jsonOut {
		return writeJSON(info)
	}
	fmt.Println(info.String())
	return int(host.ExitOK)
}
