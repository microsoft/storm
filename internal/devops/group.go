package devops

import (
	"fmt"
	"slices"
	"sync"
)

// Groups function as a stack, so we keep track of the groups in a stack.
var groups = make([]*Group, 0)
var groupsMutex sync.Mutex

// Opens a new group and adds it to the stack.
func OpenGroup(name string) *Group {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()

	newGroup := &Group{index: len(groups)}
	groups = append(groups, newGroup)
	logCreateGroup(name)
	return newGroup
}

func logCreateGroup(name string) {
	fmt.Fprintf(realStdOut, "##[group]%s\n", name)
}

func logEndGroup() {
	fmt.Fprintln(realStdOut, "##[endgroup]")
}

// Group type. It MUST have a non-zero size to ensure unique pointers.
type Group struct{ index int }

// Closes the group and closes+removes all groups above it from the stack(aka
// subgroups).
func (g *Group) Close() {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()

	groupIndex := slices.Index(groups, g)
	if groupIndex == -1 {
		// Group not found; nothing to close
		return
	}

	// Figure out how many groups to close
	groupsToClose := len(groups) - groupIndex

	// Close all groups above and including this one
	for range groupsToClose {
		logEndGroup()
	}

	// Remove all groups from the stack starting from groupIndex
	groups = groups[:groupIndex]
}
