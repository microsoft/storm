package testmgr

import (
	"fmt"
	"time"

	"github.com/microsoft/storm/internal/artifacts"
	"github.com/microsoft/storm/internal/collector"
	"github.com/microsoft/storm/pkg/storm/core"
)

type StormTestManager struct {
	registrant core.TestRegistrantMetadata
	suite      core.SuiteContext
	startTime  time.Time
	testCases  []*TestCase
}

func NewStormTestManager(
	suite core.SuiteContext,
	registrant interface {
		core.TestRegistrant
		core.TestRegistrantMetadata
	},
	logDir *string,
) (*StormTestManager, error) {
	collected, err := collector.CollectTestCases(registrant)
	if err != nil {
		return nil, fmt.Errorf("failed to collect test cases: %w", err)
	}

	// Create a global artifact manager. Each test case will attach itself to
	// this manager when it is invoked.
	artifactManager := artifacts.NewArtifactManager(suite, logDir)

	testCases := make([]*TestCase, len(collected))
	for i, testCase := range collected {
		testCases[i] = newTestCase(testCase.Name, testCase.F, suite.Context(), artifactManager.NewBroker())
	}

	return &StormTestManager{
		registrant: registrant,
		suite:      suite,
		startTime:  time.Now(),
		testCases:  testCases,
	}, nil
}

func (tm *StormTestManager) TestCases() []*TestCase {
	return tm.testCases
}

func (tm *StormTestManager) Registrant() core.TestRegistrantMetadata {
	return tm.registrant
}

func (tm *StormTestManager) Suite() core.SuiteContext {
	return tm.suite
}
