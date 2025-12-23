package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	stormartifacts "github.com/microsoft/storm/pkg/storm/artifacts"
	"github.com/microsoft/storm/pkg/storm/core"
	"github.com/sirupsen/logrus"
)

type fakeSuiteContext struct {
	name        string
	azureDevops bool
	logger      *logrus.Logger
}

func (s *fakeSuiteContext) Name() string { return s.name }
func (s *fakeSuiteContext) Logger() *logrus.Logger {
	if s.logger == nil {
		s.logger = logrus.New()
	}
	return s.logger
}
func (s *fakeSuiteContext) Scenarios() []core.Scenario         { return nil }
func (s *fakeSuiteContext) Scenario(name string) core.Scenario { return nil }
func (s *fakeSuiteContext) Helpers() []core.Helper             { return nil }
func (s *fakeSuiteContext) Helper(name string) core.Helper     { return nil }
func (s *fakeSuiteContext) AzureDevops() bool                  { return s.azureDevops }
func (s *fakeSuiteContext) Context() context.Context           { return context.Background() }

type fakeTestCase struct{ name string }

func (t *fakeTestCase) Name() string                            { return t.name }
func (t *fakeTestCase) Registrant() core.TestRegistrantMetadata { return nil }
func (t *fakeTestCase) Fail(reason string)                      {}
func (t *fakeTestCase) FailFromError(err error)                 {}
func (t *fakeTestCase) Error(err error)                         {}
func (t *fakeTestCase) Skip(reason string)                      {}
func (t *fakeTestCase) RunTime() time.Duration                  { return 0 }
func (t *fakeTestCase) SuiteCleanup(f func())                   {}
func (t *fakeTestCase) Context() context.Context                { return context.Background() }
func (t *fakeTestCase) BackgroundWaitGroup() *sync.WaitGroup    { return &sync.WaitGroup{} }
func (t *fakeTestCase) ArtifactBroker() stormartifacts.ArtifactBroker {
	return nil
}

func TestNewArtifactManager_PrepareCreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")
	artifactDir := filepath.Join(tmpDir, "artifacts")

	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, &logDir, &artifactDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatalf("expected manager")
	}

	if info, err := os.Stat(logDir); err != nil || !info.IsDir() {
		t.Fatalf("expected logDir to exist as directory")
	}
	if info, err := os.Stat(artifactDir); err != nil || !info.IsDir() {
		t.Fatalf("expected artifactDir to exist as directory")
	}
}

func TestPublishLogFile_NoLogDirIsNoOp(t *testing.T) {
	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.publishLogFile(&fakeTestCase{name: "tc"}, "log.txt", "does-not-matter"); err != nil {
		t.Fatalf("expected no-op, got error: %v", err)
	}
}

func TestPublishLogFile_CopiesFileIntoTestCaseDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "logs")

	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, &logDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src := filepath.Join(tmpDir, "src.log")
	payload := []byte("log-content")
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	tc := &fakeTestCase{name: "case1"}
	if err := m.publishLogFile(tc, "log.txt", src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := filepath.Join(logDir, tc.Name(), "log.txt")
	content, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(content) != string(payload) {
		t.Fatalf("content mismatch: expected %q, got %q", string(payload), string(content))
	}
}

func TestPublishArtifact_NoArtifactDirIsNoOp(t *testing.T) {
	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.publishArtifact("", ""); err != nil {
		t.Fatalf("expected no-op, got error: %v", err)
	}
}

func TestPublishArtifact_ValidatesInputsWhenConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")
	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, nil, &artifactDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.publishArtifact("", "x"); err == nil {
		t.Fatalf("expected error for empty destination")
	}
	if err := m.publishArtifact("a.txt", ""); err == nil {
		t.Fatalf("expected error for empty source")
	}

	dirSource := filepath.Join(tmpDir, "dir")
	if err := os.MkdirAll(dirSource, 0o755); err != nil {
		t.Fatalf("failed to create dir source: %v", err)
	}
	if err := m.publishArtifact("a.txt", dirSource); err == nil {
		t.Fatalf("expected error for directory source")
	}
}

func TestPublishArtifact_CopiesFile(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")
	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, nil, &artifactDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src := filepath.Join(tmpDir, "src.bin")
	payload := []byte("artifact")
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	destination := filepath.Join("nested", "artifact.bin")
	if err := m.publishArtifact(destination, src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	destPath := filepath.Join(artifactDir, destination)
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(content) != string(payload) {
		t.Fatalf("content mismatch: expected %q, got %q", string(payload), string(content))
	}
}

func TestPublishArtifactData_WritesFile(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")
	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, nil, &artifactDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload := []byte("bytes")
	destination := filepath.Join("a", "b", "c.txt")
	if err := m.publishArtifactData(destination, payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(artifactDir, destination))
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(content) != string(payload) {
		t.Fatalf("content mismatch: expected %q, got %q", string(payload), string(content))
	}
}

func TestStreamArtifact_NoArtifactDirReturnsDiscarder(t *testing.T) {
	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w, err := m.streamArtifact("anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := []byte("hello")
	if n, err := w.Write(p); err != nil || n != len(p) {
		t.Fatalf("expected discard write success, n=%d err=%v", n, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}
}

func TestStreamArtifact_ValidatesDestination(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")
	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, nil, &artifactDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := m.streamArtifact(""); err == nil {
		t.Fatalf("expected error for empty destination")
	}
	if _, err := m.streamArtifact("../bad"); err == nil {
		t.Fatalf("expected error for invalid path")
	}
}

func TestStreamArtifact_DuplicateBlockedUntilClose(t *testing.T) {
	// Reset global stream manager for test isolation.
	globalOpenStreamManager = openStreamManager{}

	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")
	suite := &fakeSuiteContext{name: "suite"}
	m, err := NewArtifactManager(suite, nil, &artifactDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w1, err := m.streamArtifact("out.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := m.streamArtifact("out.txt"); err == nil {
		t.Fatalf("expected duplicate destination to error")
	}

	payload := []byte("stream")
	if _, err := w1.Write(payload); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if err := w1.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(artifactDir, "out.txt"))
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(content) != string(payload) {
		t.Fatalf("content mismatch: expected %q, got %q", string(payload), string(content))
	}

	w2, err := m.streamArtifact("out.txt")
	if err != nil {
		t.Fatalf("expected open after close to succeed, got error: %v", err)
	}
	payload2 := []byte("stream2")
	if _, err := w2.Write(payload2); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	content2, err := os.ReadFile(filepath.Join(artifactDir, "out.txt"))
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(content2) != string(payload2) {
		t.Fatalf("content mismatch: expected %q, got %q", string(payload2), string(content2))
	}
}

func TestUploadArtifact_NotAzureDevopsIsNoOp(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")
	suite := &fakeSuiteContext{name: "suite", azureDevops: false}
	m, err := NewArtifactManager(suite, nil, &artifactDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := m.uploadArtifact("", "", ""); err != nil {
		t.Fatalf("expected no-op, got error: %v", err)
	}
}
