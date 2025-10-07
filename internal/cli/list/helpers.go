package list

import (
	"fmt"

	"github.com/microsoft/storm/pkg/storm/core"
)

type ListHelpersCmd struct {
}

func (cmd *ListHelpersCmd) Run(suite core.SuiteContext) error {
	log := suite.Logger()
	log.Info("Listing all helpers")

	for _, helper := range suite.Helpers() {
		fmt.Println(helper.Name())
	}

	return nil
}
