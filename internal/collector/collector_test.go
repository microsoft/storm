package collector

import (
	"errors"
	"testing"

	"github.com/microsoft/storm/pkg/storm/core"
)

// Mock TestRegistrant for testing
type mockRegistrant struct {
	name              string
	registerTestCases func(r core.TestRegistrar) error
}

func (m *mockRegistrant) Name() string {
	return m.name
}

func (m *mockRegistrant) RegisterTestCases(r core.TestRegistrar) error {
	return m.registerTestCases(r)
}

// Mock TestCaseFunction for testing
func mockTestCaseFunc(tc core.TestCase) error {
	return nil
}

func TestCollectTestCases_Success(t *testing.T) {
	registrant := &mockRegistrant{
		name: "test-registrant",
		registerTestCases: func(r core.TestRegistrar) error {
			r.RegisterTestCase("test1", mockTestCaseFunc)
			r.RegisterTestCase("test2", mockTestCaseFunc)
			r.RegisterTestCase("test3", mockTestCaseFunc)
			return nil
		},
	}

	testCases, err := CollectTestCases(registrant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(testCases) != 3 {
		t.Errorf("expected 3 test cases, got %d", len(testCases))
	}

	expectedNames := []string{"test1", "test2", "test3"}
	for i, tc := range testCases {
		if tc.Name != expectedNames[i] {
			t.Errorf("expected test case name '%s', got '%s'", expectedNames[i], tc.Name)
		}
		if tc.F == nil {
			t.Errorf("test case function should not be nil")
		}
	}
}

func TestCollectTestCases_EmptyRegistration(t *testing.T) {
	registrant := &mockRegistrant{
		name: "empty-registrant",
		registerTestCases: func(r core.TestRegistrar) error {
			// Don't register any test cases
			return nil
		},
	}

	testCases, err := CollectTestCases(registrant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(testCases) != 0 {
		t.Errorf("expected 0 test cases, got %d", len(testCases))
	}
}

func TestCollectTestCases_RegistrationError(t *testing.T) {
	expectedErr := errors.New("registration failed")
	registrant := &mockRegistrant{
		name: "error-registrant",
		registerTestCases: func(r core.TestRegistrar) error {
			return expectedErr
		},
	}

	testCases, err := CollectTestCases(registrant)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if testCases != nil {
		t.Errorf("expected nil test cases on error, got %v", testCases)
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap original error")
	}
}

func TestCollectTestCases_DuplicateNames(t *testing.T) {
	registrant := &mockRegistrant{
		name: "duplicate-registrant",
		registerTestCases: func(r core.TestRegistrar) error {
			r.RegisterTestCase("test1", mockTestCaseFunc)
			r.RegisterTestCase("test2", mockTestCaseFunc)
			r.RegisterTestCase("test1", mockTestCaseFunc) // Duplicate
			return nil
		},
	}

	testCases, err := CollectTestCases(registrant)
	if err == nil {
		t.Fatal("expected error for duplicate test case names, got nil")
	}

	if testCases != nil {
		t.Errorf("expected nil test cases on error, got %v", testCases)
	}

	expectedErrMsg := "test case name 'test1' is not unique"
	if err.Error() != expectedErrMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedErrMsg, err.Error())
	}
}

func TestCollectTestCases_InvalidName_SpecialCharacters(t *testing.T) {
	registrant := &mockRegistrant{
		name: "invalid-name-registrant",
		registerTestCases: func(r core.TestRegistrar) error {
			r.RegisterTestCase("test@case", mockTestCaseFunc) // Invalid character
			return nil
		},
	}

	testCases, err := CollectTestCases(registrant)
	if err == nil {
		t.Fatal("expected error for invalid test case name, got nil")
	}

	if testCases != nil {
		t.Errorf("expected nil test cases on error, got %v", testCases)
	}

	// Check that it's an InvalidNameError
	var invalidNameErr core.InvalidNameError
	if !errors.As(err, &invalidNameErr) {
		t.Errorf("expected InvalidNameError, got %T", err)
	}
}

func TestCollectTestCases_InvalidName_Spaces(t *testing.T) {
	registrant := &mockRegistrant{
		name: "invalid-space-registrant",
		registerTestCases: func(r core.TestRegistrar) error {
			r.RegisterTestCase("test case", mockTestCaseFunc) // Space not allowed
			return nil
		},
	}

	testCases, err := CollectTestCases(registrant)
	if err == nil {
		t.Fatal("expected error for invalid test case name with space, got nil")
	}

	if testCases != nil {
		t.Errorf("expected nil test cases on error, got %v", testCases)
	}
}

func TestCollectTestCases_ValidNames_WithDashesAndUnderscores(t *testing.T) {
	registrant := &mockRegistrant{
		name: "valid-names-registrant",
		registerTestCases: func(r core.TestRegistrar) error {
			r.RegisterTestCase("test-case-1", mockTestCaseFunc)
			r.RegisterTestCase("test_case_2", mockTestCaseFunc)
			r.RegisterTestCase("test-case_3", mockTestCaseFunc)
			return nil
		},
	}

	testCases, err := CollectTestCases(registrant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(testCases) != 3 {
		t.Errorf("expected 3 test cases, got %d", len(testCases))
	}

	expectedNames := []string{"test-case-1", "test_case_2", "test-case_3"}
	for i, tc := range testCases {
		if tc.Name != expectedNames[i] {
			t.Errorf("expected test case name '%s', got '%s'", expectedNames[i], tc.Name)
		}
	}
}

func TestTestCaseCollector_RegisterTestCase(t *testing.T) {
	collector := testCaseCollector{
		testCases: make([]TestCaseMetadata, 0),
	}

	collector.RegisterTestCase("test1", mockTestCaseFunc)
	collector.RegisterTestCase("test2", mockTestCaseFunc)

	if len(collector.testCases) != 2 {
		t.Errorf("expected 2 test cases, got %d", len(collector.testCases))
	}

	if collector.testCases[0].Name != "test1" {
		t.Errorf("expected first test case name 'test1', got '%s'", collector.testCases[0].Name)
	}

	if collector.testCases[1].Name != "test2" {
		t.Errorf("expected second test case name 'test2', got '%s'", collector.testCases[1].Name)
	}
}
