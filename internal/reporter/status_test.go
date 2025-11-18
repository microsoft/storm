package reporter

import (
	"testing"
)

func TestTestSummaryStatus_String(t *testing.T) {
	tests := []struct {
		name     string
		status   TestSummaryStatus
		expected string
	}{
		{"OK status", TestStatusOk, "OK"},
		{"FAILED status", TestStatusFailed, "FAILED"},
		{"ERROR status", TestStatusError, "ERROR"},
		{"Invalid status", TestSummaryStatus(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.String()
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestTestSummaryStatus_StringColor(t *testing.T) {
	t.Run("OK status returns colored string", func(t *testing.T) {
		status := TestStatusOk
		result := status.StringColor()
		if result == "" {
			t.Error("expected non-empty colored string")
		}
		// Should contain the text "OK"
		if !contains(result, "OK") {
			t.Errorf("expected colored string to contain 'OK', got: %s", result)
		}
	})

	t.Run("FAILED status returns colored string", func(t *testing.T) {
		status := TestStatusFailed
		result := status.StringColor()
		if result == "" {
			t.Error("expected non-empty colored string")
		}
		// Should contain the text "FAILED"
		if !contains(result, "FAILED") {
			t.Errorf("expected colored string to contain 'FAILED', got: %s", result)
		}
	})

	t.Run("ERROR status returns colored string", func(t *testing.T) {
		status := TestStatusError
		result := status.StringColor()
		if result == "" {
			t.Error("expected non-empty colored string")
		}
		// Should contain the text "ERROR"
		if !contains(result, "ERROR") {
			t.Errorf("expected colored string to contain 'ERROR', got: %s", result)
		}
	})

	t.Run("UNKNOWN status returns plain string", func(t *testing.T) {
		status := TestSummaryStatus(999)
		result := status.StringColor()
		if result != "UNKNOWN" {
			t.Errorf("expected 'UNKNOWN', got '%s'", result)
		}
	})
}

func TestTestSummaryStatus_IsBad(t *testing.T) {
	tests := []struct {
		name     string
		status   TestSummaryStatus
		expected bool
	}{
		{"OK is not bad", TestStatusOk, false},
		{"FAILED is bad", TestStatusFailed, true},
		{"ERROR is bad", TestStatusError, true},
		{"Unknown is not bad", TestSummaryStatus(999), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.IsBad()
			if result != tt.expected {
				t.Errorf("expected IsBad() to return %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestTestSummaryStatus_AllValues(t *testing.T) {
	t.Run("TestStatusOk has correct value", func(t *testing.T) {
		if TestStatusOk != 0 {
			t.Errorf("expected TestStatusOk to be 0, got %d", TestStatusOk)
		}
	})

	t.Run("TestStatusFailed has correct value", func(t *testing.T) {
		if TestStatusFailed != 1 {
			t.Errorf("expected TestStatusFailed to be 1, got %d", TestStatusFailed)
		}
	})

	t.Run("TestStatusError has correct value", func(t *testing.T) {
		if TestStatusError != 2 {
			t.Errorf("expected TestStatusError to be 2, got %d", TestStatusError)
		}
	})
}

// Helper function
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
