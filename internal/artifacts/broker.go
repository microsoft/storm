package artifacts

import (
	"fmt"
	"io"

	"github.com/microsoft/storm/pkg/storm/core"
)

type ArtifactBroker struct {
	// The parent artifact manager.
	manager *ArtifactManager

	// The test case this broker is attached to.
	testCase core.TestCase

	// Optional strategy for reporting publish errors. When nil, errors are
	// reported through testCase.Error(). The OnFailure hook sets this to a
	// logging strategy because its failed test case is already closed, so
	// calling testCase.Error() on it would panic.
	reportErr func(error)
}

func (b *ArtifactBroker) AttachTestCase(tc core.TestCase) {
	b.testCase = tc
}

// SetErrorReporter overrides how the broker reports publish errors. It is used
// for brokers (such as the OnFailure hook's) that are attached to an
// already-closed test case, where testCase.Error() must not be called.
func (b *ArtifactBroker) SetErrorReporter(fn func(error)) {
	b.reportErr = fn
}

// reportError reports a publish error via the configured strategy, defaulting
// to testCase.Error() when none is set.
func (b *ArtifactBroker) reportError(err error) {
	if b.reportErr != nil {
		b.reportErr(err)
		return
	}
	b.testCase.Error(err)
}

func (b *ArtifactBroker) checkState() {
	if b.testCase == nil {
		// This should never happen as the broker is initialized and attached to
		// a test case internally by storm, but just in case, we report an
		// internal error via panic.
		panic("internal error: Artifact broker was not attached to a test case before publishing a log file")
	}

	if b.manager == nil {
		panic("internal error: Artifact broker was not attached to an artifact manager before publishing a log file")
	}
}

// PublishLogFile implements storm/artifacts.ArtifactBroker.
func (b *ArtifactBroker) PublishLogFile(name string, source string) {
	b.checkState()

	err := b.manager.publishLogFile(b.testCase, name, source)
	if err != nil {
		b.reportError(fmt.Errorf("failed to publish log file '%s' from path '%s': %w", name, source, err))
	}
}

// PublishArtifact implements storm/artifacts.ArtifactBroker.
func (b *ArtifactBroker) PublishArtifact(destination string, source string) {
	b.checkState()

	err := b.manager.publishArtifact(destination, source)
	if err != nil {
		b.reportError(fmt.Errorf("failed to publish artifact from path '%s' to output directory: %w", source, err))
	}
}

// PublishArtifactData implements storm/artifacts.ArtifactBroker.
func (b *ArtifactBroker) PublishArtifactData(destination string, data []byte) {
	b.checkState()

	err := b.manager.publishArtifactData(destination, data)
	if err != nil {
		b.reportError(fmt.Errorf("failed to publish artifact data to '%s': %w", destination, err))
	}
}

// StreamArtifactData implements storm/artifacts.ArtifactBroker.
func (b *ArtifactBroker) StreamArtifactData(destination string) io.WriteCloser {
	b.checkState()

	writer, err := b.manager.streamArtifact(destination)
	if err != nil {
		b.reportError(fmt.Errorf("failed to create stream for artifact '%s': %w", destination, err))
	}

	return writer
}

// UploadArtifact implements storm/artifacts.ArtifactBroker.
func (b *ArtifactBroker) UploadArtifact(name string, directory string, source string) {
	b.checkState()

	err := b.manager.uploadArtifact(name, directory, source)
	if err != nil {
		b.reportError(fmt.Errorf("failed to upload artifact '%s' from path '%s': %w", name, source, err))
	}
}
