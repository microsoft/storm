package run

import "github.com/alecthomas/kong"

// ScriptCmd represents the command to run a specific script. All scripts are
// dynamically added as subcommands via kong plugins.
type ScriptCmd struct {
	kong.Plugins
}
