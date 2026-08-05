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

// SetStdout overrides the writer returned by Stdout and used for all Azure
// DevOps logging commands, returning a function that restores the previous
// writer. It is intended for tests that need to capture Azure DevOps output.
func SetStdout(w io.Writer) (restore func()) {
	prev := realStdOut
	realStdOut = w
	return func() { realStdOut = prev }
}
