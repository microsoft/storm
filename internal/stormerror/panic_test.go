package stormerror

import (
	"testing"
)

func TestNewPanicError(t *testing.T) {
	t.Run("creates panic error with string", func(t *testing.T) {
		stack := []byte("stack trace here")
		err := NewPanicError("test panic", stack)

		if err.any != "test panic" {
			t.Errorf("expected panic value 'test panic', got %v", err.any)
		}

		if string(err.Stack) != string(stack) {
			t.Errorf("expected stack trace to match")
		}
	})

	t.Run("creates panic error with error", func(t *testing.T) {
		stack := []byte("stack trace")
		panicErr := NewPanicError(42, stack)

		if panicErr.any != 42 {
			t.Errorf("expected panic value 42, got %v", panicErr.any)
		}
	})

	t.Run("creates panic error with nil", func(t *testing.T) {
		stack := []byte("stack")
		err := NewPanicError(nil, stack)

		if err.any != nil {
			t.Errorf("expected panic value nil, got %v", err.any)
		}
	})
}

func TestPanicError_Error(t *testing.T) {
	t.Run("formats string panic", func(t *testing.T) {
		err := NewPanicError("something went wrong", []byte("stack"))
		expected := "panic occurred: something went wrong"

		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("formats integer panic", func(t *testing.T) {
		err := NewPanicError(123, []byte("stack"))
		expected := "panic occurred: 123"

		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("formats nil panic", func(t *testing.T) {
		err := NewPanicError(nil, []byte("stack"))
		expected := "panic occurred: <nil>"

		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("formats struct panic", func(t *testing.T) {
		type testStruct struct {
			Field string
		}
		err := NewPanicError(testStruct{Field: "value"}, []byte("stack"))
		expected := "panic occurred: {value}"

		if err.Error() != expected {
			t.Errorf("expected '%s', got '%s'", expected, err.Error())
		}
	})
}
