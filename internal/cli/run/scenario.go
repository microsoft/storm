package run

import (
	"github.com/microsoft/storm/internal/runner"
	"github.com/microsoft/storm/pkg/storm/core"
)

type ScenarioCmd struct {
	Scenario     string             `arg:"" name:"scenario" help:"Name of the scenario to run"`
	Common       CommonRunnableOpts `embed:""`
	ScenarioArgs []string           `arg:"" passthrough:"all" help:"Arguments to pass to the scenario, you may use '--' to force passthrough." optional:""`
}

func (cmd *ScenarioCmd) Run(suite core.SuiteContext) error {
	log := suite.Logger()
	log.Infof("Running scenario '%s'", cmd.Scenario)

	scenario := suite.Scenario(cmd.Scenario)

	return runner.RegisterAndRunTests(
		suite,
		scenario,
		cmd.ScenarioArgs,
		cmd.Common.Watch,
		cmd.Common.LogDir,
		cmd.Common.JUnit,
		cmd.Common.Output,
	)
}
