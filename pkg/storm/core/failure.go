package core

import "github.com/microsoft/storm/pkg/storm/artifacts"

// FailedCase describes a single test case that ended in a failed or errored
// state. It is provided to OnFailure via FailureContext.
type FailedCase interface {
	Named

	// IsError reports whether the case ended via a test-infrastructure error
	// (tc.Error, a panic, or a returned error) rather than a product failure
	// (tc.Fail / tc.FailFromError).
	IsError() bool

	// Reason is the recorded failure reason or error message for the case.
	Reason() string
}

// FailureContext is provided to the OnFailure hook. It exposes everything a
// runnable needs to capture diagnostics for a failed run: the failing case(s)
// and an artifact broker to publish with.
type FailureContext interface {
	LoggerProvider
	TestRegistrantMetadata

	// FailedCases returns the cases that ended Failed or Errored, in execution
	// order. Because the runner bails on the first bad case, this holds exactly
	// one element under current semantics; it is a slice for forward
	// compatibility.
	FailedCases() []FailedCase

	// CleanupPaused reports whether the run was started with cleanup pausing
	// enabled (the --pause-cleanup / -c flag), meaning a developer intends to
	// inspect the still-live environment after this hook returns. A hook whose
	// capture is destructive (e.g. powering off a VM to copy its disk) can use
	// this to skip or defer the destructive work so the paused environment is
	// preserved for inspection.
	CleanupPaused() bool

	// ArtifactBroker returns a broker for publishing diagnostics, with the same
	// surface as TestCase.ArtifactBroker(). It is a no-op unless the suite was
	// run with an artifact output directory (-o), a log directory (-l), or
	// under Azure DevOps (for UploadArtifact).
	ArtifactBroker() artifacts.ArtifactBroker
}

// OnFailure is an optional interface a runnable (scenario or helper) may
// implement to capture diagnostics when its run fails. It is discovered by a
// type assertion on the runnable, so a runnable opts in purely by defining the
// method; no registration call is required.
//
// OnFailure is invoked at most once, after all test cases have run and before
// the runnable's Cleanup() (so resources under test still exist), and only when
// at least one case ended Failed or Errored. It does not fire for skipped cases
// (including SkipAll) or for Setup() failures.
//
// An error or panic returned from OnFailure is logged but does not replace the
// original failure as the reported cause and does not change the exit code.
type OnFailure interface {
	OnFailure(FailureContext) error
}
