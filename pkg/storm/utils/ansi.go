package utils

import (
	"regexp"
)

var (
	// ANSI escape code cleaner
	ANSI_CLEANER = regexp.MustCompile(`(\x9B|\x1B\[)[0-?]*[ -\/]*[@-~]`)

	// ANSI non-color escape code cleaner, matches only control codes
	ANSI_CONTROL_CLEANER = regexp.MustCompile(`(\x9B|\x1B\[)[0-?]*[ -\/]*[@-ln-~]`)
)

func RemoveAllANSI(input string) string {
	return ANSI_CLEANER.ReplaceAllString(input, "")
}

func RemoveControlANSI(input string) string {
	return ANSI_CONTROL_CLEANER.ReplaceAllString(input, "")
}
