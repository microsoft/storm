package runner

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/microsoft/storm/internal/artifacts"
	"github.com/microsoft/storm/internal/devops"
	"github.com/microsoft/storm/internal/testmgr"
	"github.com/microsoft/storm/pkg/storm/core"
	"github.com/microsoft/storm/pkg/storm/utils"

	"github.com/sirupsen/logrus"
)

// fakeSuite is a minimal core.SuiteContext for exercising executeTestCases.
type fakeSuite struct {
	log *logrus.Logger
	ado bool
}

func (s *fakeSuite) Name() string                  { return "storm-test" }
func (s *fakeSuite) Logger() *logrus.Logger        { return s.log }
func (s *fakeSuite) Scenarios() []core.Scenario    { return nil }
func (s *fakeSuite) Scenario(string) core.Scenario { return nil }
func (s *fakeSuite) Helpers() []core.Helper        { return nil }
func (s *fakeSuite) Helper(string) core.Helper     { return nil }
func (s *fakeSuite) AzureDevops() bool             { return s.ado }
func (s *fakeSuite) Context() context.Context      { return context.Background() }

// fakeHelper is a minimal core.Helper with a single passing test case that
// emits a line of output.
type fakeHelper struct{}

func (h *fakeHelper) Name() string { return "hw" }
func (h *fakeHelper) Args() any    { return nil }
func (h *fakeHelper) RegisterTestCases(r core.TestRegistrar) error {
	r.RegisterTestCase("myPassingTestCase", func(tc core.TestCase) error {
		fmt.Println("hello from the case")
		return nil
	})
	return nil
}

// TestExecuteTestCasesAzureDevopsGrouping asserts that under Azure DevOps a
// test case's live output is bracketed by its "(started)" and status lines
// OUTSIDE a collapsible ##[group]/##[endgroup] section, all on the shared
// devops stream and in the correct order.
func TestExecuteTestCasesAzureDevopsGrouping(t *testing.T) {
	var buf bytes.Buffer
	restore := devops.SetStdout(&buf)
	defer restore()

	logger := logrus.New()
	logger.SetOutput(&bytes.Buffer{}) // keep suite logger noise out of the test output
	suite := &fakeSuite{log: logger, ado: true}

	helper := &fakeHelper{}
	registrant := &runnableInstance{Argumented: helper, TestRegistrant: helper}

	artifactManager, err := artifacts.NewArtifactManager(suite, nil, nil)
	if err != nil {
		t.Fatalf("failed to create artifact manager: %v", err)
	}

	testMgr, err := testmgr.NewStormTestManager(suite, registrant, artifactManager)
	if err != nil {
		t.Fatalf("failed to create test manager: %v", err)
	}

	if err := executeTestCases(suite, registrant, testMgr, false); err != nil {
		t.Fatalf("executeTestCases returned error: %v", err)
	}

	// Split into ANSI-stripped, trimmed lines.
	var lines []string
	for _, l := range strings.Split(buf.String(), "\n") {
		lines = append(lines, strings.TrimRight(utils.RemoveAllANSI(l), " \r"))
	}

	// indexOf returns the index of the first line satisfying match, or -1.
	indexOf := func(match func(string) bool) int {
		for i, l := range lines {
			if match(l) {
				return i
			}
		}
		return -1
	}

	started := indexOf(func(l string) bool { return l == "myPassingTestCase (started)" })
	group := indexOf(func(l string) bool { return l == "##[group]myPassingTestCase" })
	output := indexOf(func(l string) bool {
		return strings.HasPrefix(l, "  ├") && strings.Contains(l, "hello from the case")
	})
	endgroup := indexOf(func(l string) bool { return l == "##[endgroup]" })
	status := indexOf(func(l string) bool { return l == "myPassingTestCase PASS" })

	for _, tc := range []struct {
		name string
		idx  int
	}{
		{"(started) line", started},
		{"##[group] line", group},
		{"forwarded output line", output},
		{"##[endgroup] line", endgroup},
		{"status line", status},
	} {
		if tc.idx == -1 {
			t.Fatalf("expected to find %s in output; got:\n%s", tc.name, buf.String())
		}
	}

	if !(started < group && group < output && output < endgroup && endgroup < status) {
		t.Errorf("lines out of order: started=%d group=%d output=%d endgroup=%d status=%d\noutput:\n%s",
			started, group, output, endgroup, status, buf.String())
	}
}

// TestExecuteTestCasesNonAzureDevopsNoMarkers asserts that outside Azure DevOps
// no group markers are emitted to the shared devops stream.
func TestExecuteTestCasesNonAzureDevopsNoMarkers(t *testing.T) {
	var buf bytes.Buffer
	restore := devops.SetStdout(&buf)
	defer restore()

	logger := logrus.New()
	logger.SetOutput(&bytes.Buffer{})
	suite := &fakeSuite{log: logger, ado: false}

	helper := &fakeHelper{}
	registrant := &runnableInstance{Argumented: helper, TestRegistrant: helper}

	artifactManager, err := artifacts.NewArtifactManager(suite, nil, nil)
	if err != nil {
		t.Fatalf("failed to create artifact manager: %v", err)
	}
	testMgr, err := testmgr.NewStormTestManager(suite, registrant, artifactManager)
	if err != nil {
		t.Fatalf("failed to create test manager: %v", err)
	}

	if err := executeTestCases(suite, registrant, testMgr, false); err != nil {
		t.Fatalf("executeTestCases returned error: %v", err)
	}

	if strings.Contains(buf.String(), "##[group]") || strings.Contains(buf.String(), "##[endgroup]") {
		t.Errorf("expected no group markers outside Azure DevOps, got:\n%s", buf.String())
	}
}
