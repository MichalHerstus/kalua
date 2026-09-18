package cli

import (
	"fmt"
	"os"

	"kalua/internal/format"
	"kalua/internal/host"
)

// formatMode is the formatting operation selected by the check flags.
type formatMode int

const (
	formatNone formatMode = iota
	formatStdout
	formatWrite
	formatList
	formatDiff
)

// runFormat applies gofmt-style formatting to the given scripts under the
// selected mode and returns the process exit code.
//
//	formatStdout: print formatted source, exit 0 even when changed
//	formatWrite:  write in place (permissions preserved), exit 0
//	formatList:   print the names of scripts whose formatting differs, exit 1
//	formatDiff:   print a 0-context unified diff, exit 1
//
// Any formatting error (unparseable input, formatter bug) is reported to
// stderr, nothing is written, and the exit code is ExitError.
func runFormat(fm formatMode, scripts []string, jsonOut bool) int {
	status := int(host.ExitOK)
	fr := formatResult{OK: true, Files: len(scripts)}
	for _, s := range scripts {
		src, err := os.ReadFile(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return int(host.ExitError)
		}
		formatted, err := format.Format(src, s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", s, err)
			return int(host.ExitError)
		}
		dirty := string(formatted) != string(src)
		switch fm {
		case formatWrite:
			if !dirty {
				continue
			}
			info, statErr := os.Stat(s)
			mode := os.FileMode(0o644)
			if statErr == nil {
				mode = info.Mode()
			}
			if err := os.WriteFile(s, formatted, mode); err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", s, err)
				return int(host.ExitError)
			}
			fr.Changed = append(fr.Changed, s)
			status = int(host.ExitOK)
		case formatList:
			if dirty {
				if jsonOut {
					fr.Changed = append(fr.Changed, s)
				} else {
					fmt.Println(s)
				}
				fr.OK = false
				status = int(host.ExitError)
			}
		case formatDiff:
			if dirty {
				if jsonOut {
					fr.Changed = append(fr.Changed, s+"\n"+format.Diff(s, string(src), string(formatted)))
				} else {
					fmt.Print(format.Diff(s, string(src), string(formatted)))
				}
				fr.OK = false
				status = int(host.ExitError)
			}
		default: // formatStdout
			os.Stdout.Write(formatted)
		}
	}
	if jsonOut && fm != formatStdout {
		writeJSON(fr)
	}
	return status
}
