package runner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/microsoft/storm/internal/reporter"
	"github.com/microsoft/storm/internal/stormerror"
	"github.com/microsoft/storm/internal/testmgr"
	"github.com/microsoft/storm/pkg/storm/core"

	"github.com/sirupsen/logrus"
)

// RegisterAndRunTests registers the tests from the given registrant and runs
// them. It takes care of argument parsing, setting up the test manager, and
// producing reports. If watch is true, the output of the tests is forwarded to
// the console in real-time. If logDir is not nil, logs are saved to the given
// directory. If junitPath is not nil, a JUnit XML report is produced at the
// given path.
func RegisterAndRunTests(suite core.SuiteContext,
	registrant interface {
		core.Argumented
		core.TestRegistrant
	},
	args []string,
	watch bool,
	logDir *string,
	junitPath *string,
) error {
	// Create a new runnable instance
	registrantInstance := &runnableInstance{
		TestRegistrant: registrant,
		Argumented:     registrant,
	}

	// Parse the extra arguments for the runnable
	err := parseExtraArguments(suite, args, registrantInstance)
	if err != nil {
		return err
	}

	// Create a new test manager for the runnable
	testMgr, err := testmgr.NewStormTestManager(suite, registrantInstance)
	if err != nil {
		return fmt.Errorf("failed to create test manager: %w", err)
	}

	// Actually run the thing
	err = executeTestCases(suite, registrantInstance, testMgr, watch)
	testMgr.StopTimer()
	if err != nil {
		switch err.(type) {
		case *setupError:
			// If setup failed we have no test results to report, we can just
			// exist now.
			return err
		case *cleanupError:
			// If cleanup failed we still want to report the test results.
			suite.Logger().Error(err)
		default:
			// Unknown error, log it and continue.
			suite.Logger().WithError(err).Error("Unknown error occurred!")
		}
	}

	rep := reporter.NewTestReporter(testMgr)

	rep.PrintReport()

	if junitPath != nil {
		junitPath := *junitPath
		junitDir := path.Dir(junitPath)
		suite.Logger().Infof("Producing JUnit XML output at '%s'", junitPath)
		err := os.MkdirAll(junitDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create JUnit output directory '%s': %w", junitDir, err)
		}

		err = rep.ProduceJUnitXML(junitPath)
		if err != nil {
			return fmt.Errorf("failed to produce JUnit XML at '%s': %w", junitPath, err)
		}
	}

	if logDir != nil {
		suite.Logger().Infof("Saving logs to '%s'", *logDir)
		err := os.MkdirAll(*logDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create log directory '%s': %w", *logDir, err)
		}

		err = rep.SaveLogs(*logDir)
		if err != nil {
			return fmt.Errorf("failed to save logs to '%s': %w", *logDir, err)
		}
	}

	return rep.ExitError()
}

// executeTestCases runs all test cases in the given test manager. It takes
// care of calling setup and cleanup methods if the runnable implements the
// SetupCleanup interface. If watch is true, the output of the tests is
// forwarded to the console in real-time.
func executeTestCases(suite core.SuiteContext,
	runnable *runnableInstance,
	testManager *testmgr.StormTestManager,
	watch bool,
) error {

	ctx := &runnableContext{
		LoggerProvider:         suite,
		TestRegistrantMetadata: runnable,
	}

	// If the runnable implements the SetupCleanup interface, we call
	// the setup method before running the tests.
	if r, ok := runnable.TestRegistrant.(core.SetupCleanup); ok {
		err := runCatchPanic(func() error { return r.Setup(ctx) })
		if err != nil {
			return newSetupError(runnable, err)
		}
	}

	cleanupFuncs := make([]func(), 0)

	bail := false

	for _, testCase := range testManager.TestCases() {
		// If bail is true, we are no longer running tests. Mark this test case
		// as not run and 'continue' to iterate over all remaining test cases to
		// mark them as not run.
		if bail {
			testCase.MarkNotRun("dependency failure")
			continue
		}

		suite.Logger().Infof("%s (started)", testCase.Name())

		// Capture the number of goroutines before running the test case.
		// After the test case has run, we compare the number of goroutines to
		// see if we have leaked any. Note that this is not a perfect check as
		// other goroutines in the system may start or stop while we are running
		// the test case, but it is better than nothing.
		var startGoroutines = runtime.NumGoroutine()

		// Call the captureOutput function to run the test case and capture its
		// output. We also forward the output to the console if we are running
		// in watch mode or in Azure DevOps.
		captured, err := captureOutput(func() {
			executeTestCase(testCase)
		}, func(w io.Writer, s string) {
			if suite.AzureDevops() || watch {
				fmt.Fprintf(w, "  ├ %s\n", s)
			}
		})

		// Calculate the difference in goroutine count.
		delta := runtime.NumGoroutine() - startGoroutines

		// Store the captured output in the test case.
		testCase.SetCollectedOutput(captured)

		// If we failed to collect the output, return an error. This means
		// that we didn't even run.
		if err != nil {
			return fmt.Errorf("failed to capture output for '%s': %w", testCase.Name(), err)
		}

		// Grab and store the cleanup functions for this test case.
		cleanupFuncs = append(cleanupFuncs, testCase.SuiteCleanupList()...)

		// Check if the test case caused a bail condition.
		bail = testCase.IsBailCondition()

		// Output the test case status.
		suite.Logger().Infof("%s %s", testCase.Name(), testCase.Status().ColorString())

		// Print a warning if we suspect the test case has leaked goroutines.
		if delta > 0 {
			suite.Logger().Warnf("Test case %s has leaked goroutines: ended with %d more goroutine(s) than expected", testCase.Name(), delta)
		}

	}

	// If we have any cleanup functions, run them in reverse order.
	slices.Reverse(cleanupFuncs)
	for _, f := range cleanupFuncs {
		runCatchPanic(func() error {
			f()
			return nil
		})
	}

	// If the runnable implements the SetupCleanup interface, we call
	// the Cleanup method after running the tests.
	if r, ok := runnable.TestRegistrant.(core.SetupCleanup); ok {
		err := runCatchPanic(func() error { return r.Cleanup(ctx) })
		if err != nil {
			return newCleanupError(runnable, err)
		}
	}

	return nil
}

