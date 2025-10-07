# Usage

Every test suite is a standalone binary: `storm-<suite-name>`. You can run it
like any other go binary.

- [Usage](#usage)
  - [CLI Usage](#cli-usage)
  - [Entry Point Definition](#entry-point-definition)
  - [Scenarios](#scenarios)
  - [Helpers](#helpers)
  - [Defining Runtime Args for Scenarios and Helpers](#defining-runtime-args-for-scenarios-and-helpers)
  - [The `RegisterTestCases` Method](#the-registertestcases-method)
  - [Logging](#logging)
  - [Test Cases](#test-cases)
    - [Test Case Logging](#test-case-logging)


## CLI Usage

```text
Usage: storm-<suite-name> <command> [flags]

Flags:
  -h, --help              Show context-sensitive help.
  -v, --verbosity=info    Set log level
  -a, --azure-devops      Enable Azure DevOps integration ($TF_BUILD)

Commands:
  list scenarios [flags]
    List available scenarios

  list tags
    List all tags

  list stage-paths [flags]
    List all stage paths

  list helpers
    List all helpers

  run <scenario> [<scenario-args> ...] [flags]
    Run a specific scenario

  helper <helper> [<helper-args> ...] [flags]
    Run a specific helper
```

## Entry Point Definition

The entry point for each suite is the `main` function defined in `cmd/<suite-name>/main.go`.

This is a sample main function:

```go
package main

import (
    "github.com/microsoft/storm"
)

func main() {
    storm := storm.CreateSuite("trident")

    // Add your scenarios/helpers to the suite here!

    storm.Run()
}
```

## Scenarios

Scenarios should be defined inside `my_module/<suite-name>/testsuite/` or a similar
directory structure.

A scenario is a struct that implements the `storm.Scenario` interface.
It is recommended to compose the `storm.BaseScenario` struct to get the default
implementation of the interface.

The bare minimum for a scenario is to implement the `Name` and 
`RegisterTestCases` methods.

## Helpers

Helpers should be defined inside `my_module/<suite-name>/testsuite/` or a similar
directory structure. *Preferably* in a helpers module.

A helper is a struct that implements the `storm.Helper` interface.
It is recommended to compose the `storm.BaseHelper` struct to get the default
implementation of the interface.

## Defining Runtime Args for Scenarios and Helpers

Both the `storm.Scenario` and `storm.Helper` interfaces include an `Args` method
that MUST return a pointer to a [kong](github.com/alecthomas/kong)-annotated struct.

Example from the `helloworld` suite:

```go
type HelloWorldHelper struct {
    args struct {
        Name string `arg:"" help:"Name of the helper" default:"default" required:""`
    }
}

func (h *HelloWorldHelper) Args() any {
    // 👆 IMPORTANT: Note that the receiver is a POINTER! If you receive by 
    // value, a copy of the struct is made so the returned pointer will point
    // to a copy of the struct and not the original struct.

    //    👇 Note that the returned value is a POINTER too!
    return &h.args
}
```

## The `RegisterTestCases` Method

Both scenarios and helpers must implement a `RegisterTestCases` method where
test cases must be registered in the correct order.

```go
// For both SCENARIOS and HELPERS, the signature is:
func (s MyScenario) RegisterTestCases(r storm.TestRegistrar) error {
    r.RegisterTestCase("test-case-name", func(tc storm.TestCase) error {
        // Your test case logic here
        return nil
    })

    // You can also register other functions and methods of `MyScenario` here!

    return nil
}
```

## Logging

By default, storm will capture stdout, stderr and logrus. Test suites are
encouraged to use these facilities.

## Test Cases

Test cases MUST have unique names within each scenario or helper, and ideally
across the entire suite, unless the same test case is performed in multiple
scenarios/helpers.

```go
func (s *MyScenario) RegisterTestCases(r storm.TestRegistrar) error {
    r.RegisterTestCase("my-test-case", s.myTestCase)
    return nil
}

func (s *MyScenario) myTestCase(tc storm.TestCase) error {
    // Your test case logic here
    return nil
}
```

The `TestCase` interface behaves similarly to the `testing.T` interface in the
standard library. It provides the following methods for reporting results:

- `Fail(reason string)`: Marks the test case as failed and stop execution of the
  current goroutine.
- `FailFromError(err error)`: Same as `Fail`, but the reason is set to the error
  message.
- `Skip(reason string)`: Marks the test case as skipped and stop execution of the
  current goroutine. Following tests can continue.
- `Error(err error)`: Marks the test case as errored and stop execution of the
  current goroutine.

```go
func (s MyScenario) RegisterTestCases(r storm.TestRegistrar) error {
    r.RegisterTestCase("my-test-case", s.myTestCase)
    return nil
}

func (s MyScenario) myTestCase(tc storm.TestCase) error {
    err := someFunction()
    if err != nil {
        tc.FailFromError(err)
    }
    return nil
}
```

### Test Case Logging

Test cases can use the standard logrus logger for logging, and Storm will capture
the output.

```go
func (s MyScenario) RegisterTestCases(r storm.TestRegistrar) error {
    r.RegisterTestCase("my-test-case", s.myTestCase)
    return nil
}

func (s MyScenario) myTestCase(tc storm.TestCase) error {
    logrus.Info("Hello, world!")
    return nil
}
```

Logs can be watched live during test execution by passing the `-w` flag to
scenarios and helpers.