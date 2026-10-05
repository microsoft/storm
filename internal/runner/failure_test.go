package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/microsoft/storm/internal/artifacts"
	"github.com/microsoft/storm/internal/testmgr"
	"github.com/microsoft/storm/pkg/storm/core"
)

// onFailureHelper is a helper whose cases and OnFailure behavior are
// configurable, and which records what the hook received.
type onFailureHelper struct {
	cases  []caseSpec
	onFail func(core.FailureContext) error

	hookCalled    bool
	gotFailed     []core.FailedCase
	gotCleanupPsd bool
}

type caseSpec struct {
	name string
	fn   core.TestCaseFunction
}

func (h *onFailureHelper) Name() string { return "of" }
func (h *onFailureHelper) Args() any    { return nil }
func (h *onFailureHelper) RegisterTestCases(r core.TestRegistrar) error {
	for _, c := range h.cases {
		r.RegisterTestCase(c.name, c.fn)
	}
	return nil
}
func (h *onFailureHelper) OnFailure(fc core.FailureContext) error {
	h.hookCalled = true
	h.gotFailed = fc.FailedCases()
	h.gotCleanupPsd = fc.CleanupPaused()
	if h.onFail != nil {
		return h.onFail(fc)
	}
	return nil
}

func runOnFailureHelper(t *testing.T, h *onFailureHelper, artifactDir *string) *testmgr.StormTestManager {
	t.Helper()
	return runOnFailureWithSuite(t, newFakeSuite(context.Background()), h, artifactDir, false)
}

func runOnFailureWithSuite(t *testing.T, suite *fakeSuite, h *onFailureHelper, artifactDir *string, pauseCleanup bool) *testmgr.StormTestManager {
	t.Helper()
	registrant := &runnableInstance{Argumented: h, TestRegistrant: h}
	am, err := artifacts.NewArtifactManager(suite, nil, artifactDir)
	if err != nil {
		t.Fatalf("NewArtifactManager: %v", err)
	}
	tm, err := testmgr.NewStormTestManager(suite, registrant, am)
	if err != nil {
		t.Fatalf("NewStormTestManager: %v", err)
	}
	if err := executeTestCases(suite, registrant, tm, am, false, pauseCleanup); err != nil {
		t.Fatalf("executeTestCases returned error: %v", err)
	}
	return tm
}

func passCase(name string) caseSpec {
	return caseSpec{name, func(tc core.TestCase) error { return nil }}
}

// OnFailure fires after a product failure (tc.Fail), receives exactly the
// failing case, and reports it as a non-error failure.
func TestOnFailureFiresOnFailure(t *testing.T) {
	h := &onFailureHelper{cases: []caseSpec{
		passCase("ok"),
		{"bad", func(tc core.TestCase) error { tc.Fail("boom"); return nil }},
	}}

	runOnFailureHelper(t, h, nil)

	if !h.hookCalled {
		t.Fatal("OnFailure was not called after a failure")
	}
	if len(h.gotFailed) != 1 {
		t.Fatalf("expected exactly 1 failed case, got %d", len(h.gotFailed))
	}
	fc := h.gotFailed[0]
	if fc.Name() != "bad" {
		t.Errorf("expected failed case 'bad', got %q", fc.Name())
	}
	if fc.IsError() {
		t.Errorf("tc.Fail should be a product failure, not an error")
	}
	if fc.Reason() != "boom" {
		t.Errorf("expected reason 'boom', got %q", fc.Reason())
	}
}

// OnFailure fires after a test-infrastructure error (returned error) and
// reports IsError()==true.
func TestOnFailureFiresOnError(t *testing.T) {
	h := &onFailureHelper{cases: []caseSpec{
		{"bad", func(tc core.TestCase) error { return errors.New("kaboom") }},
	}}

	runOnFailureHelper(t, h, nil)

	if !h.hookCalled {
		t.Fatal("OnFailure was not called after an error")
	}
	if len(h.gotFailed) != 1 || !h.gotFailed[0].IsError() {
		t.Fatalf("expected one errored case with IsError()==true, got %+v", h.gotFailed)
	}
}

// OnFailure does not fire when every case passes.
func TestOnFailureNotCalledOnPass(t *testing.T) {
	h := &onFailureHelper{cases: []caseSpec{passCase("a"), passCase("b")}}
	runOnFailureHelper(t, h, nil)
	if h.hookCalled {
		t.Error("OnFailure should not fire when all cases pass")
	}
}

// OnFailure does not fire for skips, including SkipAll.
func TestOnFailureNotCalledOnSkip(t *testing.T) {
	t.Run("Skip", func(t *testing.T) {
		h := &onFailureHelper{cases: []caseSpec{
			{"s", func(tc core.TestCase) error { tc.Skip("nope"); return nil }},
			passCase("after"),
		}}
		runOnFailureHelper(t, h, nil)
		if h.hookCalled {
			t.Error("OnFailure should not fire for a skipped case")
		}
	})
	t.Run("SkipAll", func(t *testing.T) {
		h := &onFailureHelper{cases: []caseSpec{
			{"s", func(tc core.TestCase) error { tc.SkipAll("nope"); return nil }},
			passCase("after"),
		}}
		runOnFailureHelper(t, h, nil)
		if h.hookCalled {
			t.Error("OnFailure should not fire for SkipAll")
		}
	})
}

