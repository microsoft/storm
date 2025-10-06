package artifacts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/microsoft/storm/pkg/storm/core"
)

type ArtifactBroker struct {
	suite    core.SuiteContext
	testCase core.TestCase
	logDir   *string
}

func NewArtifactBroker(suite core.SuiteContext, logDir *string) *ArtifactBroker {
	return &ArtifactBroker{
		suite:  suite,
		logDir: logDir,
	}
}

func (b *ArtifactBroker) AttachTestCase(tc core.TestCase) {
	b.testCase = tc
}

func (b *ArtifactBroker) PublishLogFile(name string, source string) {
	err := b.publishLogFileInner(name, source)
	if err != nil {
		b.testCase.Error(fmt.Errorf("failed to publish log file %s: %w", name, err))
	}
}

func (b *ArtifactBroker) publishLogFileInner(name string, source string) error {
	if b.logDir == nil {
		b.suite.Logger().Warnf("Not publishing log file '%s' because no log directory was configured", name)
		return nil
	}

	source, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %s: %w", source, err)
	}

	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", source, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("path %s is not a regular file", source)
	}

	destPath := filepath.Join(*b.logDir, b.testCase.Name(), name)
	err = MkdirParents(destPath, 0o755)
	if err != nil {
		return err
	}

	_, err = CopyFile(source, destPath)
	if err != nil {
		return err
	}

	return nil
}
