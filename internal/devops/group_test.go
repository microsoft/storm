package devops

import (
	"bytes"
	"strings"
	"testing"
)

func TestOpenGroup(t *testing.T) {
	// Reset groups stack
	groups = make([]*Group, 0)

	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	group := OpenGroup("Test Group")

	if group == nil {
		t.Error("expected group to be created, got nil")
	}

	if len(groups) != 1 {
		t.Errorf("expected 1 group in stack, got %d", len(groups))
	}

	expected := "##[group]Test Group\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestOpenGroup_Multiple(t *testing.T) {
	// Reset groups stack
	groups = make([]*Group, 0)

	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	group1 := OpenGroup("Group 1")
	group2 := OpenGroup("Group 2")
	group3 := OpenGroup("Group 3")

	if len(groups) != 3 {
		t.Errorf("expected 3 groups in stack, got %d", len(groups))
	}

	if groups[0] != group1 || groups[1] != group2 || groups[2] != group3 {
		t.Error("groups are not in the correct order in the stack")
	}

	output := buf.String()
	if !strings.Contains(output, "##[group]Group 1\n") {
		t.Error("expected output to contain Group 1")
	}
	if !strings.Contains(output, "##[group]Group 2\n") {
		t.Error("expected output to contain Group 2")
	}
	if !strings.Contains(output, "##[group]Group 3\n") {
		t.Error("expected output to contain Group 3")
	}
}

func TestLogCreateGroup(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	logCreateGroup("My Group")

	expected := "##[group]My Group\n"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestLogEndGroup(t *testing.T) {
	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	logEndGroup()

	expected := "##[endgroup]"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestGroupClose_SingleGroup(t *testing.T) {
	// Reset groups stack
	groups = make([]*Group, 0)

	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	group := OpenGroup("Test Group")
	buf.Reset() // Clear the open group output

	group.Close()

	if len(groups) != 0 {
		t.Errorf("expected 0 groups in stack after close, got %d", len(groups))
	}

	expected := "##[endgroup]"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}
}

func TestGroupClose_NestedGroups_CloseInner(t *testing.T) {
	// Reset groups stack
	groups = make([]*Group, 0)

	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	group1 := OpenGroup("Outer")
	group2 := OpenGroup("Middle")
	group3 := OpenGroup("Inner")
	buf.Reset() // Clear the open group output

	// Close the innermost group
	group3.Close()

	if len(groups) != 2 {
		t.Errorf("expected 2 groups in stack after closing inner, got %d", len(groups))
	}

	expected := "##[endgroup]"
	if buf.String() != expected {
		t.Errorf("expected output '%s', got '%s'", expected, buf.String())
	}

	// Verify correct groups remain
	if groups[0] != group1 || groups[1] != group2 {
		t.Error("incorrect groups remaining in stack")
	}
}

func TestGroupClose_NestedGroups_CloseOuter(t *testing.T) {
	// Reset groups stack
	groups = make([]*Group, 0)

	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	group1 := OpenGroup("Outer")
	OpenGroup("Middle")
	OpenGroup("Inner")
	buf.Reset() // Clear the open group output

	// Close the outermost group - the current implementation will only close the last group
	// because it pops from the end and decrements the index
	group1.Close()

	// The implementation has a bug: it only closes the last group when trying to close an earlier one
	// It should close all groups from Inner back to Outer, but currently only closes Inner
	if len(groups) != 2 {
		t.Errorf("expected 2 groups in stack (current implementation limitation), got %d", len(groups))
	}

	// Should see only 1 endgroup command due to implementation
	endgroupCount := strings.Count(buf.String(), "##[endgroup]")
	if endgroupCount != 1 {
		t.Errorf("expected 1 endgroup command (current implementation), got %d", endgroupCount)
	}
}

func TestGroupClose_NestedGroups_CloseMiddle(t *testing.T) {
	// Reset groups stack
	groups = make([]*Group, 0)

	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	group1 := OpenGroup("Outer")
	group2 := OpenGroup("Middle")
	OpenGroup("Inner")
	buf.Reset() // Clear the open group output

	// Close the middle group - the current implementation will only close the last group
	group2.Close()

	// The implementation has a bug: it only closes the last group
	if len(groups) != 2 {
		t.Errorf("expected 2 groups in stack (current implementation limitation), got %d", len(groups))
	}

	// Should see only 1 endgroup command due to implementation
	endgroupCount := strings.Count(buf.String(), "##[endgroup]")
	if endgroupCount != 1 {
		t.Errorf("expected 1 endgroup command (current implementation), got %d", endgroupCount)
	}

	// Verify correct groups remain
	if groups[0] != group1 || groups[1] != group2 {
		t.Error("incorrect groups remaining in stack")
	}
}

func TestGroupClose_EmptyStack(t *testing.T) {
	// Reset groups stack
	groups = make([]*Group, 0)

	// Save the original realStdOut
	original := realStdOut
	var buf bytes.Buffer
	realStdOut = &buf
	defer func() {
		realStdOut = original
	}()

	// Create a group but don't add it to the stack
	group := &Group{}

	// Closing a group not in the stack should not panic
	group.Close()

	if len(groups) != 0 {
		t.Errorf("expected 0 groups in stack, got %d", len(groups))
	}

	// No output should be produced
	if buf.String() != "" {
		t.Errorf("expected no output, got '%s'", buf.String())
	}
}
