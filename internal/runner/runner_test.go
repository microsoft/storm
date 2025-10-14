package runner

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/microsoft/storm/internal/stormerror"
)

func TestRunCatchPanic(t *testing.T) {
	t.Run("no panic", func(t *testing.T) {
		err := runCatchPanic(func() error { return nil })
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("error", func(t *testing.T) {
		err := runCatchPanic(func() error { return fmt.Errorf("test error") })
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		if _, ok := err.(*stormerror.PanicError); ok {
			t.Errorf("expected non-panic error, got panic error")
		}

		if err.Error() != "test error" {
			t.Errorf("expected test error, got %v", err)
		}
	})

	t.Run("panic", func(t *testing.T) {
		err := runCatchPanic(func() error {
			panic("test panic")
		})
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		pe, ok := err.(stormerror.PanicError)
		if !ok {
			t.Errorf("expected panic error, got non-panic error")
		}

		if pe.Error() != "panic occurred: test panic" {
			t.Errorf("expected panic error, got %v", pe)
		}
	})

	t.Run("panic with integer", func(t *testing.T) {
		err := runCatchPanic(func() error {
			panic(42)
		})
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		pe, ok := err.(stormerror.PanicError)
		if !ok {
			t.Errorf("expected panic error, got non-panic error: %T", err)
		}

		if pe.Error() != "panic occurred: 42" {
			t.Errorf("expected 'panic occurred: 42', got %v", pe.Error())
		}
	})

	t.Run("panic with nil", func(t *testing.T) {
		err := runCatchPanic(func() error {
			panic(nil)
		})
		if err == nil {
			t.Errorf("expected an error, got nil")
		}

		_, ok := err.(stormerror.PanicError)
		if !ok {
			t.Errorf("expected panic error, got non-panic error")
		}
	})

	t.Run("panic includes stack trace", func(t *testing.T) {
		err := runCatchPanic(func() error {
			panic("test panic")
		})

		pe, ok := err.(stormerror.PanicError)
		if !ok {
			t.Errorf("expected panic error, got non-panic error")
		}

		if len(pe.Stack) == 0 {
			t.Error("expected non-empty stack trace")
		}
	})
}

func TestCaptureOutput(t *testing.T) {
	t.Run("captures stdout", func(t *testing.T) {
		output, err := captureOutput(func() {
			fmt.Println("test output")
		}, func(w io.Writer, s string) {})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(output) != 1 {
			t.Fatalf("expected 1 line of output, got %d", len(output))
		}

		if output[0] != "test output" {
			t.Errorf("expected 'test output', got '%s'", output[0])
		}
	})

	t.Run("captures stderr", func(t *testing.T) {
		output, err := captureOutput(func() {
			fmt.Fprintln(os.Stderr, "error output")
		}, func(w io.Writer, s string) {})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(output) != 1 {
			t.Fatalf("expected 1 line of output, got %d", len(output))
		}

		if output[0] != "error output" {
			t.Errorf("expected 'error output', got '%s'", output[0])
		}
	})

	t.Run("captures multiple lines", func(t *testing.T) {
		output, err := captureOutput(func() {
			fmt.Println("line 1")
			fmt.Println("line 2")
			fmt.Println("line 3")
		}, func(w io.Writer, s string) {})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(output) != 3 {
			t.Fatalf("expected 3 lines of output, got %d", len(output))
		}

		expected := []string{"line 1", "line 2", "line 3"}
		for i, exp := range expected {
			if output[i] != exp {
				t.Errorf("line %d: expected '%s', got '%s'", i, exp, output[i])
			}
		}
	})

	t.Run("captures mixed stdout and stderr", func(t *testing.T) {
		output, err := captureOutput(func() {
			fmt.Println("stdout line")
			fmt.Fprintln(os.Stderr, "stderr line")
		}, func(w io.Writer, s string) {})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(output) != 2 {
			t.Fatalf("expected 2 lines of output, got %d", len(output))
		}
	})

	t.Run("forward function is called", func(t *testing.T) {
		var forwarded []string
		output, err := captureOutput(func() {
			fmt.Println("test")
		}, func(w io.Writer, s string) {
			forwarded = append(forwarded, s)
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(forwarded) != 1 {
			t.Fatalf("expected 1 forwarded line, got %d", len(forwarded))
		}

		if forwarded[0] != "test" {
			t.Errorf("expected 'test', got '%s'", forwarded[0])
		}

		if len(output) != 1 || output[0] != "test" {
			t.Error("output should also be captured")
		}
	})

	t.Run("captures empty output", func(t *testing.T) {
		output, err := captureOutput(func() {
			// No output
		}, func(w io.Writer, s string) {})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(output) != 0 {
			t.Fatalf("expected 0 lines of output, got %d", len(output))
		}
	})
}
