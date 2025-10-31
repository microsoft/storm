package devops

import (
	"bytes"
	"testing"
)

// captureOutput captures output written to realStdOut during test execution
func captureOutput(f func()) string {
	// Save the original realStdOut
	original := realStdOut

	// Create a buffer to capture output
	var buf bytes.Buffer
	realStdOut = &buf

	// Restore realStdOut when done
	defer func() {
		realStdOut = original
	}()

	// Run the function
	f()

	return buf.String()
}

func TestPublishArtifact_EmptyName(t *testing.T) {
	err := PublishArtifact("", "", "/path/to/file")
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
	if err.Error() != "artifact name cannot be empty" {
		t.Errorf("expected 'artifact name cannot be empty', got '%s'", err.Error())
	}
}

func TestPublishArtifact_EmptySource(t *testing.T) {
	err := PublishArtifact("", "myartifact", "")
	if err == nil {
		t.Error("expected error for empty source, got nil")
	}
	if err.Error() != "artifact source cannot be empty" {
		t.Errorf("expected 'artifact source cannot be empty', got '%s'", err.Error())
	}
}

func TestPublishArtifact_WithoutFolder(t *testing.T) {
	output := captureOutput(func() {
		err := PublishArtifact("", "myartifact", "/path/to/file.txt")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	expected := "##vso[artifact.upload artifactname=myartifact]/path/to/file.txt"
	if output != expected {
		t.Errorf("expected output '%s', got '%s'", expected, output)
	}
}

func TestPublishArtifact_WithFolder(t *testing.T) {
	output := captureOutput(func() {
		err := PublishArtifact("logs", "myartifact", "/path/to/file.txt")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	expected := "##vso[artifact.upload containerfolder=logs;artifactname=myartifact]/path/to/file.txt"
	if output != expected {
		t.Errorf("expected output '%s', got '%s'", expected, output)
	}
}

func TestPublishArtifact_WithFolderLeadingSlash(t *testing.T) {
	output := captureOutput(func() {
		err := PublishArtifact("/logs", "myartifact", "/path/to/file.txt")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	expected := "##vso[artifact.upload containerfolder=logs;artifactname=myartifact]/path/to/file.txt"
	if output != expected {
		t.Errorf("expected output '%s', got '%s'", expected, output)
	}
}

func TestUploadArtifactInFolder(t *testing.T) {
	output := captureOutput(func() {
		uploadArtifactInFolder("reports", "test-results", "/tmp/results.xml")
	})

	expected := "##vso[artifact.upload containerfolder=reports;artifactname=test-results]/tmp/results.xml"
	if output != expected {
		t.Errorf("expected output '%s', got '%s'", expected, output)
	}
}

func TestUploadArtifactInFolder_NestedFolder(t *testing.T) {
	output := captureOutput(func() {
		uploadArtifactInFolder("logs/test/results", "output", "/var/log/test.log")
	})

	expected := "##vso[artifact.upload containerfolder=logs/test/results;artifactname=output]/var/log/test.log"
	if output != expected {
		t.Errorf("expected output '%s', got '%s'", expected, output)
	}
}

func TestUploadArtifact(t *testing.T) {
	output := captureOutput(func() {
		uploadArtifact("build-output", "/build/app.exe")
	})

	expected := "##vso[artifact.upload artifactname=build-output]/build/app.exe"
	if output != expected {
		t.Errorf("expected output '%s', got '%s'", expected, output)
	}
}

func TestUploadArtifact_WithSpaces(t *testing.T) {
	output := captureOutput(func() {
		uploadArtifact("my artifact", "/path/to/my file.txt")
	})

	expected := "##vso[artifact.upload artifactname=my artifact]/path/to/my file.txt"
	if output != expected {
		t.Errorf("expected output '%s', got '%s'", expected, output)
	}
}
