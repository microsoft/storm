package devops

import "os"

// realStdOut is the original os.Stdout before any redirection.
var realStdOut = os.Stdout
