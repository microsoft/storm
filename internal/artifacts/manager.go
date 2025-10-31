package artifacts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/microsoft/storm/internal/devops"
	"github.com/microsoft/storm/pkg/storm/core"
)

type ArtifactManager struct {
	suite       core.SuiteContext
	logDir      *string
	artifactDir *string
}

// NewArtifactManager creates a new artifact manager. If logDir is nil, no
// log artifacts will be saved.
func NewArtifactManager(suite core.SuiteContext, logDir *string, artifactDir *string) (*ArtifactManager, error) {
	a := &ArtifactManager{
		suite:       suite,
		logDir:      logDir,
		artifactDir: artifactDir,
	}

	err := a.prepare()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare artifact manager: %w", err)
	}

	return a, nil
}

// NewBroker creates a new artifact child broker that is attached to this
// manager. The broker must be attached to a test case before it can be used to
// publish artifacts.
func (m *ArtifactManager) NewBroker() *ArtifactBroker {
	return &ArtifactBroker{
		manager: m,
	}
}

// Prepare prepares the artifact manager for use. This creates any necessary
// directories.
func (b *ArtifactManager) prepare() error {
	// Prepare the log directory if needed
	if b.logDir != nil {
		b.suite.Logger().Infof("Saving logs to '%s'", *b.logDir)
		err := os.MkdirAll(*b.logDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create log directory '%s': %w", *b.logDir, err)
		}
	}

	// Prepare the artifact directory if needed
	if b.artifactDir != nil {
		b.suite.Logger().Infof("Saving artifacts to '%s'", *b.artifactDir)
		err := os.MkdirAll(*b.artifactDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create artifact directory '%s': %w", *b.artifactDir, err)
		}
	}

	return nil
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

func (b *ArtifactManager) publishArtifact(directory string, source string) error {
	if b.artifactDir == nil {
		b.suite.Logger().Warnf("Not publishing artifact from '%s' because no artifact directory was configured", source)
		return nil
	}

	if source == "" {
		return fmt.Errorf("artifact source cannot be empty")
	}

	abspath, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for source %s: %w", source, err)
	}

	// Check if source is a directory or a file
	info, err := os.Stat(abspath)
	if err != nil {
		return fmt.Errorf("failed to stat source %s: %w", abspath, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("source %s is not a regular file", abspath)
	}

	// Default to current directory if none specified
	if directory == "" {
		directory = "."
	}

	destPath := filepath.Join(*b.artifactDir, directory, info.Name())
	err = MkdirParents(destPath, 0o755)
	if err != nil {
		return err
	}

	_, err = CopyFile(abspath, destPath)
	if err != nil {
		return err
	}

	return nil
}

func (b *ArtifactManager) uploadArtifact(name string, directory string, source string) error {
	if !b.suite.AzureDevops() {
		b.suite.Logger().Warnf("Not uploading artifact '%s' because not running in Azure DevOps context", name)
		return nil
	}

	if name == "" {
		return fmt.Errorf("artifact name cannot be empty")
	}

	if source == "" {
		return fmt.Errorf("artifact source cannot be empty")
	}

	abspath, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for source %s: %w", source, err)
	}

	// Check if source is a directory or a file
	info, err := os.Stat(abspath)
	if err != nil {
		return fmt.Errorf("failed to stat source %s: %w", abspath, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("source %s is not a regular file", abspath)
	}

	return devops.PublishArtifact(directory, name, abspath)
}
