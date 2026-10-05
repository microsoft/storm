package storm

import (
	"github.com/microsoft/storm/pkg/storm/core"
	"github.com/microsoft/storm/pkg/storm/suite"
)

type StormSuite = suite.StormSuite
type SuiteContext = core.SuiteContext

type Scenario = core.Scenario
type BaseScenario = core.BaseScenario

type Helper = core.Helper
type BaseHelper = core.BaseHelper

type SetupCleanupContext = core.SetupCleanupContext

type OnFailure = core.OnFailure
type FailureContext = core.FailureContext
type FailedCase = core.FailedCase

type TestRegistrar = core.TestRegistrar
type TestCase = core.TestCase
type TestCaseFunction = core.TestCaseFunction

type LoggerProvider = core.LoggerProvider

// Creates a new suite with the given name.
func CreateSuite(name string) StormSuite {
	return suite.CreateSuite(name)
}
