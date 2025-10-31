package helloworld

import "fmt"

// This is a sample script set to demonstrate how to create scripts in Storm.
//
// A script set is a collection of related scripts that can be executed via the
// Storm CLI.
//
// It contains two sample scripts: 1. HelloWorldScript: Prints "Hello, <name>!"
// for each name provided as an argument. 2. MyOtherScript: Prints "This is my
// other script!" a specified number of times.
//
// To run these scripts, you would typically use the Storm CLI with commands
// like:
//
// - `mystorm script hello-world Alice Bob`
// - `mystorm script other 5`
//
// The script set MUST be a struct where all public fields are subcommands
// tagged with `cmd:""`. Each subcommand struct must implement a Run() error
// method.
//
// You can read more about CLI parsing in the Kong documentation:
// https://github.com/alecthomas/kong
type HelloWorldScriptSet struct {
	// This field represents the "hello-world" subcommand.
	HelloWorld HelloWorldScript `cmd:"" help:"Prints hello world messages"`

	// This field represents the "other" subcommand.
	Other MyOtherScript `cmd:"" help:"Prints a custom message multiple times"`

	// You can add more subcommands here as needed. They will all be available
	// under the main "script" command.
	// NOTE: Script names must be unique across ALL script sets.
}

type HelloWorldScript struct {
	Names []string `arg:"" help:"List of names to greet"`
}

func (s *HelloWorldScript) Run() error {
	for _, d := range s.Names {
		fmt.Println("Hello, " + d + "!")
	}

	return nil
}

type MyOtherScript struct {
	Count int `arg:"" help:"Number of times to print the message"`
}

func (s *MyOtherScript) Run() error {
	for i := 0; i < s.Count; i++ {
		fmt.Println("This is my other script!")
	}

	return nil
}
