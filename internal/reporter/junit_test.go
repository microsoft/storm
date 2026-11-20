package reporter

import (
	"bytes"
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jstemmer/go-junit-report/v2/junit"
	"github.com/microsoft/storm/internal/artifacts"
	"github.com/microsoft/storm/internal/testmgr"
	"github.com/microsoft/storm/pkg/storm/core"

	"github.com/sirupsen/logrus"
)

// fakeSuite is a minimal core.SuiteContext for reporter tests.
type fakeSuite struct{ log *logrus.Logger }

func newFakeSuite() *fakeSuite {
	l := logrus.New()
	l.SetOutput(&bytes.Buffer{})
	return &fakeSuite{log: l}
}

func (s *fakeSuite) Name() string                  { return "storm-test" }
func (s *fakeSuite) Logger() *logrus.Logger        { return s.log }
func (s *fakeSuite) Scenarios() []core.Scenario    { return nil }
func (s *fakeSuite) Scenario(string) core.Scenario { return nil }
func (s *fakeSuite) Helpers() []core.Helper        { return nil }
func (s *fakeSuite) Helper(string) core.Helper     { return nil }
func (s *fakeSuite) AzureDevops() bool             { return false }
func (s *fakeSuite) Context() context.Context      { return context.Background() }

// fakeRegistrant satisfies core.TestRegistrant + core.TestRegistrantMetadata.
type fakeRegistrant struct {
	cases map[string]core.TestCaseFunction
}

func (r *fakeRegistrant) Name() string                        { return "myscenario" }
func (r *fakeRegistrant) RegistrantType() core.RegistrantType { return core.RegistrantTypeScenario }
func (r *fakeRegistrant) RegisterTestCases(t core.TestRegistrar) error {
	for name, fn := range r.cases {
		t.RegisterTestCase(name, fn)
	}
	return nil
}

// runCase drives a single test case to completion, mirroring the runner's
// goroutine execution so a case that calls tc.Fail (runtime.Goexit) only
// terminates its own goroutine.
func runCase(c *testmgr.TestCase) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = c.Execute()
	}()
	wg.Wait()
	if c.Status().IsRunning() {
		c.Pass()
	}
}