// executeTestCase runs the given test case in a standalone goroutine to support
// tests calling runtime.Goexit() to terminate. The function will wait for the
// goroutine to finish one way or another and then close the test case
// as appropriate.
//
// If the test case finishes without error and is still marked as running, it is
// marked as passed. Otherwise, if the test case panicked or returned an error,
// it is marked as errored.
func executeTestCase(testCase *testmgr.TestCase) {
	var err error
	var wg sync.WaitGroup

	// Run the runnable in a separate goroutine to so that runtime.Goexit() can
	// be called to stop the test execution.
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Catch any panic that occurs during the execution of the test case and
		// convert it to an error with runCatchPanic.
		err = runCatchPanic(func() error {
			return testCase.Execute()
		})
	}()

	// Wait for the goroutine to finish and close the test with whatever
	// error we receive, if any.
	wg.Wait()

	if err != nil {
		testCase.MarkError(err)
	} else if testCase.Status().IsRunning() {
		testCase.Pass()
	}
}

// runCatchPanic runs the given function f and catches any panic that occurs
// during its execution. If a panic occurs, it is converted to an error of
// type *stormerror.PanicError and returned. If no panic occurs, the error
// returned by f is returned as-is.
func runCatchPanic(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = stormerror.NewPanicError(r, debug.Stack())
		}
	}()

	return f()
}

// captureOutput runs the given function f while capturing all output to
// stdout and stderr. The captured output is returned as a slice of strings,
// one per line. The forward function is called for each line of output, which
// can be used to forward the output to another writer (e.g. the console).
// The forward function is called synchronously, so it should not block for
// too long. The function returns an error if it fails to capture the output.
func captureOutput(f func(), forward func(io.Writer, string)) ([]string, error) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	rOut, wOut, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout capture pipe: %w", err)
	}

	rErr, wErr, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr capture pipe: %w", err)
	}

	os.Stdout = wOut
	os.Stderr = wErr

	logrusOutput := logrus.StandardLogger().Out
	logrusFormatter := logrus.StandardLogger().Formatter
	logrusLevel := logrus.StandardLogger().Level

	// Logrust's standard logger is created on startup and stores a reference to
	// the real stderr then, so our clever redirection does not work. To enable it, we
	// need to set the output of the logger to our pipe as well.
	logrus.SetOutput(os.Stderr)

	// Trick logrus into treating our pipe as the real stderr and force it to TRACE level.
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors: true,
	})
	logrus.SetLevel(logrus.TraceLevel)

	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr

		// Restore the original logrus configuration
		logrus.SetOutput(logrusOutput)
		logrus.SetFormatter(logrusFormatter)
		logrus.SetLevel(logrusLevel)
	}()

	var combinedOutput []string
	var outMutex sync.Mutex
	var wg sync.WaitGroup

	var streamReader = func(r io.Reader, w io.Writer) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			outMutex.Lock()
			combinedOutput = append(combinedOutput, line)
			outMutex.Unlock()
			forward(w, line)
		}
	}

	wg.Add(2)

	go streamReader(rOut, oldStdout)
	go streamReader(rErr, oldStderr)

	f()

	wOut.Close()
	wErr.Close()

	wg.Wait()

	return combinedOutput, nil
}
