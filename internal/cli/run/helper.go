package run

import (
	"github.com/microsoft/storm/internal/runner"
	"github.com/microsoft/storm/pkg/storm/core"
)

type HelperCmd struct {
	Helper     string             `arg:"" name:"helper" help:"Name of the helper to run"`
	Common     CommonRunnableOpts `embed:""`
	HelperArgs []string           `arg:"" passthrough:"all" help:"Arguments to pass to the helper, you may use '--' to force passthrough." optional:""`
}

func (cmd *HelperCmd) Run(suite core.SuiteContext) error {
	log := suite.Logger()
	log.Infof("Running helper '%s'", cmd.Helper)

	helper := suite.Helper(cmd.Helper)

	return runner.RegisterAndRunTests(
		suite,
		helper,
		cmd.HelperArgs,
		cmd.Common.Watch,
		cmd.Common.LogDir,
		cmd.Common.JUnit,
		cmd.Common.Output,
	)
}