// An error returned from OnFailure must not propagate: executeTestCases still
// returns nil (the original failure remains the reported cause) and the failing
// case keeps its failed status.
func TestOnFailureErrorDoesNotPropagate(t *testing.T) {
	h := &onFailureHelper{
		cases:  []caseSpec{{"bad", func(tc core.TestCase) error { tc.Fail("boom"); return nil }}},
		onFail: func(core.FailureContext) error { return errors.New("hook broke") },
	}
	tm := runOnFailureHelper(t, h, nil) // runOnFailureHelper fails the test if executeTestCases errors

	if !h.hookCalled {
		t.Fatal("OnFailure was not called")
	}
	if !tm.TestCases()[0].Status().Failed() {
		t.Errorf("original failing case should remain Failed, got %s", tm.TestCases()[0].Status().String())
	}
}

// A panic in OnFailure must be recovered and must not propagate.
func TestOnFailurePanicDoesNotPropagate(t *testing.T) {
	h := &onFailureHelper{
		cases:  []caseSpec{{"bad", func(tc core.TestCase) error { tc.Fail("boom"); return nil }}},
		onFail: func(core.FailureContext) error { panic("hook panicked") },
	}
	// If the panic escaped, executeTestCases (and thus the test) would crash.
	runOnFailureHelper(t, h, nil)
	if !h.hookCalled {
		t.Fatal("OnFailure was not called")
	}
}

// The FailureContext broker publishes into the artifact output directory.
func TestOnFailureBrokerPublishes(t *testing.T) {
	dir := t.TempDir()
	h := &onFailureHelper{
		cases: []caseSpec{{"bad", func(tc core.TestCase) error { tc.Fail("boom"); return nil }}},
		onFail: func(fc core.FailureContext) error {
			fc.ArtifactBroker().PublishArtifactData("diag/evidence.txt", []byte("captured"))
			return nil
		},
	}
	runOnFailureHelper(t, h, &dir)

	got, err := os.ReadFile(filepath.Join(dir, "diag", "evidence.txt"))
	if err != nil {
		t.Fatalf("expected published artifact: %v", err)
	}
	if string(got) != "captured" {
		t.Errorf("artifact content = %q, want %q", string(got), "captured")
	}
}

// CleanupPaused reflects the pauseCleanup flag so a destructive hook can adapt.
func TestOnFailureCleanupPaused(t *testing.T) {
	failingCase := caseSpec{"bad", func(tc core.TestCase) error { tc.Fail("boom"); return nil }}

	t.Run("not paused", func(t *testing.T) {
		h := &onFailureHelper{cases: []caseSpec{failingCase}}
		runOnFailureWithSuite(t, newFakeSuite(context.Background()), h, nil, false)
		if !h.hookCalled || h.gotCleanupPsd {
			t.Errorf("expected CleanupPaused()==false, got called=%t paused=%t", h.hookCalled, h.gotCleanupPsd)
		}
	})

	t.Run("paused", func(t *testing.T) {
		// With pauseCleanup=true the runner waits on the suite context after the
		// hook, so the hook cancels it to release the wait. The context is live
		// during the test loop, so the case still runs and fails.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		suite := newFakeSuite(ctx)
		h := &onFailureHelper{
			cases:  []caseSpec{failingCase},
			onFail: func(core.FailureContext) error { cancel(); return nil },
		}
		runOnFailureWithSuite(t, suite, h, nil, true)
		if !h.hookCalled || !h.gotCleanupPsd {
			t.Errorf("expected CleanupPaused()==true, got called=%t paused=%t", h.hookCalled, h.gotCleanupPsd)
		}
	})
}

// A runnable that does not implement OnFailure is unaffected by a failure.
func TestOnFailureAbsentIsNoop(t *testing.T) {
	h := &fakeHelper{numCases: 1}
	suite := newFakeSuite(context.Background())
	registrant := &runnableInstance{Argumented: h, TestRegistrant: h}
	am, err := artifacts.NewArtifactManager(suite, nil, nil)
	if err != nil {
		t.Fatalf("NewArtifactManager: %v", err)
	}
	tm, err := testmgr.NewStormTestManager(suite, registrant, am)
	if err != nil {
		t.Fatalf("NewStormTestManager: %v", err)
	}
	// Should simply run without invoking any hook or erroring.
	if err := executeTestCases(suite, registrant, tm, am, false, false); err != nil {
		t.Fatalf("executeTestCases returned error: %v", err)
	}
	_ = fmt.Sprint(tm.TestCases())
}
