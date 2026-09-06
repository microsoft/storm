package core

import (
	"testing"
)

func TestValidateEntityName(t *testing.T) {
	t.Run("accepts valid alphanumeric names", func(t *testing.T) {
		validNames := []string{
			"test123",
			"Test123",
			"TEST",
			"123test",
			"T",
			"1",
		}
		for _, name := range validNames {
			err := ValidateEntityName(name, "test entity")
			if err != nil {
				t.Errorf("expected valid name '%s' to pass, got error: %v", name, err)
			}
		}
	})

	t.Run("accepts names with dashes", func(t *testing.T) {
		validNames := []string{
			"test-name",
			"test-123",
			"a-b-c",
			"-test",
			"test-",
		}
		for _, name := range validNames {
			err := ValidateEntityName(name, "test entity")
			if err != nil {
				t.Errorf("expected valid name '%s' to pass, got error: %v", name, err)
			}
		}
	})

	t.Run("accepts names with underscores", func(t *testing.T) {
		validNames := []string{
			"test_name",
			"test_123",
			"a_b_c",
			"_test",
			"test_",
		}
		for _, name := range validNames {
			err := ValidateEntityName(name, "test entity")
			if err != nil {
				t.Errorf("expected valid name '%s' to pass, got error: %v", name, err)
			}
		}
	})

	t.Run("accepts mixed valid characters", func(t *testing.T) {
		validNames := []string{
			"Test_Name-123",
			"my-test_case",
			"ABC_123-xyz",
		}
		for _, name := range validNames {
			err := ValidateEntityName(name, "test entity")
			if err != nil {
				t.Errorf("expected valid name '%s' to pass, got error: %v", name, err)
			}
		}
	})

	t.Run("rejects names with spaces", func(t *testing.T) {
		invalidNames := []string{
			"test name",
			"test name 123",
			" test",
			"test ",
			"a b c",
		}
		for _, name := range invalidNames {
			err := ValidateEntityName(name, "test entity")
			if err == nil {
				t.Errorf("expected invalid name '%s' to fail", name)
			}
		}
	})

	t.Run("rejects names with special characters", func(t *testing.T) {
		invalidNames := []string{
			"test@name",
			"test.name",
			"test!name",
			"test#name",
			"test$name",
			"test%name",
			"test&name",
			"test*name",
			"test(name",
			"test)name",
			"test+name",
			"test=name",
			"test[name",
			"test]name",
			"test{name",
			"test}name",
			"test|name",
			"test\\name",
			"test/name",
			"test:name",
			"test;name",
			"test'name",
			"test\"name",
			"test<name",
			"test>name",
			"test?name",
			"test,name",
		}
		for _, name := range invalidNames {
			err := ValidateEntityName(name, "test entity")
			if err == nil {
				t.Errorf("expected invalid name '%s' to fail", name)
			}
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		err := ValidateEntityName("", "test entity")
		if err == nil {
			t.Error("expected empty name to fail")
		}
	})

	t.Run("returns InvalidNameError with correct fields", func(t *testing.T) {
		name := "invalid name"
		entity := "test entity"
		err := ValidateEntityName(name, entity)

		if err == nil {
			t.Fatal("expected error")
		}

		invalidErr, ok := err.(InvalidNameError)
		if !ok {
			t.Fatalf("expected InvalidNameError, got %T", err)
		}

		if invalidErr.Name != name {
			t.Errorf("expected Name to be '%s', got '%s'", name, invalidErr.Name)
		}

		if invalidErr.Entity != entity {
			t.Errorf("expected Entity to be '%s', got '%s'", entity, invalidErr.Entity)
		}
	})
}

func TestInvalidNameError_Error(t *testing.T) {
	t.Run("formats error message correctly", func(t *testing.T) {
		err := InvalidNameError{
			Name:   "bad-name!",
			Entity: "test case",
		}
		expected := "Invalid name 'bad-name!' for test case, only alphanumeric characters, dashes and underscores are allowed"

		if err.Error() != expected {
			t.Errorf("expected error message:\n%s\ngot:\n%s", expected, err.Error())
		}
	})

	t.Run("includes name and entity in message", func(t *testing.T) {
		err := InvalidNameError{
			Name:   "my@helper",
			Entity: "helper",
		}
		msg := err.Error()

		if msg == "" {
			t.Error("expected non-empty error message")
		}
		// The message should contain both the name and entity
		expectedSubstrings := []string{"my@helper", "helper"}
		for _, substr := range expectedSubstrings {
			if !contains(msg, substr) {
				t.Errorf("expected error message to contain '%s', got: %s", substr, msg)
			}
		}
	})
}

func TestNAME_REGEX(t *testing.T) {
	t.Run("matches valid names", func(t *testing.T) {
		validNames := []string{
			"a",
			"A",
			"0",
			"abc123",
			"test-name",
			"test_name",
			"Test_Name-123",
		}
		for _, name := range validNames {
			if !NAME_REGEX.MatchString(name) {
				t.Errorf("expected NAME_REGEX to match '%s'", name)
			}
		}
	})

	t.Run("does not match invalid names", func(t *testing.T) {
		invalidNames := []string{
			"",
			"test name",
			"test.name",
			"test@name",
			"test!",
			"name with spaces",
		}
		for _, name := range invalidNames {
			if NAME_REGEX.MatchString(name) {
				t.Errorf("expected NAME_REGEX not to match '%s'", name)
			}
		}
	})
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