// TestProduceJUnitXMLSanitizesAndSetsClassname verifies that a run whose output
// and failure reason contain XML-1.0-illegal control bytes still produces a
// well-formed document, and that every <testcase> carries the registrant name
// as its classname.
func TestProduceJUnitXMLSanitizesAndSetsClassname(t *testing.T) {
	suite := newFakeSuite()

	reg := &fakeRegistrant{cases: map[string]core.TestCaseFunction{
		"passing": func(tc core.TestCase) error { return nil },
		"failing": func(tc core.TestCase) error {
			tc.Fail("failure reason with NUL \x00 byte")
			return nil
		},
	}}

	am, err := artifacts.NewArtifactManager(suite, nil, nil)
	if err != nil {
		t.Fatalf("NewArtifactManager: %v", err)
	}
	tm, err := testmgr.NewStormTestManager(suite, reg, am)
	if err != nil {
		t.Fatalf("NewStormTestManager: %v", err)
	}

	for _, c := range tm.TestCases() {
		// Inject raw console-style output containing a NUL byte, other C0
		// control bytes, and a literal "]]>" CDATA terminator. The terminator
		// is made of legal XML characters, so sanitizeXMLText keeps it; we rely
		// on encoding/xml's ,cdata marshaling to split it safely (verified by
		// the well-formedness + round-trip assertions below).
		c.SetCollectedOutput([]string{"console output with \x00 NUL, \x07 bell and a ]]> terminator, then more"})
		runCase(c)
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "report.junit.xml")

	reporter := NewTestReporter(tm)
	if err := reporter.ProduceJUnitXML(out, false); err != nil {
		t.Fatalf("ProduceJUnitXML: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}

	// 1. No NUL byte must remain anywhere in the document.
	if bytes.IndexByte(data, 0x00) != -1 {
		t.Errorf("emitted JUnit XML still contains a NUL byte")
	}

	// 2. The document must be well-formed XML 1.0 (encoding/xml rejects
	//    illegal control characters and an unescaped "]]>" CDATA terminator,
	//    so this fails on unsanitized/unsplit output).
	type tcase struct {
		Name      string `xml:"name,attr"`
		Classname string `xml:"classname,attr"`
		SystemOut string `xml:"system-out"`
	}
	var parsed struct {
		Cases []tcase `xml:"testsuite>testcase"`
	}
	if err := xml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("emitted JUnit XML is not well-formed: %v", err)
	}

	// 3. Every testcase must carry the registrant name as classname.
	if len(parsed.Cases) != 2 {
		t.Fatalf("expected 2 testcases, got %d", len(parsed.Cases))
	}
	for _, c := range parsed.Cases {
		if c.Classname != "myscenario" {
			t.Errorf("testcase %q: expected classname 'myscenario', got %q", c.Name, c.Classname)
		}
		// 4. The "]]>" terminator must survive round-trip in system-out,
		//    proving it was safely escaped rather than dropped or breaking the
		//    document.
		if c.SystemOut != "" && !strings.Contains(c.SystemOut, "]]>") {
			t.Errorf("testcase %q: expected system-out to preserve ']]>' after round-trip, got %q", c.Name, c.SystemOut)
		}
	}
}

func TestProduceJUnitXMLOmitSkipped(t *testing.T) {
	cases := map[string]core.TestCaseFunction{
		"passed":  func(core.TestCase) error { return nil },
		"skipped": func(tc core.TestCase) error { tc.Skip("not applicable"); return nil },
		"skipAll": func(tc core.TestCase) error { tc.SkipAll("not applicable"); return nil },
		"failed":  func(tc core.TestCase) error { tc.Fail("failed"); return nil },
		"errored": func(tc core.TestCase) error { tc.Error(os.ErrNotExist); return nil },
		"notRun":  func(core.TestCase) error { return nil },
	}
	for _, tt := range []struct {
		name        string
		omitSkipped bool
		allSkipped  bool
		wantTests   int
		wantSkipped int
		wantFailed  int
		wantErrors  int
	}{
		{name: "default", wantTests: 6, wantSkipped: 3, wantFailed: 1, wantErrors: 1},
		{name: "omit", omitSkipped: true, wantTests: 4, wantSkipped: 1, wantFailed: 1, wantErrors: 1},
		{name: "all skipped", omitSkipped: true, allSkipped: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			suite := newFakeSuite()
			selected := cases
			if tt.allSkipped {
				selected = map[string]core.TestCaseFunction{"skipped": cases["skipped"], "skipAll": cases["skipAll"]}
			}
			am, err := artifacts.NewArtifactManager(suite, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			tm, err := testmgr.NewStormTestManager(suite, &fakeRegistrant{cases: selected}, am)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range tm.TestCases() {
				if c.Name() == "notRun" {
					c.MarkNotRun("dependency failure")
				} else {
					runCase(c)
				}
			}
			reporter := NewTestReporter(tm)
			summary := reporter.Summary()
			out := filepath.Join(t.TempDir(), "report.xml")
			if err := reporter.ProduceJUnitXML(out, tt.omitSkipped); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			var report junit.Testsuites
			if err := xml.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			if len(report.Suites) != 1 {
				t.Fatalf("got %d suites, want 1", len(report.Suites))
			}
			ts := report.Suites[0]
			if report.Tests != tt.wantTests || ts.Tests != tt.wantTests || len(ts.Testcases) != tt.wantTests {
				t.Errorf("test counts: root=%d suite=%d cases=%d, want %d", report.Tests, ts.Tests, len(ts.Testcases), tt.wantTests)
			}
			if report.Skipped != tt.wantSkipped || ts.Skipped != tt.wantSkipped {
				t.Errorf("skipped counts: root=%d suite=%d, want %d", report.Skipped, ts.Skipped, tt.wantSkipped)
			}
			if report.Failures != tt.wantFailed || ts.Failures != tt.wantFailed || report.Errors != tt.wantErrors || ts.Errors != tt.wantErrors {
				t.Errorf("unexpected failure/error counts: root=%d/%d suite=%d/%d", report.Failures, report.Errors, ts.Failures, ts.Errors)
			}
			for _, tc := range ts.Testcases {
				if tt.omitSkipped && (tc.Name == "skipped" || tc.Name == "skipAll") {
					t.Errorf("intentional skip %q was included", tc.Name)
				}
				if tc.Name == "notRun" && (tc.Skipped == nil || tc.Skipped.Type != "NotRun") {
					t.Errorf("not-run status was changed: %+v", tc)
				}
			}
			if reporter.Summary() != summary {
				t.Error("JUnit filtering changed the run summary")
			}
			if (reporter.ExitError() != nil) != (tt.wantFailed+tt.wantErrors > 0) {
				t.Error("JUnit filtering changed the exit result")
			}
		})
	}
}
