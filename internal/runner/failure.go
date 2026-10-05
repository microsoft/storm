package runner

import (
	"fmt"
	"io"
	"strings"

	"github.com/microsoft/storm/internal/artifacts"
	"github.com/microsoft/storm/internal/testmgr"
	stormartifacts "github.com/microsoft/storm/pkg/storm/artifacts"
	"github.com/microsoft/storm/pkg/storm/core"
)

// failedCase adapts a testmgr.TestCase to the core.FailedCase interface.
type failedCase struct {
	tc *testmgr.TestCase
}

func (f failedCase) Name() string   { return f.tc.Name() }
func (f failedCase) IsError() bool  { return f.tc.Status().Errored() }
func (f failedCase) Reason() string { return f.tc.Reason() }

// failureContext implements core.FailureContext.
type failureContext struct {
	core.LoggerProvider
	core.TestRegistrantMetadata
	failed        []core.FailedCase
	cleanupPaused bool
	broker        stormartifacts.ArtifactBroker
}

func (c *failureContext) FailedCases() []core.FailedCase {
	return c.failed
}

func (c *failureContext) CleanupPaused() bool {
	return c.cleanupPaused
}

func (c *failureContext) ArtifactBroker() stormartifacts.ArtifactBroker {
	return c.broker
}

// runOnFailureHook invokes the runnable's OnFailure hook when the run failed.
//
// It fires only when at least one case ended Failed or Errored (never for
// skips), is invoked via captureOutput + runCatchPanic (panic-safe,
// output-captured, and not subject to the per-test-case cleanup timeout), and
// never propagates an error: the originally failed case remains the reported
// cause and the exit code is unaffected.
func runOnFailureHook(
	suite core.SuiteContext,
	runnable *runnableInstance,
	testManager *testmgr.StormTestManager,
	artifactManager *artifacts.ArtifactManager,
	watch bool,
	pauseCleanup bool,
) {
	hook, ok := runnable.TestRegistrant.(core.OnFailure)
	if !ok {
		return
	}

	var failed []core.FailedCase
	var firstFailed *testmgr.TestCase
	for _, tc := range testManager.TestCases() {
		if tc.Status().IsBad() {
			failed = append(failed, failedCase{tc: tc})
			if firstFailed == nil {
				firstFailed = tc
			}
		}
	}

	// Nothing failed - the hook does not fire.
	if len(failed) == 0 {
		return
	}

	// Build a broker attached to the first failed case (so PublishLogFile lands
	// under that case's directory) but reporting publish errors through the
	// logger: the case is already closed, so calling its Error() would panic.
	broker := artifactManager.NewBroker()
	broker.AttachTestCase(firstFailed)
	broker.SetErrorReporter(func(err error) {
		suite.Logger().WithError(err).Error("OnFailure artifact publish failed")
	})

	fctx := &failureContext{
		LoggerProvider:         suite,
		TestRegistrantMetadata: runnable,
		failed:                 failed,
		cleanupPaused:          pauseCleanup,
		broker:                 broker,
	}

	suite.Logger().Info("Running OnFailure() Hook")

	var hookErr error
	captured, captureErr := captureOutput(func() {
		hookErr = runCatchPanic(func() error { return hook.OnFailure(fctx) })
	}, func(w io.Writer, s string) {
		if suite.AzureDevops() || watch {
			fmt.Fprintf(w, "  ├ %s\n", s)
		}
	})

	if captureErr != nil {
		suite.Logger().WithError(captureErr).Error("Failed to capture output for OnFailure()")
	}

	// Surface any hook error or recovered panic without propagating it: the
	// original failure stays the reported cause and the exit code is unchanged.
	if hookErr != nil {
		suite.Logger().WithError(hookErr).Error("OnFailure() hook failed (diagnostic capture only; original failure preserved)")
		if len(captured) > 0 {
			suite.Logger().Errorf("OnFailure() output:\n%s", strings.Join(captured, "\n"))
		}
	}
}
