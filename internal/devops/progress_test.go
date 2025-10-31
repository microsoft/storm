package devops

import (
	"bytes"
	"testing"
)

func TestSetProgress_IntZero(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(0)

	expected := "##vso[task.setprogress value=0]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_Int50(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(50)

	expected := "##vso[task.setprogress value=50]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_Int100(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(100)

	expected := "##vso[task.setprogress value=100]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_IntAbove100(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(150)

	expected := "##vso[task.setprogress value=100]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_IntNegative(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(-10)

	expected := "##vso[task.setprogress value=0]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_FloatZero(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(0.0)

	expected := "##vso[task.setprogress value=0]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_FloatHalf(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(0.5)

	expected := "##vso[task.setprogress value=50]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_FloatOne(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(1.0)

	expected := "##vso[task.setprogress value=100]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_FloatAboveOne(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(1.5)

	expected := "##vso[task.setprogress value=100]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_FloatNegative(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(-0.1)

	expected := "##vso[task.setprogress value=0]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_FloatRoundDown(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(0.244) // Should round to 24%

	expected := "##vso[task.setprogress value=24]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_FloatRoundUp(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(0.755) // Should round to 76%

	expected := "##vso[task.setprogress value=76]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_FloatRoundHalf(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(0.335) // Should round to 34% (0.5 rounds to even)

	expected := "##vso[task.setprogress value=34]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestSetProgress_FloatPrecision(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	SetProgress(0.12345) // Should round to 12%

	expected := "##vso[task.setprogress value=12]\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}
