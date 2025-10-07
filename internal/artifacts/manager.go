package artifacts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/microsoft/storm/pkg/storm/core"
)

type ArtifactManager struct {
	suite  core.SuiteContext
	logDir *string
}

// NewArtifactManager creates a new artifact manager. If logDir is nil, no
// log artifacts will be saved.
func NewArtifactManager(suite core.SuiteContext, logDir *string) *ArtifactManager {
	return &ArtifactManager{
		suite:  suite,
		logDir: logDir,
	}
}

// NewBroker creates a new artifact child broker that is attached to this
// manager. The broker must be attached to a test case before it can be used to
// publish artifacts.
func (m *ArtifactManager) NewBroker() *ArtifactBroker {
	return &ArtifactBroker{
		manager: m,
	}
}

// publishLogFile is the internal implementation of publishing a log file. It
// is called by the artifact broker when a test case wants to publish a log
// file.
func (b *ArtifactManager) publishLogFile(testcase core.TestCase, name string, source string) error {
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

	destPath := filepath.Join(*b.logDir, testcase.Name(), name)
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
