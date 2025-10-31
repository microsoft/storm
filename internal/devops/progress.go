package devops

import (
	"fmt"
	"math"
)

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
