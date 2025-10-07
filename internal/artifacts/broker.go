package artifacts

import (
	"fmt"

	"github.com/microsoft/storm/pkg/storm/core"
)

type ArtifactBroker struct {
	// The parent artifact manager.
	manager *ArtifactManager

	// The test case this broker is attached to.
	testCase core.TestCase
}

func (b *ArtifactBroker) AttachTestCase(tc core.TestCase) {
	b.testCase = tc
}

func (b *ArtifactBroker) PublishLogFile(name string, source string) {
	if b.testCase == nil {
		// This should never happen as the broker is initialized and attached to
		// a test case internally by storm, but just in case, we report an
		// internal error via panic.
		panic("internal error: Artifact broker was not attached to a test case before publishing a log file")
	}

	if b.manager == nil {
		panic("internal error: Artifact broker was not attached to an artifact manager before publishing a log file")
	}

	err := b.manager.publishLogFile(b.testCase, name, source)
	if err != nil {
		b.testCase.Error(fmt.Errorf("failed to publish log file %s from path %s: %w", name, source, err))
	}
}
