package runner

import (
	"fmt"
	"strings"

	"github.com/microsoft/storm/pkg/storm/core"
)

type runnerError struct {
	err             error
	metadata        core.TestRegistrantMetadata
	collectedOutput []string
}

func (be *runnerError) Error() string {
	return fmt.Sprintf(
		"error in %s '%s': %v",
		be.metadata.RegistrantType().String(),
		be.metadata.Name(),
		be.err,
	)
}

// outputSection renders the output collected while the failing hook ran as an
// indented, labelled block, or an empty string if no output was collected.
// Setup/Cleanup hooks run outside the test report, so without this their
// stdout/stderr/logrus output would be hidden in the default (non-watch) run
// mode; surfacing it in the error makes hook failures diagnosable.
func (be *runnerError) outputSection() string {
	if len(be.collectedOutput) == 0 {
		return ""
	}
	return fmt.Sprintf("\n---- collected output ----\n%s\n--------------------------",
		strings.Join(be.collectedOutput, "\n"))
}

type setupError struct {
	runnerError
}

func newSetupError(metadata core.TestRegistrantMetadata, err error, collectedOutput []string) *setupError {
	return &setupError{
		runnerError: runnerError{
			err:             err,
			metadata:        metadata,
			collectedOutput: collectedOutput,
		},
	}
}

func (se *setupError) Error() string {
	return fmt.Sprintf(
		"setup error in %s '%s': %v%s",
		se.metadata.RegistrantType().String(),
		se.metadata.Name(),
		se.err,
		se.outputSection(),
	)
}

type cleanupError struct {
	runnerError
}

func newCleanupError(metadata core.TestRegistrantMetadata, err error, collectedOutput []string) error {
	return &cleanupError{
		runnerError: runnerError{
			err:             err,
			metadata:        metadata,
			collectedOutput: collectedOutput,
		},
	}
}

func (se *cleanupError) Error() string {
	return fmt.Sprintf(
		"cleanup error in %s '%s': %v%s",
		se.metadata.RegistrantType().String(),
		se.metadata.Name(),
		se.err,
		se.outputSection(),
	)
}
