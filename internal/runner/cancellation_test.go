package runner

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/microsoft/storm/internal/artifacts"
	"github.com/microsoft/storm/internal/testmgr"
	"github.com/microsoft/storm/pkg/storm/core"

	"github.com/sirupsen/logrus"
)

// fakeSuite is a minimal core.SuiteContext for exercising executeTestCases.
type fakeSuite struct {
	log *logrus.Logger
	ctx context.Context
}

func newFakeSuite(ctx context.Context) *fakeSuite {
	logger := logrus.New()
	logger.SetOutput(&bytes.Buffer{}) // keep logger noise out of test output
	return &fakeSuite{log: logger, ctx: ctx}
}

func (s *fakeSuite) Name() string                  { return "storm-test" }
func (s *fakeSuite) Logger() *logrus.Logger        { return s.log }
func (s *fakeSuite) Scenarios() []core.Scenario    { return nil }
func (s *fakeSuite) Scenario(string) core.Scenario { return nil }
func (s *fakeSuite) Helpers() []core.Helper        { return nil }
func (s *fakeSuite) Helper(string) core.Helper     { return nil }
func (s *fakeSuite) AzureDevops() bool             { return false }
func (s *fakeSuite) Context() context.Context      { return s.ctx }

// fakeHelper is a configurable core.Helper (optionally core.SetupCleanup).
type fakeHelper struct {
	numCases   int
	setupFn    func() error
	cleanupFn  func() error
	suiteClean func() // registered via tc.SuiteCleanup on the first case
}

func (h *fakeHelper) Name() string { return "hw" }
func (h *fakeHelper) Args() any    { return nil }
func (h *fakeHelper) RegisterTestCases(r core.TestRegistrar) error {
	for i := 0; i < h.numCases; i++ {
		name := fmt.Sprintf("case%d", i)
		first := i == 0
		r.RegisterTestCase(name, func(tc core.TestCase) error {
			if first && h.suiteClean != nil {
				tc.SuiteCleanup(h.suiteClean)
			}
			return nil
		})
	}
	return nil
}

// Setup/Cleanup are only used when the test sets setupFn/cleanupFn.
func (h *fakeHelper) Setup(core.SetupCleanupContext) error {
	if h.setupFn != nil {
		return h.setupFn()
	}
	return nil
}

func (h *fakeHelper) Cleanup(core.SetupCleanupContext) error {
	if h.cleanupFn != nil {
		return h.cleanupFn()
	}
	return nil
}

func newTestManager(t *testing.T, suite core.SuiteContext, helper *fakeHelper) (*testmgr.StormTestManager, *runnableInstance) {
	t.Helper()
	registrant := &runnableInstance{Argumented: helper, TestRegistrant: helper}
	am, err := artifacts.NewArtifactManager(suite, nil, nil)
	if err != nil {
		t.Fatalf("failed to create artifact manager: %v", err)
	}
	tm, err := testmgr.NewStormTestManager(suite, registrant, am)
	if err != nil {
		t.Fatalf("failed to create test manager: %v", err)
	}
	return tm, registrant
}

// When the suite context is already cancelled, every test case must be marked
// NotRun rather than executed.
func TestExecuteTestCasesSuiteCancelledMarksNotRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before running

	suite := newFakeSuite(ctx)
	helper := &fakeHelper{numCases: 3}
	tm, registrant := newTestManager(t, suite, helper)

	if err := executeTestCases(suite, registrant, tm, false, false); err != nil {
		t.Fatalf("executeTestCases returned error: %v", err)
	}

	for _, tc := range tm.TestCases() {
		if !tc.Status().NotRun() {
			t.Errorf("case %s: expected NotRun, got %s", tc.Name(), tc.Status().String())
		}
		if tc.Reason() != "suite cancelled" {
			t.Errorf("case %s: expected reason 'suite cancelled', got %q", tc.Name(), tc.Reason())
		}
	}
}

// A normal (uncancelled) run should execute and pass all cases.
func TestExecuteTestCasesNormalRunPasses(t *testing.T) {
	suite := newFakeSuite(context.Background())
	helper := &fakeHelper{numCases: 2}
	tm, registrant := newTestManager(t, suite, helper)

	if err := executeTestCases(suite, registrant, tm, false, false); err != nil {
		t.Fatalf("executeTestCases returned error: %v", err)
	}

	for _, tc := range tm.TestCases() {
		if !tc.Status().Passed() {
			t.Errorf("case %s: expected Passed, got %s", tc.Name(), tc.Status().String())
		}
	}
}

// A failing Setup() hook must surface the output it produced in the returned
// error, so it is not swallowed in the default (non-watch) run mode.
func TestSetupErrorSurfacesCollectedOutput(t *testing.T) {
	suite := newFakeSuite(context.Background())
	helper := &fakeHelper{
		numCases: 1,
		setupFn: func() error {
			fmt.Println("SETUP-OUTPUT-MARKER")
			return fmt.Errorf("setup boom")
		},
	}
	tm, registrant := newTestManager(t, suite, helper)

	err := executeTestCases(suite, registrant, tm, false, false)
	if err == nil {
		t.Fatal("expected a setup error, got nil")
	}
	if _, ok := err.(*setupError); !ok {
		t.Fatalf("expected *setupError, got %T", err)
	}
	if !strings.Contains(err.Error(), "SETUP-OUTPUT-MARKER") {
		t.Errorf("expected setup error to surface collected output, got:\n%s", err.Error())
	}
	if !strings.Contains(err.Error(), "setup boom") {
		t.Errorf("expected setup error to include the underlying error, got:\n%s", err.Error())
	}
}

// With pauseCleanup enabled, executeTestCases waits on suite.Context().Done()
// before cleanup; when the context is already cancelled it must not block.
func TestPauseCleanupReturnsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	suite := newFakeSuite(ctx)
	// No test cases: isolates the pauseCleanup wait from the cancellation
	// skip-loop, so we specifically prove the pause wait unblocks.
	helper := &fakeHelper{numCases: 0}
	tm, registrant := newTestManager(t, suite, helper)

	done := make(chan error, 1)
	go func() {
		done <- executeTestCases(suite, registrant, tm, false, true)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("executeTestCases returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("executeTestCases blocked on pauseCleanup despite cancelled context")
	}
}
