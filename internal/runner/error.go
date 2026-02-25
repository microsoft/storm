package runner

import (
	"fmt"

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
		"setup error in %s '%s': %v",
		se.metadata.RegistrantType().String(),
		se.metadata.Name(),
		se.err,
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
		"cleanup error in %s '%s': %v",
		se.metadata.RegistrantType().String(),
		se.metadata.Name(),
		se.err,
	)
}
