package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"kalua/internal/checker"
	"kalua/internal/describe"
	"kalua/internal/host"
)

func describeCmd(args []string) int {
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var jsonOut = fs.Bool("json", false, "Emit the description as JSON")
	if err := fs.Parse(args); err != nil {
		return int(host.ExitUsage)
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "describe: requires a script argument")
		return int(host.ExitUsage)
	}
	if err := fs.Parse(rest[1:]); err != nil {
		return int(host.ExitUsage)
	}
	script := rest[0]

	src, err := os.ReadFile(script)
	if err != nil {
		fmt.Fprintf(os.Stderr, "describe: cannot read %s: %v\n", script, err)
		if os.IsNotExist(err) || os.IsPermission(err) {
			return int(host.ExitIOError)
		}
		return int(host.ExitError)
	}

	chk := checker.Check(string(src), script)
	res := describe.Result{
		OK:       len(chk.Errors) == 0,
		Entry:    checker.EntryMode(string(src), script),
		Lines:    len(strings.Split(string(src), "\n")),
		KUsage:   map[string]int{},
		Handlers: []string{},
	}
	if !res.OK {
		res.Error = strings.Join(chk.Errors[:1], "; ")
		if *jsonOut {
			writeJSON(res)
			return int(host.ExitError)
		}
		for _, iss := range chk.Issues {
			fmt.Fprintf(os.Stderr, "%s:%d:%d: %s\n", script, iss.Line, iss.Col, iss.Message)
		}
		return int(host.ExitError)
	}

	describe.Scan(string(src), script, &res)

	if !*jsonOut {
		describe.Print(res, script)
		return int(host.ExitOK)
	}
	writeJSON(res)
	return int(host.ExitOK)
}