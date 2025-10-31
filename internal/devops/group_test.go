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

	expected := "##[endgroup]\n"
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

	expected := "##[endgroup]\n"
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

	expected := "##[endgroup]\n"
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

	// Close the outermost group - should close all groups (Outer, Middle, Inner)
	group1.Close()

	// All groups should be closed
	if len(groups) != 0 {
		t.Errorf("expected 0 groups in stack after closing outer, got %d", len(groups))
	}

	// Should see 3 endgroup commands (one for each group)
	endgroupCount := strings.Count(buf.String(), "##[endgroup]\n")
	if endgroupCount != 3 {
		t.Errorf("expected 3 endgroup commands, got %d", endgroupCount)
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

	// Close the middle group - should close Middle and Inner
	group2.Close()

	// Only outer group should remain
	if len(groups) != 1 {
		t.Errorf("expected 1 group in stack after closing middle, got %d", len(groups))
	}

	// Should see 2 endgroup commands (Middle and Inner)
	endgroupCount := strings.Count(buf.String(), "##[endgroup]\n")
	if endgroupCount != 2 {
		t.Errorf("expected 2 endgroup commands, got %d", endgroupCount)
	}

	// Verify correct group remains
	if len(groups) > 0 && groups[0] != group1 {
		t.Error("incorrect group remaining in stack")
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
