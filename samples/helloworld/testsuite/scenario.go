// Package helloworld implements a simple hello world scenario and helper.
package helloworld

import (
	"github.com/microsoft/storm"
	"github.com/microsoft/storm/pkg/storm/core"

	"github.com/sirupsen/logrus"
)

type HelloWorldScenario struct {
	// You can embed storm.BaseScenario to get the default implementation of the Scenario interface.
	// This is useful if you are not using most methods.
	// storm.BaseScenario
}

// Args implements core.Scenario.
func (s *HelloWorldScenario) Args() any {
	return nil
}

// Setup implements core.Scenario.
func (s *HelloWorldScenario) Setup(core.SetupCleanupContext) error {
	logrus.Info("Setup called for HelloWorldScenario")
	return nil
}

// Cleanup implements core.Scenario.
func (s *HelloWorldScenario) Cleanup(core.SetupCleanupContext) error {
	logrus.Info("Cleanup called for HelloWorldScenario")
	return nil
}

// OnFailure is an OPTIONAL hook (storm.OnFailure). When implemented, storm
// calls it after the test cases run and before Cleanup() - so resources under
// test still exist - but ONLY when at least one case failed or errored (never
// for skips). Use it to capture diagnostics for the failing run; the broker
// publishes to the directory given with the run's -o flag.
func (s *HelloWorldScenario) OnFailure(fc storm.FailureContext) error {
	for _, c := range fc.FailedCases() {
		logrus.Warnf("Case '%s' failed (error=%t): %s", c.Name(), c.IsError(), c.Reason())
	}

	// A destructive capture would check fc.CleanupPaused() first so it does not
	// tear down an environment a developer paused (--pause-cleanup) to inspect.
	fc.ArtifactBroker().PublishArtifactData("failure-notes.txt", []byte("captured on failure"))
	return nil
}

// RequiredFiles implements core.Scenario.
func (s *HelloWorldScenario) RequiredFiles() []string {
	return nil
}

// StagePaths implements core.Scenario.
func (s *HelloWorldScenario) StagePaths() []string {
	return nil
}

// Tags implements core.Scenario.
func (s *HelloWorldScenario) Tags() []string {
	return nil
}

// Type implements core.Scenario.
func (s *HelloWorldScenario) Name() string {
	return "hello-world"
}

// Description implements core.Scenario.
func (h *HelloWorldScenario) RegisterTestCases(r storm.TestRegistrar) error {
	r.RegisterTestCase("myPassingTestCase", func(tc storm.TestCase) error {
		logrus.Info("This message will be logged in the test case!")

		// Do something here!
		// ...

		return nil
	})
	return nil
}
