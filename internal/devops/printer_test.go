package devops

import (
	"bytes"
	"testing"
)

func TestLogError(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	LogError("something went wrong")

	expected := "##vso[task.logissue type=error]something went wrong\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestLogError_WithFormatting(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	LogError("error code: %d, message: %s", 404, "not found")

	expected := "##vso[task.logissue type=error]error code: 404, message: not found\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestLogError_MultipleArgs(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	LogError("failed to process %s at line %d in file %s", "token", 42, "main.go")

	expected := "##vso[task.logissue type=error]failed to process token at line 42 in file main.go\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestLogWarning(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	LogWarning("this is a warning")

	expected := "##vso[task.logissue type=warning]this is a warning\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestLogWarning_WithFormatting(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	LogWarning("deprecated function %s, use %s instead", "OldFunc()", "NewFunc()")

	expected := "##vso[task.logissue type=warning]deprecated function OldFunc(), use NewFunc() instead\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestLogWarning_MultipleArgs(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	LogWarning("memory usage at %d%% for process %s (PID: %d)", 85, "myapp", 1234)

	expected := "##vso[task.logissue type=warning]memory usage at 85% for process myapp (PID: 1234)\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestLogError_EmptyMessage(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	LogError("")

	expected := "##vso[task.logissue type=error]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestLogWarning_EmptyMessage(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	LogWarning("")

	expected := "##vso[task.logissue type=warning]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}
