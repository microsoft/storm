package suite

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"slices"

	"github.com/microsoft/storm/internal/cli"
	"github.com/microsoft/storm/internal/collector"
	"github.com/microsoft/storm/pkg/storm/core"

	"github.com/sirupsen/logrus"
)

type StormSuite struct {
	name        string
	scenarios   []core.Scenario
	ctx         context.Context
	cancel      context.CancelFunc
	Log         *logrus.Logger
	helpers     []core.Helper
	scripts     []any
	azureDevops bool
}

func CreateSuite(name string) StormSuite {
	name = fmt.Sprintf("storm-%s", name)

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		ForceColors: true,
	})

	// Create a copy of stdErr and set it as the output for the logger. This
	// means that regardless of any changes to os.Stderr we will still log
	// correctly.
	stdErrCopy := os.Stderr
	logger.SetOutput(stdErrCopy)

	logger.Infof("Creating suite '%s'", name)

	ctx, cancel := context.WithCancel(context.Background())

	return StormSuite{
		name:      name,
		ctx:       ctx,
		cancel:    cancel,
		scenarios: make([]core.Scenario, 0),
		helpers:   make([]core.Helper, 0),
		scripts:   make([]any, 0),
		Log:       logger,
	}
}

// Run the storm suite
func (s *StormSuite) Run() {
	// Parse command-line arguments
	kong_ctx, globals := cli.ParseCommandLine(s.name, s.scripts)

	s.azureDevops = globals.AzureDevops
	s.Log.SetLevel(globals.Verbosity)

	s.Log.Infof("Running suite '%s' - %d scenarios, %d helpers collected.", s.name, len(s.scenarios), len(s.helpers))
	kong_ctx.BindTo(s, (*core.SuiteContext)(nil))
	err := kong_ctx.Run()

	// Cancel the suite context.
	s.cancel()

	// This call will end the program.
	s.reportExitStatus(err)
}

// Adds a scenario to the suite
func (s *StormSuite) AddScenario(new_scenario core.Scenario) {
	if slices.ContainsFunc(s.scenarios, func(scenario core.Scenario) bool {
		return scenario.Name() == new_scenario.Name()
	}) {
		s.Log.Fatalf("Scenario '%s' already exists", new_scenario.Name())
	}

	if err := core.ValidateEntityName(new_scenario.Name(), core.RegistrantTypeScenario.String()); err != nil {
		s.Log.WithError(err).Fatal("Failed to create scenario")
	}

	// Check that we can collect test cases from the scenario
	_, err := collector.CollectTestCases(new_scenario)
	if err != nil {
		s.Log.WithError(err).Fatalf("Failed to collect test cases from scenario '%s'", new_scenario.Name())
	}

	s.Log.Debugf("Registering scenario '%s'", new_scenario.Name())
	s.Log.Tracef("Tags: %v", new_scenario.Tags())
	s.Log.Tracef("Stage paths: %v", new_scenario.StagePaths())
	s.scenarios = append(s.scenarios, new_scenario)
}

// Adds a helper to the suite
func (s *StormSuite) AddHelper(helper core.Helper) {
	if slices.ContainsFunc(s.helpers, func(h core.Helper) bool {
		return h.Name() == helper.Name()
	}) {
		s.Log.Fatalf("Helper '%s' already exists", helper.Name())
	}

	if err := core.ValidateEntityName(helper.Name(), core.RegistrantTypeHelper.String()); err != nil {
		s.Log.WithError(err).Fatal("Failed to create helper")
	}

	// Check that we can collect test cases from the scenario
	_, err := collector.CollectTestCases(helper)
	if err != nil {
		s.Log.WithError(err).Fatalf("Failed to collect test cases from helper '%s'", helper.Name())
	}

	s.Log.Debugf("Registering helper '%s'", helper.Name())
	s.helpers = append(s.helpers, helper)
}

// Adds a script to the suite
func (s *StormSuite) AddScriptSet(script any) {
	v := reflect.ValueOf(script)
	iv := reflect.Indirect(v)
	if v.Kind() != reflect.Ptr || iv.Kind() != reflect.Struct {
		_, file, line, ok := runtime.Caller(1)
		if !ok {
			file = "unknown"
			line = 0
		}
		s.Log.Fatalf("Script from '%s:%d' must be a pointer to a struct but got %T", file, line, script)
	}

	scriptName := v.Type().String()

	// Validate that all public fields have the cmd:"" tag
	t := iv.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Check for cmd tag
		_, ok := field.Tag.Lookup("cmd")
		if !ok {
			s.Log.Fatalf(
				"Failed to validate script struct '%s': script structs may only contain subcommand fields, but field '%s' is missing a cmd tag.",
				scriptName,
				field.Name,
			)
		}
	}

	s.scripts = append(s.scripts, script)
}

// Returns the name of the suite
func (s *StormSuite) Name() string {
	return s.name
}

// Returns a list of all scenarios
func (s *StormSuite) Scenarios() []core.Scenario {
	return s.scenarios
}

// Returns a scenario by name, will exit with an error if the scenario is not
// found.
func (s *StormSuite) Scenario(name string) core.Scenario {
	for _, scenario := range s.scenarios {
		if scenario.Name() == name {
			return scenario
		}
	}

	s.Log.Fatalf("Scenario '%s' not found", name)
	return nil
}

// Returns a list of all helpers
func (s *StormSuite) Helpers() []core.Helper {
	return s.helpers
}

// Returns a helper by name, will exit with an error if the helper is not
// found.
func (s *StormSuite) Helper(name string) core.Helper {
	for _, helper := range s.helpers {
		if helper.Name() == name {
			return helper
		}
	}

	s.Log.Fatalf("Helper '%s' not found", name)
	return nil
}

func (s *StormSuite) Scripts() []any {
	return s.scripts
}

func (s *StormSuite) Logger() *logrus.Logger {
	return s.Log
}

func (s *StormSuite) AzureDevops() bool {
	return s.azureDevops
}

func (s *StormSuite) Context() context.Context {
	return s.ctx
}
