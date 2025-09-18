package runner

import (
	"github.com/microsoft/storm/pkg/storm/core"
)

type runnableContext struct {
	core.TestRegistrantMetadata
	core.LoggerProvider
}
