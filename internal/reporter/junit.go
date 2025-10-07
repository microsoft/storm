package reporter

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jstemmer/go-junit-report/v2/junit"
	log "github.com/sirupsen/logrus"

	"github.com/microsoft/storm/internal/testmgr"
	"github.com/microsoft/storm/pkg/storm/utils"
)

func toSecondsStr(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 6, 64)
}

func (tr *TestReporter) ProduceJUnitXML(filename string) error {
	testSuites := junit.Testsuites{
		Name: tr.testManager.Suite().Name(),
		Time: toSecondsStr(tr.testManager.Duration()),
	}

	newSuite := junit.Testsuite{
		Name: tr.testManager.Registrant().Name(),
		Time: toSecondsStr(tr.testManager.Duration()),
	}

	for _, testCase := range tr.testManager.TestCases() {
		// Fill in basic properties
		tc := junit.Testcase{
			Name:   testCase.Name(),
			Status: testCase.Status().String(),
		}

		// These properties only make sense if the test was actually run,
		// otherwise they will be misleading.
		if testCase.Status().Ran() {
			tc.Time = toSecondsStr(testCase.RunTime())
			tc.SystemOut = &junit.Output{
				Data: utils.RemoveAllANSI(strings.Join(testCase.CollectedOutput(), "\n")),
			}
		}

		// Now handle the various statuses
		switch testCase.Status() {
		case testmgr.TestCaseStatusPending:
			log.Errorf("Test case %s is still pending, marking as skipped in JUnit report", testCase.Name())
			tc.Skipped = &junit.Result{
				Message: "Test case is still pending",
				Type:    "Pending",
			}
		case testmgr.TestCaseStatusRunning:
			log.Errorf("Test case %s is still running, marking as error in JUnit report", testCase.Name())
			tc.Error = &junit.Result{
				Message: "Test case is still running",
				Type:    "Running",
			}
		case testmgr.TestCaseStatusNotRun:
			tc.Skipped = &junit.Result{
				Message: "Test case was not run",
				Type:    "NotRun",
			}
		case testmgr.TestCaseStatusSkipped:
			tc.Skipped = &junit.Result{
				Message: testCase.Reason(),
				Type:    "Skipped",
			}
		case testmgr.TestCaseStatusFailed:
			tc.Failure = &junit.Result{
				Message: testCase.Reason(),
			}
		case testmgr.TestCaseStatusError:
			tc.Error = &junit.Result{
				Message: testCase.Reason(),
			}
		case testmgr.TestCaseStatusPassed:
			// No action needed
		default:
			log.Warnf("Test case %s has unknown status %v, marking as error in JUnit report", testCase.Name(), testCase.Status())
			tc.Error = &junit.Result{
				Message: "Unknown test case status",
				Type:    "InvalidStatus",
			}
		}

		newSuite.AddTestcase(tc)
	}

	testSuites.AddSuite(newSuite)

	var buffer bytes.Buffer
	err := testSuites.WriteXML(&buffer)
	if err != nil {
		return fmt.Errorf("failed to generate JUnit XML: %w", err)
	}

	err = os.WriteFile(filename, buffer.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("failed to write JUnit XML to file: %w", err)
	}

	return nil
}
