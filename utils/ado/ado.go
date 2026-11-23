// Package ado emits Azure DevOps pipeline values.
//
// Matrix selection is the caller's concern; this package only validates and
// encodes a matrix and writes logging commands.
package ado

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var (
	// Job-matrix leg names become job identifiers: they start with a letter and
	// are at most 100 characters.
	legName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,99}$`)
	// Variable names are used both as properties and as environment variable names.
	varName = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)
)

// Azure DevOps reserves these (case-insensitive) variable name prefixes for its
// own use, for example for secrets.
var reservedPrefixes = []string{"secret", "input", "endpoint", "path", "securefile"}

func checkVarName(name string) error {
	if !varName.MatchString(name) {
		return fmt.Errorf("invalid variable name %q: want [A-Za-z0-9_.]+", name)
	}
	lower := strings.ToLower(name)
	for _, p := range reservedPrefixes {
		if strings.HasPrefix(lower, p) {
			return fmt.Errorf("variable name %q uses the reserved prefix %q", name, p)
		}
	}
	return nil
}

// MarshalMatrix encodes entries as a job-matrix value, {"<leg>": {"<var>": "<value>"}}.
// Leg names use ASCII letters, digits or underscores, start with a letter, and
// contain at most 100 characters. Variables follow SetVariable's naming rules.
// Legs and variables are emitted in sorted order, so output is stable.
// Nil maps encode as empty objects; the caller's maps are not modified.
func MarshalMatrix(entries map[string]map[string]string) ([]byte, error) {
	matrix := make(map[string]map[string]string, len(entries))
	for leg, vars := range entries {
		if !legName.MatchString(leg) {
			return nil, fmt.Errorf("invalid matrix leg name %q: want a letter followed by up to 99 letters, digits or underscores", leg)
		}
		for name := range vars {
			if err := checkVarName(name); err != nil {
				return nil, fmt.Errorf("leg %q: %w", leg, err)
			}
		}
		if vars == nil {
			vars = map[string]string{}
		}
		matrix[leg] = vars
	}
	// encoding/json sorts map keys.
	return json.Marshal(matrix)
}

// SetVariable writes one newline-terminated task.setvariable command to w.
// Names must be nonempty and use only ASCII letters, digits, underscores or dots.
// The prefixes secret, input, endpoint, path and securefile are reserved
// (case-insensitive). Values are escaped, not masked as secrets. isOutput
// enables the task's output-variable scope.
// Validation and writer errors are returned.
//
// Use stdout outside captured test output; disable shell tracing in any wrapper.
func SetVariable(w io.Writer, name, value string, isOutput bool) error {
	if err := checkVarName(name); err != nil {
		return err
	}
	props := "variable=" + escapeProperty(name)
	if isOutput {
		props += ";isOutput=true"
	}
	_, err := io.WriteString(w, "##vso[task.setvariable "+props+"]"+escapeData(value)+"\n")
	return err
}

// escapeData escapes a logging-command message.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%AZP25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// escapeProperty escapes a logging-command property value.
func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%AZP25", "\r", "%0D", "\n", "%0A", ";", "%3B", "]", "%5D").Replace(s)
}
