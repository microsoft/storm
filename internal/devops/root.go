package devops

import (
	"io"
	"os"
)

// realStdOut is the original os.Stdout before any redirection.
var realStdOut io.Writer = os.Stdout

// Stdout returns the writer bound to the process's original standard output,
// captured before any redirection. Azure DevOps parses logging commands from
// both stdout and stderr on separate threads and orders lines by arrival, so
// commands and any output that must stay strictly ordered relative to them
// (e.g. ##[group]/##[endgroup] markers and the lines they bracket) should all
// be written here, to a single shared stream, to avoid stdout/stderr
// interleaving races in the agent's rendered log.
func Stdout() io.Writer {
	return realStdOut
}
