package devops

import (
	"fmt"
	"math"
)

// SetProgress sets the progress percentage for a task.
//
// If an int is provided, it is interpreted as a percentage (0-100).
// If a float64 is provided, it is interpreted as a fraction (0.0-1.0) and will be multiplied by 100.
func SetProgress[T float64 | int](progress T) {
	var progressPercent int
	switch v := any(progress).(type) {
	case float64:
		progressPercent = int(math.Round(v * 100))
	case int:
		progressPercent = v
	}

	if progressPercent < 0 {
		progressPercent = 0
	} else if progressPercent > 100 {
		progressPercent = 100
	}

	fmt.Fprintf(realStdOut, "##vso[task.setprogress value=%d]\n", progressPercent)
}
