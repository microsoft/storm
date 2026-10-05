package runner

import (
	"bytes"
	"context"

	"github.com/microsoft/storm/pkg/storm/core"

	"github.com/sirupsen/logrus"
)

// fakeSuite is a minimal core.SuiteContext for exercising executeTestCases.
// It is shared across the runner tests: ctx drives Context() (defaulting to
// context.Background() when nil) and ado drives AzureDevops().
type fakeSuite struct {
	log *logrus.Logger
	ctx context.Context
	ado bool
}

// newFakeSuite builds a fakeSuite with a discarding logger and the given
// context.
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
func (s *fakeSuite) AzureDevops() bool             { return s.ado }

func (s *fakeSuite) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}
