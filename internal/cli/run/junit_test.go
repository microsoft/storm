package run_test

import (
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/jstemmer/go-junit-report/v2/junit"
	"github.com/microsoft/storm/internal/cli/run"
	"github.com/microsoft/storm/pkg/storm/core"
	"github.com/sirupsen/logrus"
)

type junitScenario struct{ core.BaseScenario }

type junitSuite struct {
	log *logrus.Logger
}

func (*junitSuite) Name() string                  { return "junit-test" }
func (s *junitSuite) Logger() *logrus.Logger      { return s.log }
func (*junitSuite) Context() context.Context      { return context.Background() }
func (*junitSuite) AzureDevops() bool             { return false }
func (*junitSuite) Scenarios() []core.Scenario    { return []core.Scenario{&junitScenario{}} }
func (*junitSuite) Scenario(string) core.Scenario { return &junitScenario{} }
func (*junitSuite) Helpers() []core.Helper        { return []core.Helper{&junitScenario{}} }
func (*junitSuite) Helper(string) core.Helper     { return &junitScenario{} }

func (*junitScenario) Name() string { return "junit-test" }
func (*junitScenario) RegisterTestCases(r core.TestRegistrar) error {
	r.RegisterTestCase("passed", func(core.TestCase) error { return nil })
	r.RegisterTestCase("skipped", func(tc core.TestCase) error {
		tc.Skip("not applicable")
		return nil
	})
	return nil
}

func TestJUnitOmitSkippedFlag(t *testing.T) {
	for _, command := range []string{"run", "helper"} {
		for _, omit := range []bool{false, true} {
			name := command + "/default"
			if omit {
				name = command + "/omit"
			}
			t.Run(name, func(t *testing.T) {
				s := &junitSuite{log: logrus.New()}
				s.Logger().SetOutput(io.Discard)
				var cli struct {
					Run    run.ScenarioCmd `cmd:""`
					Helper run.HelperCmd   `cmd:""`
				}
				parser, err := kong.New(&cli)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(t.TempDir(), "report.xml")
				args := []string{command, "-j", path}
				wantTests := 2
				wantSkipped := 1
				if omit {
					args = append(args, "--junit-omit-skipped")
					wantTests, wantSkipped = 1, 0
				}
				args = append(args, "junit-test")
				ctx, err := parser.Parse(args)
				if err != nil {
					t.Fatal(err)
				}
				ctx.BindTo(s, (*core.SuiteContext)(nil))
				if err := ctx.Run(); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var report junit.Testsuites
				if err := xml.Unmarshal(data, &report); err != nil {
					t.Fatal(err)
				}
				if report.Tests != wantTests || report.Skipped != wantSkipped {
					t.Errorf("got tests=%d skipped=%d, want tests=%d skipped=%d", report.Tests, report.Skipped, wantTests, wantSkipped)
				}
			})
		}
	}
}
