package devops

import (
	"io"
	"os"
)

// realStdOut is the original os.Stdout before any redirection.
var realStdOut io.Writer = os.Stdout
